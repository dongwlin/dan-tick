package task

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/dongwlin/dan-tick/internal/schedule"
	"github.com/dongwlin/dan-tick/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func TestUpdateReplacesTaskAndOrderedMessages(t *testing.T) {
	s, db := newTestService(t)
	ctx := context.Background()
	firstRoom := addRoom(t, db, "5440")
	secondRoom := addRoom(t, db, "5441")
	created, err := s.Create(ctx, singleInput(firstRoom))
	require.NoError(t, err)
	other, err := s.Create(ctx, singleInput(firstRoom))
	require.NoError(t, err)
	input := CreateInput{
		RoomID: secondRoom, Name: "  changed title ", Kind: Loop,
		Messages:        []string{"  third  ", "", "second", "first"},
		IntervalSeconds: ptr(int64(15)), MaxDurationSeconds: ptr(int64(90)),
		Schedule: schedule.Rule{
			Kind: schedule.Weekly, Timezone: "America/New_York", LocalTime: "22:00:00", Weekdays: 5,
		},
		Enabled: true,
	}
	s.now = func() time.Time { return clockTime.Add(2 * time.Hour) }
	updated, err := s.Update(ctx, created.ID, input)
	require.NoError(t, err)
	assert.Equal(t, created.ID, updated.ID)
	assert.Equal(t, created.CreatedAt, updated.CreatedAt)
	assert.Equal(t, clockTime.Add(2*time.Hour).UnixMilli(), updated.UpdatedAt)
	assert.Equal(t, secondRoom, updated.RoomID)
	assert.Equal(t, "changed title", updated.Name)
	assert.Equal(t, Loop, updated.Kind)
	assert.Equal(t, int64(15), *updated.IntervalSeconds)
	assert.Equal(t, int64(90), *updated.MaxDurationSeconds)
	assert.Nil(t, updated.MaxCount)
	assert.Nil(t, updated.LocalDate)
	require.NotNil(t, updated.Weekdays)
	assert.Equal(t, 5, *updated.Weekdays)
	next, exists, err := schedule.NextAfter(input.Schedule, clockTime.Add(2*time.Hour))
	require.NoError(t, err)
	require.True(t, exists)
	require.NotNil(t, updated.NextDueAt)
	assert.Equal(t, next.UnixMilli(), *updated.NextDueAt)
	require.Len(t, updated.Messages, 3)
	for i, content := range []string{"third", "second", "first"} {
		assert.Equal(t, content, updated.Messages[i].Content)
		assert.Equal(t, i, updated.Messages[i].Position)
	}
	var messageCount int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_messages WHERE task_id = ?`, created.ID).Scan(&messageCount))
	assert.Equal(t, 3, messageCount, "old messages must be replaced, not appended")
	got, err := s.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, updated, got)
	filtered, err := s.List(ctx, firstRoom)
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, other.ID, filtered[0].ID, "other tasks must not change")
	filtered, err = s.List(ctx, secondRoom)
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, created.ID, filtered[0].ID)
	unchanged, err := s.Get(ctx, other.ID)
	require.NoError(t, err)
	assert.Equal(t, other, unchanged)
}

func TestUpdatePreservesPlanCursorIfRuleUnchanged(t *testing.T) {
	s, db := newTestService(t)
	ctx := context.Background()
	id := addRoom(t, db, "5440")
	created, err := s.Create(ctx, singleInput(id))
	require.NoError(t, err)
	// Scheduler may already have advanced the cursor beyond the current clock.
	advanced := clockTime.AddDate(0, 0, 10).UnixMilli()
	_, err = db.ExecContext(ctx, `UPDATE tasks SET next_due_at = ? WHERE id = ?`, advanced, created.ID)
	require.NoError(t, err)
	input := singleInput(id)
	input.Name = "renamed"
	input.Messages = []string{"replacement"}
	s.now = func() time.Time { return clockTime.Add(-time.Hour) }
	updated, err := s.Update(ctx, created.ID, input)
	require.NoError(t, err)
	require.NotNil(t, updated.NextDueAt)
	assert.Equal(t, advanced, *updated.NextDueAt, "editing content must not rewind the cursor")
	assert.Equal(t, "replacement", updated.Messages[0].Content)
	// Finished one-time plans must not be rearmed by a name/message edit.
	once := singleInput(id)
	once.Schedule = schedule.Rule{Kind: schedule.Once, Timezone: "Asia/Shanghai", LocalDate: "2026-10-15", LocalTime: "22:00:00"}
	s.now = func() time.Time { return clockTime }
	finished, err := s.Create(ctx, once)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE tasks SET next_due_at = NULL WHERE id = ?`, finished.ID)
	require.NoError(t, err)
	s.now = func() time.Time { return clockTime.Add(10 * 24 * time.Hour) }
	once.Name = "after completion"
	got, err := s.Update(ctx, finished.ID, once)
	require.NoError(t, err)
	assert.True(t, got.Enabled)
	assert.Nil(t, got.NextDueAt)
}

