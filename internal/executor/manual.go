// Package executor runs manually triggered tasks and persists their send results.
// Scheduled claiming and cross-task FIFO admission belong to later D/E units.
package executor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dongwlin/dan-tick/internal/credential"
	"github.com/dongwlin/dan-tick/internal/danmaku"
	"github.com/dongwlin/dan-tick/internal/diagnostic"
	"github.com/dongwlin/dan-tick/internal/task"
	"github.com/uptrace/bun"
)

var ErrNotRunning = errors.New("execution is not active in this process")

// sendingGate is supplied by the shared danmaku.Sender in production. It is
// deliberately not a Client: manual runs must not bypass account-wide limits.
type sendingGate interface {
	Send(context.Context, string, string) (danmaku.Result, error)
}

type Service struct {
	db     *bun.DB
	sender sendingGate
	log    *diagnostic.Logger
	now    func() time.Time
	wait   func(context.Context, time.Duration) error

	mu     sync.Mutex
	active map[int64]*activeRun
}

func NewService(db *bun.DB, sender *danmaku.Sender, log *diagnostic.Logger) *Service {
	return &Service{db: db, sender: sender, log: log, now: time.Now, wait: waitInterval}
}

type activeRun struct {
	cancel  context.CancelFunc
	stopped bool // Protected by Service.mu.
}

type runPlan struct {
	roomID   string
	messages []string
	interval time.Duration
	maxCount *int64
	duration time.Duration
}

// ManualResult is the persisted outcome of a single-message or loop task run.
// A send failure is returned as an outcome and a safe error code, not as raw
// upstream text. Errors from RunManual mean registration or persistence failed.
type ManualResult struct {
	ExecutionID int64           `json:"execution_id"`
	Status      string          `json:"status"`
	Outcome     danmaku.Outcome `json:"outcome"`
	ErrorCode   string          `json:"error_code,omitempty"`
	SentCount   int             `json:"sent_count"`
	FailedCount int             `json:"failed_count"`
}

// StopExecution requests cancellation of a locally active run (including waits
// and in-flight HTTP). RunManual writes the final stopped status when the
// current request has settled; this method does not falsify that status early.
func (s *Service) StopExecution(executionID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run := s.active[executionID]
	if run == nil {
		return ErrNotRunning
	}
	run.stopped = true
	run.cancel()
	return nil
}

func (s *Service) wasStopped(run *activeRun) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return run.stopped
}

