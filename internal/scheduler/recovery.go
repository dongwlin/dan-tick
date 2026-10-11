package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dongwlin/dan-tick/internal/diagnostic"
	"github.com/dongwlin/dan-tick/internal/schedule"
	"github.com/uptrace/bun"
)

// CallbackGrace is the maximum lateness accepted for a live Cron callback.
// A normal timer jitter is harmless; prolonged sleep/clock jumps are not
// permission to send historical messages. Explicit startup/wake reconciliation
// never catches up overdue work, even within this window.
const CallbackGrace = 30 * time.Second

type RecoverySummary struct {
	Interrupted int
	Unknown     int
	Missed      int
}

type RecoveryService struct {
	db  *bun.DB
	log *diagnostic.Logger
	now func() time.Time
}

func NewRecoveryService(db *bun.DB, log *diagnostic.Logger) *RecoveryService {
	return &RecoveryService{db: db, log: log, now: time.Now}
}

type recoveredAttempt struct {
	executionID, taskID int64
	sequence            int
}

type recoveredExecution struct{ id, taskID int64 }

type missedPlan struct {
	executionID, taskID, scheduledAt int64
}

type recoveryEvents struct {
	interrupted []recoveredExecution
	unknown     []recoveredAttempt
	missed      []missedPlan
}

// RecoverStartup runs after opening/migrating SQLite and BEFORE registering
// callbacks or enabling any executor. In one transaction, it finalizes
// abandoned executions, resolves pending attempts as unknown, and advances
// every overdue enabled cursor without sending. A failed transaction leaves
// the entire recovery retryable on the next startup.
func (s *RecoveryService) RecoverStartup(ctx context.Context) (RecoverySummary, error) {
	now := s.now().UTC()
	var events recoveryEvents
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := s.interruptAbandoned(ctx, tx, now.UnixMilli(), &events); err != nil {
			return err
		}
		return s.reconcileDue(ctx, tx, now, nil, &events)
	})
	if err != nil {
		s.recordFailure()
		return RecoverySummary{}, err
	}
	return s.record(events), nil
}

// RecoverAfterWake handles an explicit suspend/resume notification (or a
// periodic reconciliation by the application coordinator). Unlike startup,
// it must NOT interrupt executions that are alive in the current process.
// Overdue plans are missed, never replayed. The Manager rebuilds Cron jobs
// only after this transaction commits.
func (s *RecoveryService) RecoverAfterWake(ctx context.Context) (RecoverySummary, error) {
	now := s.now().UTC()
	var events recoveryEvents
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return s.reconcileDue(ctx, tx, now, nil, &events)
	})
	if err != nil {
		s.recordFailure()
		return RecoverySummary{}, err
	}
	return s.record(events), nil
}

// ReconcileLate is used inside a live Cron callback. Only a callback more
// than CallbackGrace late may consume the occurrence as missed; a punctual
// callback is still claimed by E1. Stale callbacks are idempotent no-ops.
func (s *RecoveryService) ReconcileLate(ctx context.Context, taskID, due int64) (bool, error) {
	now := s.now().UTC()
	if taskID <= 0 || due <= 0 {
		return false, ErrInvalidOccurrence
	}
	if !now.After(time.UnixMilli(due).Add(CallbackGrace)) {
		return false, nil
	}
	var events recoveryEvents
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return s.reconcileDue(ctx, tx, now, &pendingPlan{id: taskID, due: due}, &events)
	})
	if err != nil {
		s.recordFailure()
		return false, err
	}
	s.record(events)
	return true, nil
}