func TestUpdateValidationAndRoomChecksLeaveOriginalUntouched(t *testing.T) {
	s, db := newTestService(t)
	ctx := context.Background()
	roomID := addRoom(t, db, "5440")
	deletedRoom := addRoom(t, db, "5441")
	_, err := db.ExecContext(ctx, `UPDATE rooms SET deleted_at = ? WHERE id = ?`, clockTime.UnixMilli(), deletedRoom)
	require.NoError(t, err)
	created, err := s.Create(ctx, singleInput(roomID))
	require.NoError(t, err)
	for _, tc := range []struct {
		name string
		edit func(*CreateInput)
		want error
	}{
		{"invalid message", func(v *CreateInput) { v.Messages = []string{" ", " "} }, ErrInvalidTask},
		{"invalid loop", func(v *CreateInput) { v.Kind = Loop }, ErrInvalidTask},
		{"invalid timezone", func(v *CreateInput) { v.Schedule.Timezone = "Invalid/Zone" }, ErrInvalidTask},
		{"past plan", func(v *CreateInput) {
			v.Schedule = schedule.Rule{Kind: schedule.Once, Timezone: "Asia/Shanghai", LocalDate: "2026-10-10", LocalTime: "22:00:00"}
		}, ErrInvalidTask},
		{"missing room", func(v *CreateInput) { v.RoomID = 12345 }, ErrRoomUnavailable},
		{"deleted room", func(v *CreateInput) { v.RoomID = deletedRoom }, ErrRoomUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := singleInput(roomID)
			tc.edit(&input)
			_, err := s.Update(ctx, created.ID, input)
			require.ErrorIs(t, err, tc.want)
			got, err := s.Get(ctx, created.ID)
			require.NoError(t, err)
			assert.Equal(t, created, got)
		})
	}
	_, err = s.Update(ctx, 0, singleInput(roomID))
	require.ErrorIs(t, err, ErrNotFound)
	_, err = s.Update(ctx, 123456, singleInput(roomID))
	require.ErrorIs(t, err, ErrNotFound)
}

func TestUpdateMessageFailureRollsBackColumnsAndMessages(t *testing.T) {
	s, db := newTestService(t)
	ctx := context.Background()
	id := addRoom(t, db, "5440")
	created, err := s.Create(ctx, singleInput(id))
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		CREATE TRIGGER fail_update_message BEFORE INSERT ON task_messages
		WHEN NEW.position = 1 BEGIN SELECT RAISE(ABORT, 'test second message failure'); END`)
	require.NoError(t, err)
	input := singleInput(id)
	input.Kind = Loop
	input.Name = "should not be stored"
	input.Messages = []string{"first", "second"}
	input.IntervalSeconds = ptr(int64(15))
	input.MaxCount = ptr(int64(2))
	input.Schedule.LocalTime = "23:00:00"
	_, err = s.Update(ctx, created.ID, input)
	require.ErrorContains(t, err, "test second message failure")
	got, err := s.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created, got, "failed update must restore original fields and old messages")
}

func TestSetEnabledRecalculatesOnlyOnReenable(t *testing.T) {
	s, db := newTestService(t)
	ctx := context.Background()
	id := addRoom(t, db, "5440")
	created, err := s.Create(ctx, singleInput(id))
	require.NoError(t, err)
	s.now = func() time.Time { return clockTime.Add(4 * time.Hour) }
	disabled, err := s.SetEnabled(ctx, created.ID, false)
	require.NoError(t, err)
	assert.False(t, disabled.Enabled)
	assert.Nil(t, disabled.NextDueAt)
	assert.Equal(t, created.CreatedAt, disabled.CreatedAt)
	assert.Equal(t, clockTime.Add(4*time.Hour).UnixMilli(), disabled.UpdatedAt)
	assert.Equal(t, created.Messages, disabled.Messages)
	s.now = func() time.Time { return clockTime.Add(48 * time.Hour) }
	same, err := s.SetEnabled(ctx, created.ID, false)
	require.NoError(t, err)
	assert.Equal(t, disabled, same, "repeated disable must not move updated_at")
	reenabled, err := s.SetEnabled(ctx, created.ID, true)
	require.NoError(t, err)
	assert.True(t, reenabled.Enabled)
	require.NotNil(t, reenabled.NextDueAt)
	next, found, err := schedule.NextAfter(ruleFor(reenabled), clockTime.Add(48*time.Hour))
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, next.UnixMilli(), *reenabled.NextDueAt)
	assert.Greater(t, *reenabled.NextDueAt, *created.NextDueAt)
	unchanged, err := s.SetEnabled(ctx, created.ID, true)
	require.NoError(t, err)
	assert.Equal(t, reenabled, unchanged, "repeated enable must not reset the plan")
	_, err = s.SetEnabled(ctx, 0, false)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = s.SetEnabled(ctx, 12345, true)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestEnablePastOnceRejectedWithoutMutatingTask(t *testing.T) {
	s, db := newTestService(t)
	id := addRoom(t, db, "5440")
	input := singleInput(id)
	input.Enabled = false
	input.Schedule = schedule.Rule{Kind: schedule.Once, LocalDate: "2026-10-15", LocalTime: "22:00:00"}
	created, err := s.Create(context.Background(), input)
	require.NoError(t, err)
	s.now = func() time.Time { return clockTime.AddDate(0, 0, 10) }
	_, err = s.SetEnabled(context.Background(), created.ID, true)
	require.ErrorIs(t, err, ErrInvalidTask)
	got, err := s.Get(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, created, got)
}

func insertExecution(t *testing.T, db *bun.DB, taskID int64, status string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO executions (task_id, trigger_kind, status, created_at, finished_at,
			task_name_snapshot, platform_room_id_snapshot)
		VALUES (?, 'manual', ?, ?, ?, 'original snapshot', '5440')`, taskID, status,
		clockTime.UnixMilli(), completionTime(status))
	require.NoError(t, err)
}

