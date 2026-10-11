package task

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/dongwlin/dan-tick/internal/schedule"
	"github.com/dongwlin/dan-tick/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

var clockTime = time.Date(2026, time.October, 11, 11, 0, 0, 0, time.UTC)

func newTestService(t *testing.T) (*Service, *bun.DB) {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "tasks.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	s := NewService(db)
	s.now = func() time.Time { return clockTime }
	return s, db
}

func addRoom(t *testing.T, db *bun.DB, externalID string) int64 {
	t.Helper()
	var id int64
	err := db.QueryRowContext(context.Background(), `
		INSERT INTO rooms (platform_room_id, created_at, updated_at)
		VALUES (?, ?, ?) RETURNING id`, externalID, clockTime.UnixMilli(), clockTime.UnixMilli()).Scan(&id)
	require.NoError(t, err)
	return id
}

func singleInput(roomID int64) CreateInput {
	return CreateInput{
		RoomID:   roomID,
		Name:     "  morning greetings ",
		Kind:     Single,
		Messages: []string{"  hello world  "},
		Schedule: schedule.Rule{
			Kind:      schedule.Daily,
			LocalTime: "22:00:00",
		},
		Enabled: true,
	}
}

func ptr[T any](value T) *T { return &value }

func TestCreateAndReadTasks(t *testing.T) {
	s, db := newTestService(t)
	ctx := context.Background()
	roomID := addRoom(t, db, "5440")
	otherRoom := addRoom(t, db, "777")
	input := singleInput(roomID)
	single, err := s.Create(ctx, input)
	require.NoError(t, err)
	assert.Positive(t, single.ID)
	assert.Equal(t, roomID, single.RoomID)
	assert.Equal(t, "morning greetings", single.Name)
	assert.Equal(t, Single, single.Kind)
	assert.True(t, single.Enabled)
	assert.Equal(t, "Asia/Shanghai", single.Timezone)
	assert.Nil(t, single.LocalDate)
	assert.Nil(t, single.Weekdays)
	assert.Nil(t, single.IntervalSeconds)
	assert.Nil(t, single.MaxCount)
	assert.Equal(t, clockTime.UnixMilli(), single.CreatedAt)
	assert.Equal(t, clockTime.UnixMilli(), single.UpdatedAt)
	require.NotNil(t, single.NextDueAt)
	assert.Equal(t, time.Date(2026, 10, 11, 14, 0, 0, 0, time.UTC).UnixMilli(), *single.NextDueAt)
	require.Len(t, single.Messages, 1)
	assert.Equal(t, "hello world", single.Messages[0].Content)
	assert.Equal(t, 0, single.Messages[0].Position)

	loopInput := CreateInput{
		RoomID:             roomID,
		Name:               "loop messages",
		Kind:               Loop,
		Messages:           []string{" first ", " ", "second", "\t", "third"},
		IntervalSeconds:    ptr(int64(15)),
		MaxCount:           ptr(int64(30)),
		MaxDurationSeconds: ptr(int64(3600)),
		Schedule: schedule.Rule{
			Kind: schedule.Weekly, Timezone: "America/New_York", LocalTime: "22:00:00", Weekdays: 1 | 1<<2,
		},
		Enabled: true,
	}
	loop, err := s.Create(ctx, loopInput)
	require.NoError(t, err)
	assert.Equal(t, int64(15), *loop.IntervalSeconds)
	assert.Equal(t, int64(30), *loop.MaxCount)
	assert.Equal(t, int64(3600), *loop.MaxDurationSeconds)
	assert.Equal(t, schedule.Weekly, loop.ScheduleKind)
	assert.Equal(t, "America/New_York", loop.Timezone)
	require.NotNil(t, loop.Weekdays)
	assert.Equal(t, 5, *loop.Weekdays)
	require.NotNil(t, loop.NextDueAt)
	assert.Equal(t, time.Date(2026, 10, 13, 2, 0, 0, 0, time.UTC).UnixMilli(), *loop.NextDueAt)
	require.Len(t, loop.Messages, 3)
	for i, want := range []string{"first", "second", "third"} {
		assert.Equal(t, want, loop.Messages[i].Content)
		assert.Equal(t, i, loop.Messages[i].Position)
		assert.Equal(t, loop.ID, loop.Messages[i].TaskID)
	}

	otherInput := singleInput(otherRoom)
	otherInput.Schedule = schedule.Rule{
		Kind: schedule.Once, Timezone: "Asia/Shanghai", LocalDate: "2026-10-15", LocalTime: "22:00:00",
	}
	otherInput.Enabled = false
	other, err := s.Create(ctx, otherInput)
	require.NoError(t, err)
	assert.False(t, other.Enabled)
	assert.Nil(t, other.NextDueAt)
	require.NotNil(t, other.LocalDate)
	assert.Equal(t, "2026-10-15", *other.LocalDate)

	got, err := s.Get(ctx, loop.ID)
	require.NoError(t, err)
	assert.Equal(t, loop, got)
	all, err := s.List(ctx, 0)
	require.NoError(t, err)
	require.Len(t, all, 3)
	assert.Equal(t, []int64{other.ID, loop.ID, single.ID}, []int64{all[0].ID, all[1].ID, all[2].ID})
	assert.Equal(t, []Message{loop.Messages[0], loop.Messages[1], loop.Messages[2]}, all[1].Messages)
	filtered, err := s.List(ctx, roomID)
	require.NoError(t, err)
	require.Len(t, filtered, 2)
	assert.Equal(t, []int64{loop.ID, single.ID}, []int64{filtered[0].ID, filtered[1].ID})
	empty, err := s.List(ctx, 99999)
	require.NoError(t, err)
	assert.Empty(t, empty)
	assert.NotNil(t, empty)
}

