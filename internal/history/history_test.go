package history

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/dongwlin/dan-tick/internal/schedule"
	"github.com/dongwlin/dan-tick/internal/storage"
	"github.com/dongwlin/dan-tick/internal/task"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func historyDB(t *testing.T) (*bun.DB, int64) {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "history.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	var roomID int64
	require.NoError(t, db.QueryRowContext(context.Background(), `
		INSERT INTO rooms (platform_room_id, created_at, updated_at)
		VALUES ('13579', 1, 1) RETURNING id`).Scan(&roomID))
	created, err := task.NewService(db).Create(context.Background(), task.CreateInput{
		RoomID: roomID, Name: "Original task", Kind: task.Single,
		Messages: []string{"Original message"},
		Schedule: schedule.Rule{Kind: schedule.Daily, LocalTime: "08:00:00"},
		Enabled:  true,
	})
	require.NoError(t, err)
	return db, created.ID
}

func insertExecution(t *testing.T, db *bun.DB, taskID, created int64, trigger, status, code string) int64 {
	t.Helper()
	var scheduled, finished any
	if trigger == "scheduled" {
		scheduled = created - 100
	}
	if status != "queued" && status != "waiting" && status != "running" {
		finished = created + 100
	}
	var id int64
	err := db.QueryRowContext(context.Background(), `
		INSERT INTO executions (task_id, trigger_kind, scheduled_at, status,
			created_at, finished_at, sent_count, failed_count, error_code,
			task_name_snapshot, platform_room_id_snapshot)
		VALUES (?, ?, ?, ?, ?, ?, 3, 2, ?, 'Old task snapshot', '13579')
		RETURNING id`, taskID, trigger, scheduled, status, created, finished, nullString(code)).Scan(&id)
	require.NoError(t, err)
	return id
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func TestListPagesWithStableOrderAndNullableTimes(t *testing.T) {
	db, taskID := historyDB(t)
	ctx := context.Background()
	svc := NewService(db)
	ids := []int64{
		insertExecution(t, db, taskID, 1000, "manual", "completed", ""),
		insertExecution(t, db, taskID, 1000, "scheduled", "missed", "schedule_overdue"),
		insertExecution(t, db, taskID, 1500, "manual", "failed", "timeout"),
		insertExecution(t, db, taskID, 1800, "manual", "stopped", "user_stopped"),
		insertExecution(t, db, taskID, 1900, "scheduled", "waiting", ""),
	}
	first, err := svc.List(ctx, 0, 2)
	require.NoError(t, err)
	require.Equal(t, int64(5), first.Total)
	require.Len(t, first.Items, 2)
	require.Equal(t, []int64{ids[4], ids[3]}, []int64{first.Items[0].ID, first.Items[1].ID})
	require.Nil(t, first.Items[0].StartedAt)
	require.Nil(t, first.Items[0].FinishedAt)
	require.Nil(t, first.Items[1].ScheduledAt)
	require.Equal(t, int64(1900), *first.Items[1].FinishedAt)
	require.Equal(t, "user_stopped", *first.Items[1].ErrorCode)

	second, err := svc.List(ctx, 2, 2)
	require.NoError(t, err)
	require.Equal(t, int64(5), second.Total)
	require.Equal(t, []int64{ids[2], ids[1]}, []int64{second.Items[0].ID, second.Items[1].ID})
	require.Equal(t, int64(900), *second.Items[1].ScheduledAt)
	require.Equal(t, "schedule_overdue", *second.Items[1].ErrorCode)
	last, err := svc.List(ctx, 4, 2)
	require.NoError(t, err)
	require.Len(t, last.Items, 1)
	require.Equal(t, ids[0], last.Items[0].ID)
	end, err := svc.List(ctx, 10, 2)
	require.NoError(t, err)
	require.Equal(t, int64(5), end.Total)
	require.Empty(t, end.Items)
	require.NotNil(t, end.Items)
	for _, pair := range [][2]int{{-1, 2}, {0, 0}, {0, -1}, {0, MaxPageSize + 1}} {
		_, err := svc.List(ctx, pair[0], pair[1])
		require.ErrorIs(t, err, ErrInvalidPage)
	}
	_, err = svc.Get(ctx, -1)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = svc.Get(ctx, 100000)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestGetRetainsSnapshotsAttemptOrderAndSafeErrorCodesAfterTaskChanges(t *testing.T) {
	db, taskID := historyDB(t)
	ctx := context.Background()
	id := insertExecution(t, db, taskID, 1000, "manual", "failed", "authentication")
	// Deliberately insert out of sequence, including two attempts for the same
	// logical message. Their chronology comes from sequence, not row ID or time.
	for _, row := range []struct {
		sequence, messageNo, at int64
		content, result, code   string
	}{
		{3, 2, 800, "Old second message", "unknown", "recovery_unknown"},
		{1, 1, 900, "Old first message", "failed", "rate_limited"},
		{2, 1, 1000, "Old first message", "accepted", ""},
	} {
		_, err := db.ExecContext(ctx, `
			INSERT INTO send_attempts (execution_id, sequence, message_no,
				content_snapshot, attempted_at, result, error_code)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, id, row.sequence, row.messageNo,
			row.content, row.at, row.result, nullString(row.code))
		require.NoError(t, err)
	}
	// Even a legacy free-text database error is not automatically safe to expose.
	_, err := db.ExecContext(ctx, `UPDATE executions SET error_message = 'secret raw response' WHERE id = ?`, id)
	require.NoError(t, err)
	_, err = task.NewService(db).Update(ctx, taskID, task.CreateInput{
		RoomID: 1, Name: "Renamed task", Kind: task.Single,
		Messages: []string{"New private message"},
		Schedule: schedule.Rule{Kind: schedule.Daily, LocalTime: "08:00:00"},
		Enabled:  true,
	})
	require.NoError(t, err)
	require.NoError(t, task.NewService(db).Delete(ctx, taskID))
	_, err = db.ExecContext(ctx, `UPDATE rooms SET platform_room_id = '24680', deleted_at = 20 WHERE id = 1`)
	require.NoError(t, err)

	detail, err := NewService(db).Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "Old task snapshot", detail.Execution.TaskNameSnapshot)
	require.Equal(t, "13579", detail.Execution.PlatformRoomIDSnapshot)
	require.Equal(t, "authentication", *detail.Execution.ErrorCode)
	require.Equal(t, int64(3), detail.Execution.SentCount)
	require.Equal(t, int64(2), detail.Execution.FailedCount)
	require.Len(t, detail.Attempts, 3)
	require.Equal(t, []int64{1, 2, 3}, []int64{detail.Attempts[0].Sequence, detail.Attempts[1].Sequence, detail.Attempts[2].Sequence})
	require.Equal(t, []int64{1, 1, 2}, []int64{detail.Attempts[0].MessageNo, detail.Attempts[1].MessageNo, detail.Attempts[2].MessageNo})
	require.Equal(t, []string{"failed", "accepted", "unknown"}, []string{detail.Attempts[0].Result, detail.Attempts[1].Result, detail.Attempts[2].Result})
	require.Equal(t, "Old first message", detail.Attempts[0].ContentSnapshot)
	require.Equal(t, int64(800), detail.Attempts[2].AttemptedAt)
	require.Equal(t, "rate_limited", *detail.Attempts[0].ErrorCode)
	require.Nil(t, detail.Attempts[1].ErrorCode)
	require.Equal(t, "recovery_unknown", *detail.Attempts[2].ErrorCode)

	listed, err := NewService(db).List(ctx, 0, 5)
	require.NoError(t, err)
	require.Equal(t, "Old task snapshot", listed.Items[0].TaskNameSnapshot)
	encoded, err := json.Marshal(detail)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "secret raw response")
	require.NotContains(t, string(encoded), "New private message")
	listJSON, err := json.Marshal(listed)
	require.NoError(t, err)
	require.NotContains(t, string(listJSON), "Old first message")
	require.NotContains(t, string(listJSON), "secret raw response")
}

func TestGetEmptyAttemptsAndUnrecognizedErrorCategory(t *testing.T) {
	db, taskID := historyDB(t)
	id := insertExecution(t, db, taskID, 1000, "scheduled", "missed", "")
	detail, err := NewService(db).Get(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, detail.Attempts)
	require.Empty(t, detail.Attempts)
	require.Nil(t, detail.Execution.ErrorCode)

	_, err = db.ExecContext(context.Background(), `UPDATE executions SET error_code = 'private token abc' WHERE id = ?`, id)
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(), `
		INSERT INTO send_attempts (execution_id, sequence, message_no,
			content_snapshot, attempted_at, result, error_code)
		VALUES (?, 1, 1, 'message', 1200, 'unknown', 'upstream secret')`, id)
	require.NoError(t, err)
	detail, err = NewService(db).Get(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, "unknown_error", *detail.Execution.ErrorCode)
	require.Equal(t, "unknown_error", *detail.Attempts[0].ErrorCode)
	encoded, err := json.Marshal(detail)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "private token abc")
	require.NotContains(t, string(encoded), "upstream secret")
}

func TestHistoryPersistsAcrossReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "persist.db")
	db, err := storage.Open(ctx, path)
	require.NoError(t, err)
	var roomID, taskID, id int64
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO rooms (platform_room_id, created_at, updated_at)
		VALUES ('98765', 1, 1) RETURNING id`).Scan(&roomID))
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO tasks (room_id, name, kind, schedule_kind,
		local_time, created_at, updated_at) VALUES (?, 'persisted', 'single', 'daily', '08:00:00', 1, 1)
		RETURNING id`, roomID).Scan(&taskID))
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO executions
		(task_id, trigger_kind, status, created_at, finished_at, task_name_snapshot, platform_room_id_snapshot)
		VALUES (?, 'manual', 'completed', 1, 2, 'persisted', '98765') RETURNING id`, taskID).Scan(&id))
	require.NoError(t, db.Close())
	reopened, err := storage.Open(ctx, path)
	require.NoError(t, err)
	defer func() { require.NoError(t, reopened.Close()) }()
	page, err := NewService(reopened).List(ctx, 0, 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), page.Total)
	require.Equal(t, id, page.Items[0].ID)
	detail, err := NewService(reopened).Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "completed", detail.Execution.Status)
	require.Empty(t, detail.Attempts)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = NewService(reopened).List(cancelled, 0, 1)
	require.True(t, errors.Is(err, context.Canceled))
}
