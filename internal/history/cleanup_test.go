package history

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/dongwlin/dan-tick/internal/storage"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func TestCleanupExpiredPreservesActiveRecentAndTaskData(t *testing.T) {
	db, taskID := historyDB(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-Retention).UnixMilli()
	service := NewService(db)

	// All terminal kinds are eligible. The cutoff is based on finished_at,
	// not creation or scheduled time, and equality is still retained.
	var removed []int64
	for i, status := range []string{"completed", "stopped", "failed", "missed", "interrupted"} {
		trigger := "manual"
		if status == "missed" {
			trigger = "scheduled"
		}
		id := insertExecution(t, db, taskID, cutoff-10000-int64(i)*1000, trigger, status, "")
		removed = append(removed, id)
	}
	boundary := insertExecution(t, db, taskID, cutoff-1000, "manual", "completed", "")
	updateFinishedAt(t, db, boundary, cutoff)
	recentlyFinished := insertExecution(t, db, taskID, cutoff-5000, "manual", "completed", "")
	updateFinishedAt(t, db, recentlyFinished, cutoff+1)
	// Even a recently created execution is eligible if it finished long ago.
	lateCreated := insertExecution(t, db, taskID, cutoff+10000, "manual", "failed", "")
	updateFinishedAt(t, db, lateCreated, cutoff-1)
	removed = append(removed, lateCreated)
	withoutFinishedAt := insertExecution(t, db, taskID, cutoff-2000, "manual", "failed", "")
	_, err := db.ExecContext(ctx, `UPDATE executions SET finished_at = NULL WHERE id = ?`, withoutFinishedAt)
	require.NoError(t, err)

	var active []int64
	for i, status := range []string{"waiting", "queued", "running"} {
		// Active executions have a per-task uniqueness constraint.
		var otherTaskID int64
		require.NoError(t, db.QueryRowContext(ctx, `
			INSERT INTO tasks (room_id, name, kind, schedule_kind, local_time, created_at, updated_at)
			VALUES (1, ?, 'single', 'daily', '08:00:00', 1, 1) RETURNING id`, status).Scan(&otherTaskID))
		id := insertExecution(t, db, otherTaskID, cutoff-50000-int64(i)*1000, "manual", status, "")
		// A corrupt/legacy active row with an old finished_at must still survive.
		updateFinishedAt(t, db, id, cutoff-100)
		active = append(active, id)
	}

	retained := append([]int64{boundary, recentlyFinished, withoutFinishedAt}, active...)
	for _, id := range append(append([]int64{}, removed...), retained...) {
		_, err := db.ExecContext(ctx, `INSERT INTO send_attempts
			(execution_id, sequence, message_no, content_snapshot, attempted_at, result)
			VALUES (?, 1, 1, 'private text', 1, 'accepted')`, id)
		require.NoError(t, err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO app_settings (key, value) VALUES ('timezone', 'Asia/Shanghai')`)
	require.NoError(t, err)
	var dueBefore int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT next_due_at FROM tasks WHERE id = ?`, taskID).Scan(&dueBefore))

	count, err := service.CleanupExpired(ctx, now.In(time.FixedZone("UTC+8", 8*3600)))
	require.NoError(t, err)
	require.Equal(t, int64(len(removed)), count)
	for _, id := range removed {
		require.Equal(t, int64(0), countByExecution(t, db, "executions", id))
		require.Equal(t, int64(0), countByExecution(t, db, "send_attempts", id))
	}
	for _, id := range retained {
		require.Equal(t, int64(1), countByExecution(t, db, "executions", id))
		require.Equal(t, int64(1), countByExecution(t, db, "send_attempts", id))
	}
	var tasks, messages int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM tasks`).Scan(&tasks))
	require.Equal(t, int64(4), tasks)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM task_messages`).Scan(&messages))
	require.Equal(t, int64(1), messages)
	var dueAfter int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT next_due_at FROM tasks WHERE id = ?`, taskID).Scan(&dueAfter))
	require.Equal(t, dueBefore, dueAfter)
	var setting string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = 'timezone'`).Scan(&setting))
	require.Equal(t, "Asia/Shanghai", setting)

	count, err = service.CleanupExpired(ctx, now)
	require.NoError(t, err)
	require.Zero(t, count, "repeat cleanup must be idempotent")
}

func TestCleanupExpiredFailureRollsBackAndRetryAfterReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "retention.db")
	db, err := storage.Open(ctx, path)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO rooms (id, platform_room_id, created_at, updated_at) VALUES (1, '42', 1, 1)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO tasks (id, room_id, name, kind, schedule_kind, local_time, created_at, updated_at)
		VALUES (1, 1, 'keep task', 'single', 'daily', '12:00:00', 1, 1)`)
	require.NoError(t, err)
	ids := []int64{
		insertExecution(t, db, 1, 1, "manual", "completed", ""),
		insertExecution(t, db, 1, 2, "manual", "failed", ""),
	}
	for _, id := range ids {
		_, err := db.ExecContext(ctx, `INSERT INTO send_attempts
			(execution_id, sequence, message_no, content_snapshot, attempted_at, result)
			VALUES (?, 1, 1, 'message', 1, 'accepted')`, id)
		require.NoError(t, err)
	}
	_, err = db.ExecContext(ctx, fmt.Sprintf(`CREATE TRIGGER fail_cleanup BEFORE DELETE ON executions
		WHEN OLD.id = %d BEGIN SELECT RAISE(ABORT, 'simulated cleanup failure'); END`, ids[1]))
	require.NoError(t, err)
	_, err = NewService(db).CleanupExpired(ctx, time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC))
	require.Error(t, err)
	for _, id := range ids {
		require.Equal(t, int64(1), countByExecution(t, db, "executions", id))
		require.Equal(t, int64(1), countByExecution(t, db, "send_attempts", id))
	}
	_, err = db.ExecContext(ctx, `DROP TRIGGER fail_cleanup`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	db, err = storage.Open(ctx, path)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	deleted, err := NewService(db).CleanupExpired(ctx, time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, int64(2), deleted)
	for _, id := range ids {
		require.Zero(t, countByExecution(t, db, "executions", id))
		require.Zero(t, countByExecution(t, db, "send_attempts", id))
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = NewService(db).CleanupExpired(cancelled, time.Now())
	require.ErrorIs(t, err, context.Canceled)
}

func updateFinishedAt(t *testing.T, db *bun.DB, id, at int64) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `UPDATE executions SET finished_at = ? WHERE id = ?`, at, id)
	require.NoError(t, err)
}

func countByExecution(t *testing.T, db *bun.DB, table string, id int64) int64 {
	t.Helper()
	column := "id"
	if table == "send_attempts" {
		column = "execution_id"
	}
	var count int64
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM `+table+` WHERE `+column+` = ?`, id).Scan(&count))
	return count
}
