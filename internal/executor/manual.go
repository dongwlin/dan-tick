// Package executor records task runs and their send outcomes in SQLite.
// Scheduling, loops and explicit stop control are added in later D/E units.
package executor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dongwlin/dan-tick/internal/credential"
	"github.com/dongwlin/dan-tick/internal/danmaku"
	"github.com/dongwlin/dan-tick/internal/diagnostic"
	"github.com/dongwlin/dan-tick/internal/task"
	"github.com/uptrace/bun"
)

var ErrUnsupportedKind = errors.New("loop execution is not supported yet")

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
}

func NewService(db *bun.DB, sender *danmaku.Sender, log *diagnostic.Logger) *Service {
	return &Service{db: db, sender: sender, log: log, now: time.Now}
}

// ManualResult is the persisted outcome of one single-message task run.
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

// RunManual registers one manual execution and its pending attempt atomically
// before calling the shared Sender. It never modifies tasks.next_due_at.
// Unknown results are never retried. D2 will handle loops and explicit stop.
func (s *Service) RunManual(ctx context.Context, taskID int64) (ManualResult, error) {
	if taskID <= 0 {
		return ManualResult{}, task.ErrNotFound
	}

	var executionID int64
	var roomID, message string
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var name string
		var kind task.Kind
		err := tx.QueryRowContext(ctx, `
			SELECT t.name, t.kind, r.platform_room_id
			FROM tasks t JOIN rooms r ON r.id = t.room_id
			WHERE t.id = ? AND t.deleted_at IS NULL AND r.deleted_at IS NULL`,
			taskID).Scan(&name, &kind, &roomID)
		if errors.Is(err, sql.ErrNoRows) {
			return task.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("load manual execution task: %w", err)
		}
		if kind != task.Single {
			return ErrUnsupportedKind
		}
		if err := tx.QueryRowContext(ctx, `
			SELECT content FROM task_messages WHERE task_id = ?
			ORDER BY position, id LIMIT 1`, taskID).Scan(&message); err != nil {
			return fmt.Errorf("load manual execution message: %w", err)
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

		now := s.now().UTC().UnixMilli()
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO executions (task_id, trigger_kind, status, created_at,
				started_at, task_name_snapshot, platform_room_id_snapshot)
			VALUES (?, 'manual', 'running', ?, ?, ?, ?) RETURNING id`,
			taskID, now, now, name, roomID).Scan(&executionID); err != nil {
			return fmt.Errorf("register manual execution: %w", err)
		}
		// Committing the pending row is a hard prerequisite for sending.
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO send_attempts (execution_id, sequence, message_no,
				content_snapshot, attempted_at, result)
			VALUES (?, 1, 1, ?, ?, 'pending')`, executionID, message, now); err != nil {
			return fmt.Errorf("register pending send: %w", err)
		}
		return nil
	})
	if err != nil {
		return ManualResult{}, err
	}
	s.record(diagnostic.ExecutionStarted, executionID, taskID, "")

	outcome, sendErr := s.sender.Send(ctx, roomID, message)
	result := classify(executionID, outcome, sendErr)
	// Cancellation of the HTTP request must not prevent us from finalizing its
	// attempt. A bounded independent context keeps the pending row recoverable.
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 6*time.Second)
	defer cancel()
	err = s.db.RunInTx(finishCtx, nil, func(ctx context.Context, tx bun.Tx) error {
		attemptUpdate, err := tx.ExecContext(ctx, `
			UPDATE send_attempts SET result = ?, error_code = ?
			WHERE execution_id = ? AND sequence = 1 AND result = 'pending'`,
			attemptResult(result.Outcome), nullableCode(result.ErrorCode), executionID)
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
			WHERE id = ? AND status = 'running'`, result.Status, now,
			result.SentCount, result.FailedCount, nullableCode(result.ErrorCode), executionID)
		if err != nil {
			return fmt.Errorf("finish manual execution: %w", err)
		}
		if affected, err := executionUpdate.RowsAffected(); err != nil || affected != 1 {
			return fmt.Errorf("finish manual execution: expected one running row")
		}
		return nil
	})
	if err != nil {
		s.record(diagnostic.ExecutionPersistFailed, executionID, taskID, "")
		return ManualResult{ExecutionID: executionID}, fmt.Errorf("persist manual execution result: %w", err)
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
	s.record(sendEvent, executionID, taskID, result.ErrorCode)
	if result.Status == "completed" {
		s.record(diagnostic.ExecutionCompleted, executionID, taskID, "")
	} else {
		s.record(diagnostic.ExecutionFailed, executionID, taskID, result.ErrorCode)
	}
	return result, nil
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