// RunManual registers one manual execution and its first pending attempt
// atomically before sending. Every later pending row is also committed before
// its send. The task's schedule cursor is never changed, and uncertain results
// are not retried. Call from a Go-managed goroutine for asynchronous UI use.
func (s *Service) RunManual(ctx context.Context, taskID int64) (ManualResult, error) {
	if taskID <= 0 {
		return ManualResult{}, task.ErrNotFound
	}

	var executionID int64
	var plan runPlan
	started := s.now()
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	run := &activeRun{cancel: stop}

	// Protect the small window between committing an execution and registering
	// its local cancel handle. A StopExecution call cannot miss a live run here.
	s.mu.Lock()
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var name string
		var kind task.Kind
		var interval, maxCount, duration sql.NullInt64
		err := tx.QueryRowContext(ctx, `
			SELECT t.name, t.kind, r.platform_room_id, t.interval_seconds,
				t.max_count, t.max_duration_seconds
			FROM tasks t JOIN rooms r ON r.id = t.room_id
			WHERE t.id = ? AND t.deleted_at IS NULL AND r.deleted_at IS NULL`,
			taskID).Scan(&name, &kind, &plan.roomID, &interval, &maxCount, &duration)
		if errors.Is(err, sql.ErrNoRows) {
			return task.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("load manual execution task: %w", err)
		}
		rows, err := tx.QueryContext(ctx, `
			SELECT content FROM task_messages WHERE task_id = ?
			ORDER BY position, id`, taskID)
		if err != nil {
			return fmt.Errorf("load manual execution messages: %w", err)
		}
		for rows.Next() {
			var content string
			if err := rows.Scan(&content); err != nil {
				rows.Close()
				return fmt.Errorf("read manual execution message: %w", err)
			}
			plan.messages = append(plan.messages, content)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return fmt.Errorf("read manual execution messages: %w", err)
		}
		switch kind {
		case task.Single:
			if len(plan.messages) != 1 {
				return task.ErrInvalidTask
			}
		case task.Loop:
			if len(plan.messages) == 0 || !interval.Valid || interval.Int64 <= 0 ||
				(!maxCount.Valid && !duration.Valid) ||
				(maxCount.Valid && maxCount.Int64 <= 0) ||
				(duration.Valid && duration.Int64 <= 0) ||
				interval.Int64 > int64((1<<63-1)/int64(time.Second)) ||
				(duration.Valid && duration.Int64 > int64((1<<63-1)/int64(time.Second))) {
				return task.ErrInvalidTask
			}
			plan.interval = time.Duration(interval.Int64) * time.Second
			if maxCount.Valid {
				plan.maxCount = &maxCount.Int64
			}
			if duration.Valid {
				plan.duration = time.Duration(duration.Int64) * time.Second
			}
		default:
			return task.ErrInvalidTask
		}
		var busy bool
		if err := tx.QueryRowContext(ctx, `
			SELECT EXISTS(SELECT 1 FROM executions WHERE task_id = ?
				AND status IN ('waiting', 'queued', 'running'))`, taskID).Scan(&busy); err != nil {
			return fmt.Errorf("check manual execution conflict: %w", err)
		}
		if busy {
			return task.ErrTaskBusy
		}

		now := started.UTC().UnixMilli()
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO executions (task_id, trigger_kind, status, created_at,
				started_at, task_name_snapshot, platform_room_id_snapshot)
			VALUES (?, 'manual', 'running', ?, ?, ?, ?) RETURNING id`,
			taskID, now, now, name, plan.roomID).Scan(&executionID); err != nil {
			return fmt.Errorf("register manual execution: %w", err)
		}
		// Committing the pending row is a hard prerequisite for sending.
		return insertPending(ctx, tx, executionID, 1, plan.messages[0], now)
	})
	if err == nil {
		if s.active == nil {
			s.active = make(map[int64]*activeRun)
		}
		s.active[executionID] = run
	}
	s.mu.Unlock()
	if err != nil {
		return ManualResult{}, err
	}
	defer func() {
		s.mu.Lock()
		delete(s.active, executionID)
		s.mu.Unlock()
	}()
	s.record(diagnostic.ExecutionStarted, executionID, taskID, "")
	return s.execute(runCtx, run, executionID, taskID, plan, started)
}

func insertPending(ctx context.Context, tx bun.IDB, id int64, seq int, message string, now int64) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO send_attempts (execution_id, sequence, message_no,
			content_snapshot, attempted_at, result)
		VALUES (?, ?, ?, ?, ?, 'pending')`, id, seq, seq, message, now); err != nil {
		return fmt.Errorf("register pending send: %w", err)
	}
	return nil
}

func (s *Service) execute(ctx context.Context, run *activeRun, id, taskID int64, plan runPlan, started time.Time) (ManualResult, error) {
	// Context timeout bounds even a blocked account permit or in-flight HTTP.
	// The explicit clock check below also supports deterministic duration tests.
	if plan.duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, plan.duration)
		defer cancel()
	}
	result := ManualResult{ExecutionID: id, Outcome: danmaku.NotSent}
	var lastStart time.Time
	for seq := 1; ; seq++ {
		if seq > 1 {
			if done, code := s.stopReason(ctx, run, plan, started); done {
				result.Status, result.ErrorCode = terminalStatus(code), code
				return s.finishExecution(ctx, taskID, result)
			}
			if delay := lastStart.Add(plan.interval).Sub(s.now()); delay > 0 {
				// Never sleep past the configured duration just to satisfy the
				// inter-message interval.
				if plan.duration > 0 {
					remaining := started.Add(plan.duration).Sub(s.now())
					if remaining < delay {
						delay = remaining
					}
				}
				wait := s.wait
				if delay > 0 {
					if wait == nil {
						wait = waitInterval
					}
					if err := wait(ctx, delay); err != nil && ctx.Err() == nil {
						// A failed timer is not permission to send early.
						result.Status, result.ErrorCode = "failed", "wait_failure"
						return s.finishExecution(ctx, taskID, result)
					}
				}
			}
			if done, code := s.stopReason(ctx, run, plan, started); done {
				result.Status, result.ErrorCode = terminalStatus(code), code
				return s.finishExecution(ctx, taskID, result)
			}
			message := plan.messages[(seq-1)%len(plan.messages)]
			// The registration must either commit or roll back independently
			// of an incoming stop; otherwise cancellation can strand an
			// unfinalized running execution without a known pending row.
			pendingCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 6*time.Second)
			err := s.db.RunInTx(pendingCtx, nil, func(ctx context.Context, tx bun.Tx) error {
				return insertPending(ctx, tx, id, seq, message, s.now().UTC().UnixMilli())
			})
			cancel()
			if err != nil {
				s.record(diagnostic.ExecutionPersistFailed, id, taskID, "")
				return ManualResult{ExecutionID: id}, fmt.Errorf("persist pending send: %w", err)
			}
		}

		message := plan.messages[(seq-1)%len(plan.messages)]
		lastStart = s.now()
		// Cancellation can arrive after a pending commit. Do not call even a
		// replaced sender in that case; resolve the row as not_sent instead.
		outcome, sendErr := danmaku.Result{Outcome: danmaku.NotSent}, ctx.Err()
		if sendErr == nil {
			outcome, sendErr = s.sender.Send(ctx, plan.roomID, message)
		}
		step := classify(id, outcome, sendErr)
		result.Outcome = step.Outcome
		result.SentCount += step.SentCount
		result.FailedCount += step.FailedCount

		// Never continue past a failed or uncertain send. A stop requested
		// during a request takes precedence over the transport's error.
		if s.wasStopped(run) {
			result.Status, result.ErrorCode = "stopped", "user_stopped"
		} else if ctx.Err() != nil {
			result.Status, result.ErrorCode = "stopped", "cancelled"
			if plan.duration > 0 && (ctx.Err() == context.DeadlineExceeded || !s.now().Before(started.Add(plan.duration))) {
				if step.Status == "completed" {
					result.Status, result.ErrorCode = "completed", ""
				} else {
					result.Status, result.ErrorCode = "failed", "timeout"
				}
			}
		} else if step.Status == "failed" {
			result.Status, result.ErrorCode = "failed", step.ErrorCode
		} else if plan.interval == 0 ||
			(plan.maxCount != nil && int64(seq) >= *plan.maxCount) ||
			(plan.duration > 0 && !s.now().Before(started.Add(plan.duration))) {
			result.Status = "completed"
		}
		if err := s.finishAttempt(ctx, taskID, result, seq, step.ErrorCode); err != nil {
			return ManualResult{ExecutionID: id}, err
		}
		if result.Status != "" {
			return result, nil
		}
	}
}

