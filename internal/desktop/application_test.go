package desktop

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/dongwlin/dan-tick/internal/buildinfo"
	"github.com/dongwlin/dan-tick/internal/schedule"
	"github.com/stretchr/testify/require"
)

func TestApplicationBuildInfoBindingExposesCompiledMetadataWithoutBusinessDependencies(t *testing.T) {
	original := buildinfo.Current()
	t.Cleanup(func() {
		buildinfo.Version, buildinfo.BuildTime, buildinfo.Commit = original.Version, original.BuildTime, original.Commit
	})
	buildinfo.Version, buildinfo.BuildTime, buildinfo.Commit = "v1.2.3", "2026-10-13T08:12:00Z", "a1b2c3d4e5f67890"

	info := (&ApplicationService{}).GetBuildInfo()
	encoded, err := json.Marshal(info)
	require.NoError(t, err)
	require.JSONEq(t, `{"version":"v1.2.3","buildTime":"2026-10-13T08:12:00Z","commit":"a1b2c3d4e5f67890"}`, string(encoded))
}

func TestApplicationSettingsAndOverviewUsePersistedState(t *testing.T) {
	ctx := context.Background()
	config, manager, db := configurationTest(t)
	var events []string
	app := NewApplicationService(db, manager, func(event string) { events = append(events, event) })
	values, err := app.GetPreferences(ctx)
	require.NoError(t, err)
	require.Equal(t, "Asia/Shanghai", values.DefaultTimezone)
	require.True(t, values.SchedulingEnabled)
	require.Error(t, app.SetDefaultTimezone(ctx, "not/a-real-zone"))
	require.Empty(t, events)
	require.NoError(t, app.SetDefaultTimezone(ctx, "UTC"))
	room, err := config.AddRoom(ctx, AddRoomInput{Value: "12345"})
	require.NoError(t, err)
	request := taskRequest(room.ID)
	request.Kind = "single"
	request.Messages = []string{"sensitive business text"}
	request.IntervalSeconds, request.MaxCount = nil, nil
	request.Schedule.Kind = schedule.Once
	task, err := config.CreateTask(ctx, request)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO executions
		(task_id, trigger_kind, status, created_at, sent_count, failed_count, task_name_snapshot, platform_room_id_snapshot)
		VALUES (?, 'manual', 'failed', ?, 0, 1, 'snapshot title', '98765')`, task.ID, time.Now().UnixMilli())
	require.NoError(t, err)
	overview, err := app.GetOverview(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, overview.TotalTasks)
	require.EqualValues(t, 1, overview.EnabledTasks)
	require.EqualValues(t, 1, overview.RoomCount)
	require.Len(t, overview.Upcoming, 1)
	require.Equal(t, task.ID, overview.Upcoming[0].ID)
	require.Equal(t, "snapshot title", overview.LatestFailure.TaskName)
	require.Len(t, overview.Recent, 1)
	require.NoError(t, app.SetSchedulingEnabled(ctx, false))
	values, err = NewApplicationService(db, manager, nil).GetPreferences(ctx)
	require.NoError(t, err)
	require.Equal(t, "UTC", values.DefaultTimezone)
	require.False(t, values.SchedulingEnabled)
	require.Equal(t, []string{EventApplicationChanged, EventApplicationChanged}, events)
	// Pausing does not disable individual tasks or erase their plans.
	overview, err = app.GetOverview(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, overview.EnabledTasks)
	require.Len(t, overview.Upcoming, 1)
}
