// Package scheduler claims persisted task occurrences before they can be
// handed to the executor. Cron registration and restart recovery are separate
// steps; neither a timer nor an in-memory lock is the source of truth.
package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dongwlin/dan-tick/internal/diagnostic"
	"github.com/dongwlin/dan-tick/internal/schedule"
	"github.com/dongwlin/dan-tick/internal/settings"
	"github.com/uptrace/bun"
)

var ErrInvalidOccurrence = errors.New("invalid scheduled occurrence")

// ClaimResult is only actionable when Claimed is true and Status is waiting.
// A missed occurrence has already been persisted and must never be sent.
// A stale, disabled, deleted or early callback returns the zero value.
type ClaimResult struct {
	Claimed     bool
	ExecutionID int64
	ScheduledAt int64 // UTC Unix milliseconds, the persisted occurrence identity
	Status      string
	ErrorCode   string
	NextDueAt   *int64
}

type ClaimService struct {
	db  *bun.DB
	log *diagnostic.Logger
	now func() time.Time
}

func NewClaimService(db *bun.DB, log *diagnostic.Logger) *ClaimService {
	return &ClaimService{db: db, log: log, now: time.Now}
}

// Claim atomically consumes precisely the expected persisted occurrence. The
// caller must pass the scheduled timestamp (not the callback arrival time).
// A successful waiting row may only be handed to the executor after commit.
// A second callback, disabled task, deleted room or early timer is a no-op.
// E3 will handle overdue occurrences and offline cursor fast-forwarding.
func (s *ClaimService) Claim(ctx context.Context, taskID, scheduledAt int64) (ClaimResult, error) {
	if taskID <= 0 || scheduledAt <= 0 {
		return ClaimResult{}, ErrInvalidOccurrence
	}
	now := s.now().UTC()
	var result ClaimResult
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		enabled, err := settings.SchedulingEnabled(ctx, tx)
		if err != nil {
			return err
		}
		if !enabled {
			return nil
		}
		var kind schedule.Kind
		var timezone, localTime, name, roomID string
		var localDate sql.NullString
		var weekdays sql.NullInt64
		var due sql.NullInt64
		err = tx.QueryRowContext(ctx, `
			SELECT t.schedule_kind, t.timezone, t.local_date, t.local_time,
				t.weekdays, t.next_due_at, t.name, r.platform_room_id
			FROM tasks t JOIN rooms r ON r.id = t.room_id
			WHERE t.id = ? AND t.enabled = 1 AND t.deleted_at IS NULL
				AND r.deleted_at IS NULL`, taskID).Scan(
			&kind, &timezone, &localDate, &localTime, &weekdays, &due, &name, &roomID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("load scheduled task: %w", err)
		}
		if !due.Valid || due.Int64 != scheduledAt || now.Before(time.UnixMilli(scheduledAt)) {
			return nil
		}

		var next *int64
		if kind != schedule.Once {
			rule := schedule.Rule{Kind: kind, Timezone: timezone, LocalDate: localDate.String,
				LocalTime: localTime, Weekdays: int(weekdays.Int64)}
			instant, ok, err := schedule.NextAfter(rule, time.UnixMilli(scheduledAt))
			if err != nil {
				return fmt.Errorf("calculate next scheduled occurrence: %w", err)
			}
			if !ok {
				return fmt.Errorf("recurring task has no next occurrence")
			}
			value := instant.UTC().UnixMilli()
			next = &value
		}

		var activeID int64
		err = tx.QueryRowContext(ctx, `
			SELECT id FROM executions WHERE task_id = ?
				AND status IN ('waiting', 'queued', 'running') LIMIT 1`, taskID).Scan(&activeID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check scheduled execution conflict: %w", err)
		}

		updated, err := tx.ExecContext(ctx, `
			UPDATE tasks SET next_due_at = ?, updated_at = ?
			WHERE id = ? AND next_due_at = ? AND enabled = 1 AND deleted_at IS NULL`,
			next, now.UnixMilli(), taskID, scheduledAt)
		if err != nil {
			return fmt.Errorf("advance scheduled cursor: %w", err)
		}
		affected, err := updated.RowsAffected()
		if err != nil {
			return fmt.Errorf("check scheduled cursor update: %w", err)
		}
		if affected != 1 {
			return nil
		}

		status, errorCode := "waiting", ""
		var finishedAt any
		if activeID != 0 {
			status, errorCode = "missed", "task_busy"
			finishedAt = now.UnixMilli()
		}
		var executionID int64
		err = tx.QueryRowContext(ctx, `
			INSERT INTO executions (task_id, trigger_kind, scheduled_at, status,
				created_at, finished_at, error_code, task_name_snapshot, platform_room_id_snapshot)
			VALUES (?, 'scheduled', ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
			taskID, scheduledAt, status, now.UnixMilli(), finishedAt,
			nullableCode(errorCode), name, roomID).Scan(&executionID)
		if err != nil {
			// Includes the persistent (task_id, scheduled_at) unique constraint.
			// Rolling back also restores the cursor; no partial claim is possible.
			return fmt.Errorf("register scheduled execution: %w", err)
		}
		result = ClaimResult{Claimed: true, ExecutionID: executionID,
			ScheduledAt: scheduledAt, Status: status, ErrorCode: errorCode, NextDueAt: next}
		return nil
	})
	if err != nil {
		return ClaimResult{}, err
	}
	if result.Claimed && s.log != nil {
		event := diagnostic.ScheduleClaimed
		if result.Status == "missed" {
			event = diagnostic.ScheduleConflict
		}
		s.log.RecordSchedule(event, result.ExecutionID, taskID, scheduledAt)
	}
	return result, nil
}

func nullableCode(code string) any {
	if code == "" {
		return nil
	}
	return code
}