func completionTime(status string) any {
	if status == "completed" || status == "stopped" {
		return clockTime.UnixMilli()
	}
	return nil
}

func TestActiveExecutionBlocksEditAndDeleteButNotDisabling(t *testing.T) {
	for _, status := range []string{"waiting", "queued", "running"} {
		t.Run(status, func(t *testing.T) {
			s, db := newTestService(t)
			ctx := context.Background()
			roomID := addRoom(t, db, "5440")
			created, err := s.Create(ctx, singleInput(roomID))
			require.NoError(t, err)
			insertExecution(t, db, created.ID, status)
			input := singleInput(roomID)
			input.Name = "cannot edit yet"
			_, err = s.Update(ctx, created.ID, input)
			require.ErrorIs(t, err, ErrTaskBusy)
			require.ErrorIs(t, s.Delete(ctx, created.ID), ErrTaskBusy)
			got, err := s.Get(ctx, created.ID)
			require.NoError(t, err)
			assert.Equal(t, created, got)
			disabled, err := s.SetEnabled(ctx, created.ID, false)
			require.NoError(t, err)
			assert.False(t, disabled.Enabled)
			assert.Nil(t, disabled.NextDueAt)
			var actualStatus string
			require.NoError(t, db.QueryRowContext(ctx, `SELECT status FROM executions WHERE task_id = ?`, created.ID).Scan(&actualStatus))
			assert.Equal(t, status, actualStatus, "disabling must not fake cancellation")
			_, err = db.ExecContext(ctx, `UPDATE executions SET status = 'stopped', finished_at = ? WHERE task_id = ?`, clockTime.UnixMilli(), created.ID)
			require.NoError(t, err)
			require.NoError(t, s.Delete(ctx, created.ID))
		})
	}
}