func TestCreateValidationCannotPersistPartialTask(t *testing.T) {
	s, db := newTestService(t)
	roomID := addRoom(t, db, "5440")
	base := singleInput(roomID)
	cases := []struct {
		name   string
		modify func(*CreateInput)
	}{
		{"missing room id", func(v *CreateInput) { v.RoomID = 0 }},
		{"blank name", func(v *CreateInput) { v.Name = " \t " }},
		{"control in name", func(v *CreateInput) { v.Name = "bad\nname" }},
		{"invalid utf8 name", func(v *CreateInput) { v.Name = string([]byte{0xff}) }},
		{"missing message", func(v *CreateInput) { v.Messages = []string{"", "  "} }},
		{"two single messages", func(v *CreateInput) { v.Messages = []string{"one", "two"} }},
		{"single loop interval", func(v *CreateInput) { v.IntervalSeconds = ptr(int64(15)) }},
		{"single loop count", func(v *CreateInput) { v.MaxCount = ptr(int64(1)) }},
		{"unknown type", func(v *CreateInput) { v.Kind = "other" }},
		{"loop no stop", func(v *CreateInput) { v.Kind = Loop; v.IntervalSeconds = ptr(int64(15)) }},
		{"loop no interval", func(v *CreateInput) { v.Kind = Loop; v.MaxCount = ptr(int64(3)) }},
		{"loop invalid interval", func(v *CreateInput) { v.Kind = Loop; v.IntervalSeconds = ptr(int64(0)); v.MaxCount = ptr(int64(3)) }},
		{"loop invalid count", func(v *CreateInput) { v.Kind = Loop; v.IntervalSeconds = ptr(int64(1)); v.MaxCount = ptr(int64(-1)) }},
		{"loop invalid duration", func(v *CreateInput) {
			v.Kind = Loop
			v.IntervalSeconds = ptr(int64(1))
			v.MaxDurationSeconds = ptr(int64(0))
		}},
		{"loop blank messages", func(v *CreateInput) {
			v.Kind = Loop
			v.IntervalSeconds = ptr(int64(15))
			v.MaxCount = ptr(int64(5))
			v.Messages = []string{" \n "}
		}},
		{"control character", func(v *CreateInput) { v.Messages = []string{"test\nmessage"} }},
		{"invalid utf8 message", func(v *CreateInput) { v.Messages = []string{string([]byte{0xff})} }},
		{"invalid timezone", func(v *CreateInput) { v.Schedule.Timezone = "Local" }},
		{"invalid local time", func(v *CreateInput) { v.Schedule.LocalTime = "25:00:00" }},
		{"invalid weekly mask", func(v *CreateInput) { v.Schedule.Kind = schedule.Weekly; v.Schedule.Weekdays = 128 }},
		{"invalid daily date", func(v *CreateInput) { v.Schedule.LocalDate = "2026-10-15" }},
		{"invalid once date", func(v *CreateInput) { v.Schedule.Kind = schedule.Once; v.Schedule.LocalDate = "2026-02-30" }},
		{"enabled one-time in past", func(v *CreateInput) { v.Schedule.Kind = schedule.Once; v.Schedule.LocalDate = "2026-10-10" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := base
			tc.modify(&input)
			_, err := s.Create(context.Background(), input)
			require.ErrorIs(t, err, ErrInvalidTask)
			var tasks, messages int
			require.NoError(t, db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM tasks`).Scan(&tasks))
			require.NoError(t, db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM task_messages`).Scan(&messages))
			assert.Zero(t, tasks)
			assert.Zero(t, messages)
		})
	}
}

