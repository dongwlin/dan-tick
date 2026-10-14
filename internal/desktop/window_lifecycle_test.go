package desktop

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/dongwlin/dan-tick/internal/settings"
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

func TestCloseRequestNeverHidesAWindowTheUserHasNotSeen(t *testing.T) {
	hidden := NewWindowLifecycle(nil, nil, settings.CloseHide, true)
	require.True(t, hidden.ShouldStartHidden(), "the stored preference governs window creation")
	require.False(t, hidden.closeWindow(), "closing a window that was never shown must quit")

	// The tray and a second launch restore the window through the same path.
	hidden.MarkShown()
	require.True(t, hidden.closeWindow(), "the background close behavior applies once the window is visible")

	visible := NewWindowLifecycle(nil, nil, settings.CloseQuit, false)
	require.False(t, visible.ShouldStartHidden())
	require.False(t, visible.closeWindow(), "closing quits when the user chose to exit")
}

func TestExplicitQuitIsNotTurnedIntoAHiddenWindow(t *testing.T) {
	lifecycle := NewWindowLifecycle(nil, nil, settings.CloseHide, false)
	lifecycle.MarkShown()
	lifecycle.exiting.Store(true)
	require.False(t, lifecycle.closeWindow())
}
