package executor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dongwlin/dan-tick/internal/task"
	"github.com/uptrace/bun"
)

// loadPlan is shared by manual registration and scheduled takeover. Enabling
// governs future scheduling; a disabled task may still execute a claimed run.
func loadPlan(ctx context.Context, tx bun.Tx, taskID int64) (runPlan, string, error) {
	var plan runPlan
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
		return runPlan{}, "", task.ErrNotFound
	}
	if err != nil {
		return runPlan{}, "", fmt.Errorf("load execution task: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT content FROM task_messages WHERE task_id = ? ORDER BY position, id`, taskID)
	if err != nil {
		return runPlan{}, "", fmt.Errorf("load execution messages: %w", err)
	}
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			rows.Close()
			return runPlan{}, "", fmt.Errorf("read execution message: %w", err)
		}
		plan.messages = append(plan.messages, content)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return runPlan{}, "", fmt.Errorf("read execution messages: %w", err)
	}
	switch kind {
	case task.Single:
		if len(plan.messages) != 1 {
			return runPlan{}, "", task.ErrInvalidTask
		}
	case task.Loop:
		if len(plan.messages) == 0 || !interval.Valid || interval.Int64 <= 0 ||
			(!maxCount.Valid && !duration.Valid) ||
			(maxCount.Valid && maxCount.Int64 <= 0) ||
			(duration.Valid && duration.Int64 <= 0) ||
			interval.Int64 > int64((1<<63-1)/int64(time.Second)) ||
			(duration.Valid && duration.Int64 > int64((1<<63-1)/int64(time.Second))) {
			return runPlan{}, "", task.ErrInvalidTask
		}
		plan.interval = time.Duration(interval.Int64) * time.Second
		if maxCount.Valid {
			plan.maxCount = &maxCount.Int64
		}
		if duration.Valid {
			plan.duration = time.Duration(duration.Int64) * time.Second
		}
	default:
		return runPlan{}, "", task.ErrInvalidTask
	}
	return plan, name, nil
}
