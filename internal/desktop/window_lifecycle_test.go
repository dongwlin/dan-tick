package desktop

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/dongwlin/dan-tick/internal/storage"
	"github.com/stretchr/testify/require"
)

func TestExitWarningCountsEnabledTasksAndRunningExecutions(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "window.db"))
	require.NoError(t, err)
	defer db.Close()
	needs, err := needsExitWarning(ctx, db)
	require.NoError(t, err)
	require.False(t, needs)

	_, err = db.ExecContext(ctx, `INSERT INTO rooms (id, platform_room_id, created_at, updated_at)
		VALUES (1, '123', 1, 1)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO tasks
		(id, room_id, name, kind, enabled, schedule_kind, local_time, created_at, updated_at)
		VALUES (1, 1, 'test', 'single', 0, 'daily', '09:00', 1, 1)`)
	require.NoError(t, err)
	needs, err = needsExitWarning(ctx, db)
	require.NoError(t, err)
	require.False(t, needs)

	_, err = db.ExecContext(ctx, "UPDATE tasks SET enabled = 1 WHERE id = 1")
	require.NoError(t, err)
	needs, err = needsExitWarning(ctx, db)
	require.NoError(t, err)
	require.True(t, needs)
	_, err = db.ExecContext(ctx, "UPDATE tasks SET enabled = 0 WHERE id = 1")
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `INSERT INTO executions
		(task_id, trigger_kind, status, created_at, task_name_snapshot, platform_room_id_snapshot)
		VALUES (1, 'manual', 'running', 1, 'test', '123')`)
	require.NoError(t, err)
	needs, err = needsExitWarning(ctx, db)
	require.NoError(t, err)
	require.True(t, needs)
	_, err = db.ExecContext(ctx, "UPDATE executions SET status = 'stopped' WHERE task_id = 1")
	require.NoError(t, err)
	needs, err = needsExitWarning(ctx, db)
	require.NoError(t, err)
	require.False(t, needs)
}