func (s *Service) stopReason(ctx context.Context, run *activeRun, plan runPlan, started time.Time) (bool, string) {
	if s.wasStopped(run) {
		return true, "user_stopped"
	}
	if ctx.Err() != nil {
		if plan.duration > 0 && ctx.Err() == context.DeadlineExceeded {
			return true, ""
		}
		return true, "cancelled"
	}
	if plan.duration > 0 && !s.now().Before(started.Add(plan.duration)) {
		return true, ""
	}
	return false, ""
}

func terminalStatus(code string) string {
	if code != "" {
		return "stopped"
	}
	return "completed"
}

// finishAttempt atomically saves the attempt result and the accumulated run
// counts. Intermediate successful attempts leave the execution running.
func (s *Service) finishAttempt(ctx context.Context, taskID int64, result ManualResult, seq int, attemptCode string) error {
	// Cancellation of the HTTP request must not prevent us from finalizing its
	// attempt. A bounded independent context keeps the pending row recoverable.
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 6*time.Second)
	defer cancel()
	err := s.db.RunInTx(finishCtx, nil, func(ctx context.Context, tx bun.Tx) error {
		attemptUpdate, err := tx.ExecContext(ctx, `
			UPDATE send_attempts SET result = ?, error_code = ?
			WHERE execution_id = ? AND sequence = ? AND result = 'pending'`,
			attemptResult(result.Outcome), nullableCode(attemptCode), result.ExecutionID, seq)
		if err != nil {
			return fmt.Errorf("finish send attempt: %w", err)
		}
		if affected, err := attemptUpdate.RowsAffected(); err != nil || affected != 1 {
			return fmt.Errorf("finish send attempt: expected one pending row")
		}
		now := s.now().UTC().UnixMilli()
		executionUpdate, err := tx.ExecContext(ctx, `
			UPDATE executions SET status = ?, finished_at = ?, sent_count = ?,
				failed_count = ?, error_code = ?
			WHERE id = ? AND status = 'running'`, runningStatus(result.Status), finishTime(result.Status, now),
			result.SentCount, result.FailedCount, nullableCode(result.ErrorCode), result.ExecutionID)
		if err != nil {
			return fmt.Errorf("finish manual execution: %w", err)
		}
		if affected, err := executionUpdate.RowsAffected(); err != nil || affected != 1 {
			return fmt.Errorf("finish manual execution: expected one running row")
		}
		return nil
	})
	if err != nil {
		s.record(diagnostic.ExecutionPersistFailed, result.ExecutionID, taskID, "")
		return fmt.Errorf("persist manual execution result: %w", err)
	}
	sendEvent := diagnostic.SendAccepted
	switch result.Outcome {
	case danmaku.Rejected:
		sendEvent = diagnostic.SendRejected
	case danmaku.NotSent:
		sendEvent = diagnostic.SendNotSent
	case danmaku.Unknown:
		sendEvent = diagnostic.SendUnknown
	}
	if s.log != nil {
		s.log.RecordSendAttempt(sendEvent, result.ExecutionID, taskID, seq, attemptCode)
	}
	if result.Status != "" {
		s.recordTerminal(taskID, result)
	}
	return nil
}

