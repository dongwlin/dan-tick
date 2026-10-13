package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dongwlin/dan-tick/internal/room"
	"github.com/dongwlin/dan-tick/internal/schedule"
	"github.com/dongwlin/dan-tick/internal/scheduler"
	"github.com/dongwlin/dan-tick/internal/storage"
	"github.com/dongwlin/dan-tick/internal/task"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func configurationTest(t *testing.T) (*ConfigurationService, *scheduler.Manager, *bun.DB) {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	// Public room lookups use a fake transport; no actual Bilibili traffic.
	client := room.NewClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Contains(t, []string{"12345", "98765"}, req.URL.Query().Get("room_id"))
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"code":0,"data":{"room_id":98765,"uid":0,"title":"private room title","user_cover":"https://i0.hdslb.com/bfs/live/cover.jpg"}}`)),
			Header:     make(http.Header),
		}, nil
	})})
	manager := scheduler.NewManager(db, nil, func(context.Context, scheduler.ClaimResult) error {
		return nil
	}, nil)
	t.Cleanup(func() { <-manager.Stop().Done() })
	require.NoError(t, manager.Start(context.Background()))
	return NewConfigurationService(db, client, manager, nil), manager, db
}

func taskRequest(roomID int64) TaskInput {
	future := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	return TaskInput{
		RoomID: roomID, Name: "private task", Kind: task.Loop,
		Messages:        []string{"alpha", "beta"},
		IntervalSeconds: intPtr(15), MaxCount: intPtr(2),
		Schedule: Plan{
			Kind: schedule.Once, Timezone: "UTC",
			LocalDate: future.Format(time.DateOnly), LocalTime: future.Format(time.TimeOnly),
		},
		Enabled: true,
	}
}

func intPtr(value int64) *int64 { return &value }

func TestConfigurationBindingRoomTaskLifecycle(t *testing.T) {
	ctx := context.Background()
	service, _, db := configurationTest(t)

	rooms, err := service.ListRooms(ctx)
	require.NoError(t, err)
	require.Empty(t, rooms)
	_, err = service.AddRoom(ctx, AddRoomInput{Value: "invalid-sensitive-input"})
	assertPublicError(t, err, "invalid_input", "invalid-sensitive-input")

	savedRoom, err := service.AddRoom(ctx, AddRoomInput{Value: "12345", Remark: "  personal room  "})
	require.NoError(t, err)
	require.Equal(t, "98765", savedRoom.PlatformRoomID)
	require.Equal(t, "private room title", savedRoom.Title)
	require.Equal(t, "personal room", savedRoom.Remark)
	require.Zero(t, savedRoom.TaskCount)
	require.Equal(t, "https://i0.hdslb.com/bfs/live/cover.jpg", savedRoom.CoverURL)
	changed, err := service.RefreshRoomMetadata(ctx, savedRoom.ID)
	require.NoError(t, err)
	require.False(t, changed) // Freshly added; no extra API request.
	_, err = service.RefreshRoomMetadata(ctx, -1)
	assertPublicError(t, err, "not_found", "private")
	_, err = service.AddRoom(ctx, AddRoomInput{Value: "12345"})
	assertPublicError(t, err, "already_exists", "98765")

	input := taskRequest(savedRoom.ID)
	created, err := service.CreateTask(ctx, input)
	require.NoError(t, err)
	require.Equal(t, input.Messages, created.Messages)
	require.NotNil(t, created.NextDueAt)
	require.Equal(t, input.Schedule, created.Schedule)

	listed, err := service.ListTasks(ctx, savedRoom.ID)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, created, listed[0])
	got, err := service.GetTask(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, created, got)
	rooms, err = service.ListRooms(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, rooms[0].TaskCount)

	input.Name = "edited task"
	input.Messages = []string{"new first", "new second"}
	edited, err := service.UpdateTask(ctx, created.ID, input)
	require.NoError(t, err)
	require.Equal(t, created.NextDueAt, edited.NextDueAt) // Non-schedule edit preserves cursor.
	require.Equal(t, input.Messages, edited.Messages)
	require.Equal(t, "edited task", edited.Name)

	disabled, err := service.SetTaskEnabled(ctx, created.ID, false)
	require.NoError(t, err)
	require.False(t, disabled.Enabled)
	require.Nil(t, disabled.NextDueAt)
	reEnabled, err := service.SetTaskEnabled(ctx, created.ID, true)
	require.NoError(t, err)
	require.True(t, reEnabled.Enabled)

	updatedRoom, err := service.UpdateRoom(ctx, savedRoom.ID, UpdateRoomInput{Remark: "  personal note  "})
	require.NoError(t, err)
	require.Equal(t, "personal note", updatedRoom.Remark)
	require.Equal(t, 1, updatedRoom.TaskCount)
	require.NoError(t, service.DeleteRoom(ctx, savedRoom.ID))
	rooms, err = service.ListRooms(ctx)
	require.NoError(t, err)
	require.Empty(t, rooms)
	tasks, err := service.ListTasks(ctx, 0)
	require.NoError(t, err)
	require.Empty(t, tasks)
	_, err = service.GetTask(ctx, created.ID)
	assertPublicError(t, err, "not_found", "private task")
	var deleted int64
	require.NoError(t, db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tasks WHERE id = ? AND enabled = 0 AND deleted_at IS NOT NULL", created.ID).Scan(&deleted))
	require.Equal(t, int64(1), deleted)
}

func TestConfigurationPagesHaveStableCursorsAndResetFilters(t *testing.T) {
	ctx := context.Background()
	svc, _, db := configurationTest(t)
	first, err := svc.AddRoom(ctx, AddRoomInput{Value: "12345"})
	require.NoError(t, err)
	// Additional rooms are inserted locally to test cursor boundaries without
	// making real Bilibili requests or depending on a fake API ID.
	for _, id := range []string{"200", "300", "400"} {
		_, err = db.ExecContext(ctx, `INSERT INTO rooms (platform_room_id, title, anchor_name, created_at, updated_at)
			VALUES (?, ?, '', 1, 1)`, id, "Title "+id)
		require.NoError(t, err)
	}
	page, err := svc.ListRoomPage(ctx, PageInput{Limit: 2})
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	require.NotNil(t, page.NextCursor)
	firstPageIDs := []int64{page.Items[0].ID, page.Items[1].ID}
	_, err = db.ExecContext(ctx, `INSERT INTO rooms (platform_room_id, title, anchor_name, created_at, updated_at)
		VALUES ('500', 'New room', '', 1, 1)`)
	require.NoError(t, err)
	page, err = svc.ListRoomPage(ctx, PageInput{Cursor: page.NextCursor, Limit: 2})
	require.NoError(t, err)
	require.Equal(t, []int64{first.ID + 1, first.ID}, []int64{page.Items[0].ID, page.Items[1].ID})
	require.Nil(t, page.NextCursor)
	require.NotContains(t, firstPageIDs, page.Items[0].ID)
	_, err = svc.ListRoomPage(ctx, PageInput{Limit: 101})
	assertPublicError(t, err, "invalid_input", "private")

	for _, name := range []string{"hello", "other", "hello again", "third hello", "last"} {
		input := taskRequest(first.ID)
		input.Name = name
		_, err = svc.CreateTask(ctx, input)
		require.NoError(t, err)
	}
	all, err := svc.ListTaskPage(ctx, TaskPageInput{Limit: 2})
	require.NoError(t, err)
	require.Len(t, all.Items, 2)
	require.NotNil(t, all.NextCursor)
	filtered, err := svc.ListTaskPage(ctx, TaskPageInput{Search: "hello", Limit: 2})
	require.NoError(t, err)
	require.Len(t, filtered.Items, 2)
	require.NotNil(t, filtered.NextCursor)
	require.Equal(t, "third hello", filtered.Items[0].Name)
	filteredEnd, err := svc.ListTaskPage(ctx, TaskPageInput{Search: "hello", Cursor: filtered.NextCursor, Limit: 2})
	require.NoError(t, err)
	require.Len(t, filteredEnd.Items, 1)
	require.Equal(t, "hello", filteredEnd.Items[0].Name)
	require.Nil(t, filteredEnd.NextCursor)
	// A different search starts at the newest matching task, not a previous cursor.
	other, err := svc.ListTaskPage(ctx, TaskPageInput{Search: "other", Limit: 2})
	require.NoError(t, err)
	require.Len(t, other.Items, 1)
	require.Equal(t, "other", other.Items[0].Name)
	_, err = svc.ListTaskPage(ctx, TaskPageInput{Limit: -1})
	assertPublicError(t, err, "invalid_input", "private")
}

func TestConfigurationBindingRejectsInvalidInputAndPreservesState(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := configurationTest(t)
	savedRoom, err := svc.AddRoom(ctx, AddRoomInput{Value: "12345"})
	require.NoError(t, err)
	input := taskRequest(savedRoom.ID)
	input.Kind = task.Single // two messages are invalid for a single task
	_, err = svc.CreateTask(ctx, input)
	assertPublicError(t, err, "invalid_input", "alpha")
	tasks, err := svc.ListTasks(ctx, 0)
	require.NoError(t, err)
	require.Empty(t, tasks)
	_, err = svc.SetTaskEnabled(ctx, -1, true)
	assertPublicError(t, err, "not_found", "task")
	require.NoError(t, svc.DeleteRoom(ctx, savedRoom.ID))
	_, err = svc.CreateTask(ctx, taskRequest(savedRoom.ID))
	assertPublicError(t, err, "room_unavailable", "private")
}

func TestConfigurationBindingReportsCommittedButUnsyncedWrite(t *testing.T) {
	ctx := context.Background()
	svc, manager, db := configurationTest(t)
	roomRow, err := svc.AddRoom(ctx, AddRoomInput{Value: "12345"})
	require.NoError(t, err)
	input := taskRequest(roomRow.ID)
	saved, err := svc.CreateTask(ctx, input)
	require.NoError(t, err)
	<-manager.Stop().Done()

	_, err = svc.SetTaskEnabled(ctx, saved.ID, false)
	assertPublicError(t, err, "schedule_sync", "private")
	var enabled bool
	require.NoError(t, db.QueryRowContext(ctx, "SELECT enabled FROM tasks WHERE id = ?", saved.ID).Scan(&enabled))
	require.False(t, enabled) // No accidental rollback claim on a post-commit error.
}

func TestConfigurationBindingDeletionRejectsBusyExecution(t *testing.T) {
	ctx := context.Background()
	svc, _, db := configurationTest(t)
	createdRoom, err := svc.AddRoom(ctx, AddRoomInput{Value: "12345"})
	require.NoError(t, err)
	saved, err := svc.CreateTask(ctx, taskRequest(createdRoom.ID))
	require.NoError(t, err)
	var executionID int64
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO executions (task_id, trigger_kind, status, created_at,
			queued_at, task_name_snapshot, platform_room_id_snapshot)
		VALUES (?, 'manual', 'queued', 1, 1, 'snapshot', '98765') RETURNING id`, saved.ID).Scan(&executionID))

	err = svc.DeleteTask(ctx, saved.ID)
	assertPublicError(t, err, "conflict", "snapshot")
	err = svc.DeleteRoom(ctx, createdRoom.ID)
	assertPublicError(t, err, "conflict", "snapshot")
	tasks, err := svc.ListTasks(ctx, 0)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	rooms, err := svc.ListRooms(ctx)
	require.NoError(t, err)
	require.Len(t, rooms, 1)

	_, err = db.ExecContext(ctx,
		"UPDATE executions SET status = 'stopped', finished_at = 2 WHERE id = ?", executionID)
	require.NoError(t, err)
	require.NoError(t, svc.DeleteTask(ctx, saved.ID))
	tasks, err = svc.ListTasks(ctx, 0)
	require.NoError(t, err)
	require.Empty(t, tasks)
	require.NoError(t, svc.DeleteRoom(ctx, createdRoom.ID))
}

