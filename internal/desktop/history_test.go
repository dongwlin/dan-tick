package desktop

import (
	"context"
	"encoding/json"
	"math"
	"path/filepath"
	"testing"

	"github.com/dongwlin/dan-tick/internal/history"
	"github.com/dongwlin/dan-tick/internal/storage"
	"github.com/stretchr/testify/require"
)

func TestHistoryBindingPaginationPrivacyAndReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "history.db")
	db, err := storage.Open(ctx, path)
	require.NoError(t, err)
	var taskID, roomID int64
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO rooms (platform_room_id, created_at, updated_at)
		VALUES ('9988', 1, 1) RETURNING id`).Scan(&roomID))
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO tasks
		(room_id, name, kind, schedule_kind, local_time, created_at, updated_at)
		VALUES (?, 'changed later', 'single', 'daily', '08:00:00', 1, 1) RETURNING id`, roomID).Scan(&taskID))
	var oldID, newID int64
	for i, id := range []*int64{&oldID, &newID} {
		require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO executions
			(task_id, trigger_kind, status, created_at, finished_at, error_code,
			 error_message, task_name_snapshot, platform_room_id_snapshot)
			VALUES (?, 'manual', 'failed', ?, ?, 'untrusted code', 'private upstream response', 'original task', '9988')
			RETURNING id`, taskID, 1000+i, 1100+i).Scan(id))
	}
	_, err = db.ExecContext(ctx, `INSERT INTO send_attempts
		(execution_id, sequence, message_no, content_snapshot, attempted_at, result, error_code)
		VALUES (?, 1, 2, 'original message', 1234, 'unknown', 'unsafe upstream code')`, oldID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE tasks SET name = 'new name', deleted_at = 12 WHERE id = ?`, taskID)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	reopened, err := storage.Open(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	svc := NewHistoryService(history.NewService(reopened))
	first, err := svc.ListExecutionHistory(ctx, 1, 1)
	require.NoError(t, err)
	require.EqualValues(t, 2, first.Total)
	require.EqualValues(t, 2, first.TotalPages)
	require.Equal(t, newID, first.Items[0].ID)
	require.Equal(t, "original task", first.Items[0].TaskNameSnapshot)
	second, err := svc.ListExecutionHistory(ctx, 2, 1)
	require.NoError(t, err)
	require.Equal(t, oldID, second.Items[0].ID)
	end, err := svc.ListExecutionHistory(ctx, 3, 1)
	require.NoError(t, err)
	require.NotNil(t, end.Items)
	require.Empty(t, end.Items)
	detail, err := svc.GetExecutionHistory(ctx, oldID)
	require.NoError(t, err)
	require.Equal(t, "original message", detail.Attempts[0].ContentSnapshot)
	require.EqualValues(t, 2, detail.Attempts[0].MessageNo)
	require.Equal(t, "unknown_error", *detail.Execution.ErrorCode)
	require.Equal(t, "unknown_error", *detail.Attempts[0].ErrorCode)
	encoded, err := json.Marshal(detail)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "private upstream response")
	require.NotContains(t, string(encoded), "unsafe upstream code")
	require.NotContains(t, string(encoded), "new name")
	for _, page := range []struct{ number, size int }{{0, 1}, {1, 0}, {1, 101}, {math.MaxInt, 100}} {
		_, err := svc.ListExecutionHistory(ctx, page.number, page.size)
		assertPublicError(t, err, "invalid_input", "private upstream response")
	}
	_, err = svc.GetExecutionHistory(ctx, 999999)
	assertPublicError(t, err, "not_found", "private upstream response")
	_, err = svc.GetExecutionHistory(ctx, -1)
	assertPublicError(t, err, "not_found", "private upstream response")

	// A canceled/database-failure path must not leak lower-level SQL messages.
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = svc.ListExecutionHistory(cancelled, 1, 1)
	assertPublicError(t, err, "cancelled", "private upstream response")
}