func runningStatus(status string) string {
	if status == "" {
		return "running"
	}
	return status
}

func finishTime(status string, now int64) any {
	if status == "" {
		return nil
	}
	return now
}

func (s *Service) finishExecution(ctx context.Context, taskID int64, result ManualResult) (ManualResult, error) {
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 6*time.Second)
	defer cancel()
	updated, err := s.db.ExecContext(finishCtx, `
		UPDATE executions SET status = ?, finished_at = ?, sent_count = ?,
			failed_count = ?, error_code = ?
		WHERE id = ? AND status = 'running'`, result.Status,
		s.now().UTC().UnixMilli(), result.SentCount, result.FailedCount,
		nullableCode(result.ErrorCode), result.ExecutionID)
	if err != nil {
		s.record(diagnostic.ExecutionPersistFailed, result.ExecutionID, taskID, "")
		return ManualResult{ExecutionID: result.ExecutionID}, fmt.Errorf("finish manual execution: %w", err)
	}
	if affected, err := updated.RowsAffected(); err != nil || affected != 1 {
		s.record(diagnostic.ExecutionPersistFailed, result.ExecutionID, taskID, "")
		return ManualResult{ExecutionID: result.ExecutionID}, fmt.Errorf("finish manual execution: expected one running row")
	}
	s.recordTerminal(taskID, result)
	return result, nil
}

func (s *Service) recordTerminal(taskID int64, result ManualResult) {
	switch result.Status {
	case "completed":
		s.record(diagnostic.ExecutionCompleted, result.ExecutionID, taskID, "")
	case "stopped":
		s.record(diagnostic.ExecutionStopped, result.ExecutionID, taskID, result.ErrorCode)
	default:
		s.record(diagnostic.ExecutionFailed, result.ExecutionID, taskID, result.ErrorCode)
	}
}

func waitInterval(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}

func classify(id int64, sent danmaku.Result, err error) ManualResult {
	r := ManualResult{ExecutionID: id, Status: "failed", Outcome: sent.Outcome, FailedCount: 1}
	if sent.Outcome == danmaku.Accepted && err == nil {
		r.Status, r.SentCount, r.FailedCount = "completed", 1, 0
		return r
	}
	if sent.Outcome != danmaku.NotSent && sent.Outcome != danmaku.Rejected &&
		sent.Outcome != danmaku.Unknown {
		r.Outcome = danmaku.Unknown
	}
	if sent.Outcome == danmaku.Accepted && err != nil {
		r.Outcome = danmaku.Unknown
	}
	r.ErrorCode = errorCode(err)
	return r
}

func attemptResult(outcome danmaku.Outcome) string {
	switch outcome {
	case danmaku.Accepted:
		return "accepted"
	case danmaku.Unknown:
		return "unknown"
	default: // rejected and not_sent are both unconfirmed logical messages.
		return "failed"
	}
}

func errorCode(err error) string {
	switch {
	case errors.Is(err, credential.ErrNotConfigured):
		return "credential_missing"
	case errors.Is(err, credential.ErrUnavailable):
		return "credential_unavailable"
	case errors.Is(err, credential.ErrCorrupt):
		return "credential_corrupt"
	case errors.Is(err, danmaku.ErrAuthentication):
		return "authentication"
	case errors.Is(err, danmaku.ErrRateLimited):
		return "rate_limited"
	case errors.Is(err, danmaku.ErrBadRequest):
		return "bad_request"
	case errors.Is(err, danmaku.ErrTooLong):
		return "too_long"
	case errors.Is(err, danmaku.ErrRejected):
		return "rejected"
	case errors.Is(err, danmaku.ErrInvalidInput):
		return "invalid_input"
	case errors.Is(err, danmaku.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, danmaku.ErrTransport):
		return "transport"
	case errors.Is(err, danmaku.ErrHTTPStatus):
		return "http_status"
	case errors.Is(err, danmaku.ErrInvalidResponse):
		return "invalid_response"
	case errors.Is(err, danmaku.ErrUncertain):
		return "uncertain"
	default:
		return "send_failure"
	}
}

func nullableCode(code string) any {
	if code == "" {
		return nil
	}
	return code
}

func (s *Service) record(event diagnostic.Event, executionID, taskID int64, category string) {
	if s.log != nil {
		s.log.RecordExecution(event, executionID, taskID, category)
	}
}
