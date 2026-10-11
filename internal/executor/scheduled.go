package executor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dongwlin/dan-tick/internal/diagnostic"
	"github.com/uptrace/bun"
)

var (
	ErrExecutionNotFound   = errors.New("execution not found")
	ErrNotScheduled        = errors.New("execution is not scheduled")
	ErrExecutionNotWaiting = errors.New("execution is not awaiting takeover")
)

// RunScheduled takes over an existing scheduled/waiting execution once. Its
// queued transition and first pending attempt commit together before sending.
// The original occurrence, snapshots and task cursor remain unchanged. A
// duplicate call returns ErrExecutionNotWaiting without repeating any send.
// Registration failures leave the untouched waiting row for the scheduler
// to finalize; errors after takeover retain their recoverable attempt state.
func (s *Service) RunScheduled(ctx context.Context, executionID int64) (ManualResult, error) {
	run, err := s.prepareScheduled(ctx, executionID)
	if err != nil {
		return ManualResult{ExecutionID: executionID}, err
	}
	defer s.done.Done()
	return s.executeScheduled(run)
}

// StartScheduled commits takeover before returning, then runs the send chain
// asynchronously. A Cron handoff can report preparation failure immediately
// without waiting for an entire loop. onDone receives the final send outcome
// or persistence error, and may be nil when no completion callback is needed.
func (s *Service) StartScheduled(ctx context.Context, executionID int64, onDone func(ManualResult, error)) error {
	run, err := s.prepareScheduled(ctx, executionID)
	if err != nil {
		return err
	}
	go func() {
		defer s.done.Done()
		result, err := s.executeScheduled(run)
		if onDone != nil {
			onDone(result, err)
		}
	}()
	return nil
}

type scheduledRun struct {
	ctx                 context.Context
	run                 *activeRun
	executionID, taskID int64
	plan                runPlan
	started             time.Time
}

func (s *Service) prepareScheduled(ctx context.Context, executionID int64) (*scheduledRun, error) {
	if executionID <= 0 {
		return nil, ErrExecutionNotFound
	}
	var taskID int64
	var plan runPlan
	started := s.now()
	runCtx, stop := context.WithCancel(ctx)
	run := &activeRun{cancel: stop}

	// As with manual runs, a committed takeover must already have its local
	// cancellation handle when StopExecution can inspect it.
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		stop()
		return nil, ErrExecutorClosed
	}
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var trigger, status string
		err := tx.QueryRowContext(ctx, `
			SELECT task_id, trigger_kind, status FROM executions WHERE id = ?`,
			executionID).Scan(&taskID, &trigger, &status)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrExecutionNotFound
		}
		if err != nil {
			return fmt.Errorf("load scheduled execution: %w", err)
		}
		if trigger != "scheduled" {
			return ErrNotScheduled
		}
		if status != "waiting" {
			return ErrExecutionNotWaiting
		}
		var attempts int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM send_attempts WHERE execution_id = ?`, executionID).Scan(&attempts); err != nil {
			return fmt.Errorf("check scheduled execution attempts: %w", err)
		}
		if attempts != 0 {
			return ErrExecutionNotWaiting
		}
		loaded, _, err := loadPlan(ctx, tx, taskID)
		if err != nil {
			return err
		}
		plan = loaded
		now := started.UTC().UnixMilli()
		updated, err := tx.ExecContext(ctx, `
			UPDATE executions SET status = 'queued', queued_at = ?
			WHERE id = ? AND trigger_kind = 'scheduled' AND status = 'waiting'
				AND NOT EXISTS (SELECT 1 FROM send_attempts WHERE execution_id = ?)`,
			now, executionID, executionID)
		if err != nil {
			return fmt.Errorf("take over scheduled execution: %w", err)
		}
		affected, err := updated.RowsAffected()
		if err != nil {
			return fmt.Errorf("check scheduled execution takeover: %w", err)
		}
		if affected != 1 {
			return ErrExecutionNotWaiting
		}
		return insertPending(ctx, tx, executionID, 1, plan.messages[0], now)
	})
	if err == nil {
		if s.active == nil {
			s.active = make(map[int64]*activeRun)
		}
		s.active[executionID] = run
		s.done.Add(1)
	}
	s.mu.Unlock()
	if err != nil {
		stop()
		return nil, err
	}
	s.record(diagnostic.ExecutionQueued, executionID, taskID, "")
	return &scheduledRun{ctx: runCtx, run: run, executionID: executionID,
		taskID: taskID, plan: plan, started: started}, nil
}

func (s *Service) executeScheduled(run *scheduledRun) (ManualResult, error) {
	defer func() {
		run.run.cancel()
		s.mu.Lock()
		delete(s.active, run.executionID)
		s.mu.Unlock()
	}()
	return s.execute(run.ctx, run.run, run.executionID, run.taskID, run.plan, run.started)
}