func TestCreateChecksRoomInsideTransaction(t *testing.T) {
	s, db := newTestService(t)
	roomID := addRoom(t, db, "5440")
	_, err := db.ExecContext(context.Background(), `UPDATE rooms SET deleted_at = ? WHERE id = ?`, clockTime.UnixMilli(), roomID)
	require.NoError(t, err)
	for _, id := range []int64{roomID, 9999} {
		_, err := s.Create(context.Background(), singleInput(id))
		require.ErrorIs(t, err, ErrRoomUnavailable)
	}
	var count int
	require.NoError(t, db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM tasks`).Scan(&count))
	assert.Zero(t, count)
}

func TestCreateMessageWriteFailureRollsBackTask(t *testing.T) {
	s, db := newTestService(t)
	roomID := addRoom(t, db, "5440")
	_, err := db.ExecContext(context.Background(), `
		CREATE TRIGGER fail_second_message BEFORE INSERT ON task_messages
		WHEN NEW.position = 1 BEGIN SELECT RAISE(ABORT, 'test message failure'); END`)
	require.NoError(t, err)
	input := singleInput(roomID)
	input.Kind = Loop
	input.Messages = []string{"first", "second"}
	input.IntervalSeconds = ptr(int64(15))
	input.MaxCount = ptr(int64(2))
	_, err = s.Create(context.Background(), input)
	require.ErrorContains(t, err, "test message failure")
	var tasks, messages int
	require.NoError(t, db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM tasks`).Scan(&tasks))
	require.NoError(t, db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM task_messages`).Scan(&messages))
	assert.Zero(t, tasks)
	assert.Zero(t, messages)
}

func TestCreatePersistsAcrossReopenAndPreservesMessageOrder(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "reopen.db")
	db, err := storage.Open(ctx, path)
	require.NoError(t, err)
	roomID := addRoom(t, db, "5440")
	s := NewService(db)
	s.now = func() time.Time { return clockTime }
	input := singleInput(roomID)
	input.Kind = Loop
	input.Messages = []string{"one", "two", "three"}
	input.IntervalSeconds = ptr(int64(15))
	input.MaxDurationSeconds = ptr(int64(120))
	created, err := s.Create(ctx, input)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	db, err = storage.Open(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	s = NewService(db)
	got, err := s.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created, got)

	// SQLite schema permits equal positions; order is position then ID.
	_, err = db.ExecContext(ctx, `UPDATE task_messages SET position = 0 WHERE task_id = ?`, created.ID)
	require.NoError(t, err)
	got, err = s.Get(ctx, created.ID)
	require.NoError(t, err)
	for i, want := range []string{"one", "two", "three"} {
		assert.Equal(t, want, got.Messages[i].Content)
	}
}

func TestReadExcludesDeletedTaskAndRoom(t *testing.T) {
	s, db := newTestService(t)
	ctx := context.Background()
	roomID := addRoom(t, db, "5440")
	first, err := s.Create(ctx, singleInput(roomID))
	require.NoError(t, err)
	second, err := s.Create(ctx, singleInput(roomID))
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE tasks SET deleted_at = ? WHERE id = ?`, clockTime.UnixMilli(), first.ID)
	require.NoError(t, err)
	_, err = s.Get(ctx, first.ID)
	require.ErrorIs(t, err, ErrNotFound)
	got, err := s.List(ctx, roomID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, second.ID, got[0].ID)
	_, err = db.ExecContext(ctx, `UPDATE rooms SET deleted_at = ? WHERE id = ?`, clockTime.UnixMilli(), roomID)
	require.NoError(t, err)
	_, err = s.Get(ctx, second.ID)
	require.ErrorIs(t, err, ErrNotFound)
	got, err = s.List(ctx, 0)
	require.NoError(t, err)
	assert.Empty(t, got)
	_, err = s.Get(ctx, -1)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = s.List(ctx, -1)
	require.ErrorIs(t, err, ErrRoomUnavailable)
}

func TestDisabledPastOnceHasNoDue(t *testing.T) {
	s, db := newTestService(t)
	input := singleInput(addRoom(t, db, "5440"))
	input.Enabled = false
	input.Schedule = schedule.Rule{Kind: schedule.Once, Timezone: "Asia/Shanghai", LocalDate: "2026-10-10", LocalTime: "22:00:00"}
	got, err := s.Create(context.Background(), input)
	require.NoError(t, err)
	assert.Nil(t, got.NextDueAt)
	assert.Equal(t, "2026-10-10", *got.LocalDate)
}

func TestEnabledOncePersistsExactDueAndDisabledDailyHasNoCursor(t *testing.T) {
	s, db := newTestService(t)
	roomID := addRoom(t, db, "5440")
	input := singleInput(roomID)
	input.Schedule = schedule.Rule{
		Kind: schedule.Once, Timezone: "Asia/Shanghai", LocalDate: "2026-10-15", LocalTime: "22:00:00",
	}
	once, err := s.Create(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, once.NextDueAt)
	assert.Equal(t, time.Date(2026, 10, 15, 14, 0, 0, 0, time.UTC).UnixMilli(), *once.NextDueAt)
	input.Schedule = schedule.Rule{Kind: schedule.Daily, LocalTime: "22:00:00"}
	input.Enabled = false
	disabled, err := s.Create(context.Background(), input)
	require.NoError(t, err)
	assert.Nil(t, disabled.NextDueAt)
	assert.Equal(t, schedule.Daily, disabled.ScheduleKind)
	assert.Equal(t, "Asia/Shanghai", disabled.Timezone)
}

func TestCancelledCreateCannotWrite(t *testing.T) {
	s, db := newTestService(t)
	roomID := addRoom(t, db, "5440")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.Create(ctx, singleInput(roomID))
	require.Error(t, err)
	var count int
	require.NoError(t, db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM tasks`).Scan(&count))
	assert.Zero(t, count)
}