func TestBindingSanitizesTopLevelErrorsAndJSONResponses(t *testing.T) {
	secret := "private SESSDATA and message text"
	unknown := protect(fmt.Errorf("query failed: %s", secret))
	assertPublicError(t, unknown, "internal", secret)
	for _, tc := range []struct {
		err  error
		code string
	}{
		{room.ErrNotFound, "not_found"},
		{room.ErrUnavailable, "upstream_unavailable"},
		{room.ErrRoomBusy, "conflict"},
		{task.ErrTaskBusy, "conflict"},
		{scheduler.ErrSchedulerClosed, "unavailable"},
		{fmt.Errorf("%w: db details: %s", scheduler.ErrScheduleSync, secret), "schedule_sync"},
		{context.DeadlineExceeded, "cancelled"},
	} {
		assertPublicError(t, protect(tc.err), tc.code, secret)
	}

	row := taskView(task.Task{
		ID: 1, Name: "task", Messages: []task.Message{{ID: 2, TaskID: 1, Content: "text"}},
	})
	encoded, err := json.Marshal(row)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "TaskID")
	require.NotContains(t, string(encoded), "deletedAt")
	require.NotContains(t, string(encoded), "bun")
	require.Contains(t, string(encoded), `"messages":["text"]`)
	require.Nil(t, protect(nil))
	require.ErrorIs(t, scheduleSyncError(errors.New("db error")), scheduler.ErrScheduleSync)
}

func assertPublicError(t *testing.T, err error, code, forbidden string) {
	t.Helper()
	require.Error(t, err)
	require.NotContains(t, err.Error(), forbidden)
	var payload safeError
	require.NoError(t, json.Unmarshal(MarshalError(err), &payload))
	require.Equal(t, code, payload.Code)
	require.NotEmpty(t, payload.Message)
	require.Equal(t, err.Error(), payload.Message)
	require.NotContains(t, string(MarshalError(err)), forbidden)
}
