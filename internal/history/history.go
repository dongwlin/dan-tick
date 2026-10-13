// Package history reads persisted execution history. It never sends messages
// or reconstructs historical data from mutable task definitions.
package history

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/uptrace/bun"
)

var (
	ErrInvalidPage = errors.New("invalid execution history page")
	ErrNotFound    = errors.New("execution not found")
)

const MaxPageSize = 100

// Execution uses the snapshots stored at execution time, even if the task or
// room has subsequently been renamed or logically deleted. TaskID is nil for
// test sends, which have no owning task. Times are UTC Unix milliseconds; nil
// means the corresponding lifecycle stage did not occur.
type Execution struct {
	ID                     int64   `json:"id"`
	TaskID                 *int64  `json:"task_id"`
	TriggerKind            string  `json:"trigger_kind"`
	ScheduledAt            *int64  `json:"scheduled_at"`
	Status                 string  `json:"status"`
	CreatedAt              int64   `json:"created_at"`
	QueuedAt               *int64  `json:"queued_at"`
	StartedAt              *int64  `json:"started_at"`
	FinishedAt             *int64  `json:"finished_at"`
	SentCount              int64   `json:"sent_count"`
	FailedCount            int64   `json:"failed_count"`
	ErrorCode              *string `json:"error_code"`
	TaskNameSnapshot       string  `json:"task_name_snapshot"`
	PlatformRoomIDSnapshot string  `json:"platform_room_id_snapshot"`
}

// Attempt is shown only for a selected execution. ContentSnapshot is user
// business data, never suitable for diagnostic logs or automatic export.
type Attempt struct {
	ID              int64   `json:"id"`
	ExecutionID     int64   `json:"execution_id"`
	Sequence        int64   `json:"sequence"`
	MessageNo       int64   `json:"message_no"`
	ContentSnapshot string  `json:"content_snapshot"`
	AttemptedAt     int64   `json:"attempted_at"`
	Result          string  `json:"result"`
	ErrorCode       *string `json:"error_code"`
}

type Page struct {
	Total int64       `json:"total"`
	Items []Execution `json:"items"`
}

type Detail struct {
	Execution Execution `json:"execution"`
	Attempts  []Attempt `json:"attempts"`
}

type Service struct{ db *bun.DB }

func NewService(db *bun.DB) *Service { return &Service{db: db} }

// List returns newest executions first, with ID as a stable tie-breaker.
// The caller controls page capacity (e.g. from available UI height); P0 does
// not require the multi-criteria filters planned for P1.
func (s *Service) List(ctx context.Context, offset, limit int) (Page, error) {
	if offset < 0 || limit < 1 || limit > MaxPageSize {
		return Page{}, ErrInvalidPage
	}
	page := Page{Items: make([]Execution, 0)}
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM executions`).Scan(&page.Total); err != nil {
			return fmt.Errorf("count execution history: %w", err)
		}
		rows, err := tx.QueryContext(ctx, `
			SELECT id, task_id, trigger_kind, scheduled_at, status, created_at,
				queued_at, started_at, finished_at, sent_count, failed_count,
				error_code, task_name_snapshot, platform_room_id_snapshot
			FROM executions ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, limit, offset)
		if err != nil {
			return fmt.Errorf("list execution history: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var item Execution
			if err := scanExecution(rows, &item); err != nil {
				return fmt.Errorf("read execution history: %w", err)
			}
			page.Items = append(page.Items, item)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate execution history: %w", err)
		}
		return nil
	})
	if err != nil {
		return Page{}, err
	}
	return page, nil
}

// Get loads an execution and its attempts as one database snapshot. Unknown
// IDs return ErrNotFound; executions without attempts have an empty slice.
func (s *Service) Get(ctx context.Context, id int64) (Detail, error) {
	if id <= 0 {
		return Detail{}, ErrNotFound
	}
	detail := Detail{Attempts: make([]Attempt, 0)}
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		err := scanExecution(tx.QueryRowContext(ctx, `
			SELECT id, task_id, trigger_kind, scheduled_at, status, created_at,
				queued_at, started_at, finished_at, sent_count, failed_count,
				error_code, task_name_snapshot, platform_room_id_snapshot
			FROM executions WHERE id = ?`, id), &detail.Execution)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("read execution detail: %w", err)
		}
		rows, err := tx.QueryContext(ctx, `
			SELECT id, execution_id, sequence, message_no, content_snapshot,
				attempted_at, result, error_code
			FROM send_attempts WHERE execution_id = ? ORDER BY sequence ASC, id ASC`, id)
		if err != nil {
			return fmt.Errorf("list execution attempts: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var attempt Attempt
			if err := rows.Scan(&attempt.ID, &attempt.ExecutionID, &attempt.Sequence,
				&attempt.MessageNo, &attempt.ContentSnapshot, &attempt.AttemptedAt,
				&attempt.Result, &attempt.ErrorCode); err != nil {
				return fmt.Errorf("read execution attempt: %w", err)
			}
			attempt.ErrorCode = safeCode(attempt.ErrorCode)
			detail.Attempts = append(detail.Attempts, attempt)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate execution attempts: %w", err)
		}
		return nil
	})
	if err != nil {
		return Detail{}, err
	}
	return detail, nil
}

type scanner interface{ Scan(...any) error }

func scanExecution(row scanner, item *Execution) error {
	if err := row.Scan(&item.ID, &item.TaskID, &item.TriggerKind, &item.ScheduledAt,
		&item.Status, &item.CreatedAt, &item.QueuedAt, &item.StartedAt,
		&item.FinishedAt, &item.SentCount, &item.FailedCount, &item.ErrorCode,
		&item.TaskNameSnapshot, &item.PlatformRoomIDSnapshot); err != nil {
		return err
	}
	item.ErrorCode = safeCode(item.ErrorCode)
	return nil
}

// Expose only the controlled categories used by the executor and recovery
// services. Never echo the stored free-text error_message (which might contain
// old or unredacted upstream data), or an unexpected error_code. UI can map
// these stable categories to localized explanations.
func safeCode(code *string) *string {
	if code == nil || *code == "" {
		return nil
	}
	switch *code {
	case "credential_missing", "credential_unavailable", "credential_corrupt",
		"authentication", "rate_limited", "bad_request", "too_long", "rejected",
		"invalid_input", "timeout", "cancelled", "transport", "http_status",
		"invalid_response", "uncertain", "send_failure", "persistence",
		"wait_failure", "user_stopped", "task_busy", "schedule_overdue",
		"recovery_unknown", "recovery_interrupted", "handoff_failed":
		return code
	default:
		unknown := "unknown_error"
		return &unknown
	}
}