func TestDeleteRetainsHistoryAndMessages(t *testing.T) {
	s, db := newTestService(t)
	ctx := context.Background()
	roomID := addRoom(t, db, "5440")
	created, err := s.Create(ctx, singleInput(roomID))
	require.NoError(t, err)
	other, err := s.Create(ctx, singleInput(roomID))
	require.NoError(t, err)
	insertExecution(t, db, created.ID, "completed")
	require.NoError(t, s.Delete(ctx, created.ID))
	require.ErrorIs(t, s.Delete(ctx, created.ID), ErrNotFound)
	_, err = s.Get(ctx, created.ID)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = s.SetEnabled(ctx, created.ID, true)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = s.Update(ctx, created.ID, singleInput(roomID))
	require.ErrorIs(t, err, ErrNotFound)
	tasks, err := s.List(ctx, roomID)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, other.ID, tasks[0].ID)
	var enabled int
	var next, deleted sql.NullInt64
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT enabled, next_due_at, deleted_at FROM tasks WHERE id = ?`, created.ID).Scan(&enabled, &next, &deleted))
	assert.Zero(t, enabled)
	assert.False(t, next.Valid)
	assert.True(t, deleted.Valid)
	assert.Equal(t, clockTime.UnixMilli(), deleted.Int64)
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_messages WHERE task_id = ?`, created.ID).Scan(&count))
	assert.Equal(t, 1, count)
	var snapshotName, snapshotRoom string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT task_name_snapshot, platform_room_id_snapshot FROM executions WHERE task_id = ?`, created.ID).Scan(&snapshotName, &snapshotRoom))
	assert.Equal(t, "original snapshot", snapshotName)
	assert.Equal(t, "5440", snapshotRoom)
	require.ErrorIs(t, s.Delete(ctx, -1), ErrNotFound)
}

func TestUpdatesAndDeletePersistAcrossReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "changed-tasks.db")
	db, err := storage.Open(ctx, path)
	require.NoError(t, err)
	id := addRoom(t, db, "5440")
	s := NewService(db)
	s.now = func() time.Time { return clockTime }
	first, err := s.Create(ctx, singleInput(id))
	require.NoError(t, err)
	second, err := s.Create(ctx, singleInput(id))
	require.NoError(t, err)
	edit := singleInput(id)
	edit.Name = "persisted edit"
	edit.Kind = Loop
	edit.Messages = []string{"2", "1"}
	edit.IntervalSeconds = ptr(int64(15))
	edit.MaxCount = ptr(int64(2))
	updated, err := s.Update(ctx, first.ID, edit)
	require.NoError(t, err)
	updated, err = s.SetEnabled(ctx, first.ID, false)
	require.NoError(t, err)
	require.NoError(t, s.Delete(ctx, second.ID))
	require.NoError(t, db.Close())
	db, err = storage.Open(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	s = NewService(db)
	loaded, err := s.Get(ctx, first.ID)
	require.NoError(t, err)
	assert.Equal(t, updated, loaded)
	assert.Nil(t, loaded.NextDueAt)
	assert.Equal(t, []string{"2", "1"}, []string{loaded.Messages[0].Content, loaded.Messages[1].Content})
	_, err = s.Get(ctx, second.ID)
	require.ErrorIs(t, err, ErrNotFound)
	var retained int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE id = ? AND deleted_at IS NOT NULL`, second.ID).Scan(&retained))
	assert.Equal(t, 1, retained)
}

func TestStatusUpdateFailureDoesNotCommit(t *testing.T) {
	s, db := newTestService(t)
	ctx := context.Background()
	roomID := addRoom(t, db, "5440")
	created, err := s.Create(ctx, singleInput(roomID))
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		CREATE TRIGGER fail_status_change BEFORE UPDATE ON tasks
		BEGIN SELECT RAISE(ABORT, 'test update refused'); END`)
	require.NoError(t, err)
	_, err = s.SetEnabled(ctx, created.ID, false)
	require.ErrorContains(t, err, "test update refused")
	require.ErrorContains(t, s.Delete(ctx, created.ID), "test update refused")
	got, err := s.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created, got)
	_, err = s.Update(ctx, created.ID, singleInput(roomID))
	require.ErrorContains(t, err, "test update refused")
	got, err = s.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created, got)
}

func TestUpdateCanDisableAndReenableWithNewPlan(t *testing.T) {
	s, db := newTestService(t)
	ctx := context.Background()
	roomID := addRoom(t, db, "5440")
	created, err := s.Create(ctx, singleInput(roomID))
	require.NoError(t, err)
	input := singleInput(roomID)
	input.Enabled = false
	input.Schedule = schedule.Rule{Kind: schedule.Once, LocalDate: "2026-10-15", LocalTime: "22:00:00"}
	disabled, err := s.Update(ctx, created.ID, input)
	require.NoError(t, err)
	assert.False(t, disabled.Enabled)
	assert.Nil(t, disabled.NextDueAt)
	assert.Equal(t, schedule.Once, disabled.ScheduleKind)
	assert.Equal(t, "2026-10-15", *disabled.LocalDate)

	input.Enabled = true
	s.now = func() time.Time { return clockTime.Add(24 * time.Hour) }
	reenabled, err := s.Update(ctx, created.ID, input)
	require.NoError(t, err)
	assert.True(t, reenabled.Enabled)
	require.NotNil(t, reenabled.NextDueAt)
	assert.Equal(t, time.Date(2026, 10, 15, 14, 0, 0, 0, time.UTC).UnixMilli(), *reenabled.NextDueAt)
}