func (s *RecoveryService) interruptAbandoned(ctx context.Context, tx bun.Tx, now int64, events *recoveryEvents) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, task_id FROM executions
		WHERE status IN ('waiting', 'queued', 'running') ORDER BY id`)
	if err != nil {
		return fmt.Errorf("find abandoned executions: %w", err)
	}
	var active []recoveredExecution
	for rows.Next() {
		var run recoveredExecution
		if err := rows.Scan(&run.id, &run.taskID); err != nil {
			rows.Close()
			return fmt.Errorf("read abandoned execution: %w", err)
		}
		active = append(active, run)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return fmt.Errorf("list abandoned executions: %w", err)
	}
	for _, run := range active {
		pending, err := tx.QueryContext(ctx, `
			SELECT sequence FROM send_attempts
			WHERE execution_id = ? AND result = 'pending' ORDER BY sequence`, run.id)
		if err != nil {
			return fmt.Errorf("find abandoned attempts: %w", err)
		}
		for pending.Next() {
			var seq int
			if err := pending.Scan(&seq); err != nil {
				pending.Close()
				return fmt.Errorf("read abandoned attempt: %w", err)
			}
			events.unknown = append(events.unknown, recoveredAttempt{run.id, run.taskID, seq})
		}
		err = pending.Err()
		pending.Close()
		if err != nil {
			return fmt.Errorf("list abandoned attempts: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE send_attempts SET result = 'unknown', error_code = 'recovery_unknown'
			WHERE execution_id = ? AND result = 'pending'`, run.id); err != nil {
			return fmt.Errorf("resolve abandoned attempts: %w", err)
		}
		// Count distinct logical messages, not HTTP retry attempts. An
		// accepted logical message must never become a failed count as well.
		var sent, failed int
		err = tx.QueryRowContext(ctx, `
			SELECT
				COUNT(DISTINCT CASE WHEN result = 'accepted' THEN message_no END),
				COUNT(DISTINCT CASE WHEN result IN ('failed', 'unknown')
					AND message_no NOT IN (
						SELECT message_no FROM send_attempts
						WHERE execution_id = ? AND result = 'accepted'
					) THEN message_no END)
			FROM send_attempts WHERE execution_id = ?`, run.id, run.id).Scan(&sent, &failed)
		if err != nil {
			return fmt.Errorf("count recovered execution attempts: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE executions SET status = 'interrupted', finished_at = ?,
				sent_count = ?, failed_count = ?, error_code = 'recovery_interrupted',
				error_message = NULL
			WHERE id = ? AND status IN ('waiting', 'queued', 'running')`,
			now, sent, failed, run.id); err != nil {
			return fmt.Errorf("finalize abandoned execution: %w", err)
		}
		events.interrupted = append(events.interrupted, run)
	}
	return nil
}

// reconcileDue selects at most the current persisted overdue cursor for each
// task. It does not enumerate past recurrence instances: NextAfter(now) jumps
// straight to the next future wall-clock occurrence, bounded by A3's search.
func (s *RecoveryService) reconcileDue(ctx context.Context, tx bun.Tx, now time.Time, only *pendingPlan, events *recoveryEvents) error {
	var plans []pendingPlan
	if only == nil {
		rows, err := tx.QueryContext(ctx, `
			SELECT t.id, t.next_due_at FROM tasks t JOIN rooms r ON r.id = t.room_id
			WHERE t.enabled = 1 AND t.deleted_at IS NULL AND r.deleted_at IS NULL
				AND t.next_due_at IS NOT NULL AND t.next_due_at <= ?
			ORDER BY t.id`, now.UnixMilli())
		if err != nil {
			return fmt.Errorf("find overdue tasks: %w", err)
		}
		for rows.Next() {
			var plan pendingPlan
			if err := rows.Scan(&plan.id, &plan.due); err != nil {
				rows.Close()
				return fmt.Errorf("read overdue task: %w", err)
			}
			plans = append(plans, plan)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return fmt.Errorf("list overdue tasks: %w", err)
		}
	} else {
		plans = append(plans, *only)
	}
	for _, plan := range plans {
		if err := s.missOccurrence(ctx, tx, now, plan, events); err != nil {
			return err
		}
	}
	return nil
}

func (s *RecoveryService) missOccurrence(ctx context.Context, tx bun.Tx, now time.Time, plan pendingPlan, events *recoveryEvents) error {
	var kind schedule.Kind
	var timezone, localTime, name, roomID string
	var localDate sql.NullString
	var weekdays sql.NullInt64
	var stored int64
	err := tx.QueryRowContext(ctx, `
		SELECT t.schedule_kind, t.timezone, t.local_date, t.local_time,
			t.weekdays, t.name, r.platform_room_id, t.next_due_at
		FROM tasks t JOIN rooms r ON r.id = t.room_id
		WHERE t.id = ? AND t.enabled = 1 AND t.deleted_at IS NULL
			AND r.deleted_at IS NULL AND t.next_due_at = ?`,
		plan.id, plan.due).Scan(&kind, &timezone, &localDate, &localTime, &weekdays, &name, &roomID, &stored)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // another callback/edit already advanced the occurrence
	}
	if err != nil {
		return fmt.Errorf("load overdue occurrence: %w", err)
	}
	if stored > now.UnixMilli() {
		return nil
	}
	var next *int64
	if kind != schedule.Once {
		instant, ok, err := schedule.NextAfter(schedule.Rule{
			Kind: kind, Timezone: timezone, LocalDate: localDate.String,
			LocalTime: localTime, Weekdays: int(weekdays.Int64),
		}, now)
		if err != nil || !ok {
			if err != nil {
				return fmt.Errorf("calculate future occurrence for task %d: %w", plan.id, err)
			}
			return fmt.Errorf("task %d: recurring rule has no future occurrence", plan.id)
		}
		value := instant.UTC().UnixMilli()
		next = &value
	}
	// A former claim may exist if an external repair rewound a cursor. Keep
	// its unique scheduled identity intact and only advance the stale cursor.
	var existing int64
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM executions WHERE task_id = ? AND trigger_kind = 'scheduled'
			AND scheduled_at = ?`, plan.id, plan.due).Scan(&existing)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check overdue occurrence identity: %w", err)
	}
	updated, err := tx.ExecContext(ctx, `
		UPDATE tasks SET next_due_at = ?, updated_at = ?
		WHERE id = ? AND enabled = 1 AND deleted_at IS NULL AND next_due_at = ?`,
		next, now.UnixMilli(), plan.id, plan.due)
	if err != nil {
		return fmt.Errorf("advance overdue cursor: %w", err)
	}
	affected, err := updated.RowsAffected()
	if err != nil {
		return fmt.Errorf("check overdue cursor: %w", err)
	}
	if affected != 1 {
		return nil
	}
	if existing != 0 {
		return nil
	}
	var id int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO executions (task_id, trigger_kind, scheduled_at, status,
			created_at, finished_at, error_code, task_name_snapshot,
			platform_room_id_snapshot)
		VALUES (?, 'scheduled', ?, 'missed', ?, ?, 'schedule_overdue', ?, ?)
		RETURNING id`, plan.id, plan.due, now.UnixMilli(), now.UnixMilli(), name, roomID).Scan(&id)
	if err != nil {
		return fmt.Errorf("record missed occurrence: %w", err)
	}
	events.missed = append(events.missed, missedPlan{id, plan.id, plan.due})
	return nil
}

func (s *RecoveryService) record(events recoveryEvents) RecoverySummary {
	if s.log != nil {
		for _, attempt := range events.unknown {
			s.log.RecordSendAttempt(diagnostic.SendUnknown, attempt.executionID, attempt.taskID,
				attempt.sequence, "recovery_unknown")
		}
		for _, run := range events.interrupted {
			s.log.RecordExecution(diagnostic.ExecutionInterrupted, run.id, run.taskID, "")
		}
		for _, plan := range events.missed {
			s.log.RecordSchedule(diagnostic.ScheduleMissed, plan.executionID, plan.taskID, plan.scheduledAt)
		}
	}
	return RecoverySummary{Interrupted: len(events.interrupted), Unknown: len(events.unknown), Missed: len(events.missed)}
}

func (s *RecoveryService) recordFailure() {
	if s.log != nil {
		s.log.Record(diagnostic.RecoveryFailed)
	}
}
