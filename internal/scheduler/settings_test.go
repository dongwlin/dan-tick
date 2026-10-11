package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/dongwlin/dan-tick/internal/schedule"
	"github.com/dongwlin/dan-tick/internal/settings"
	"github.com/dongwlin/dan-tick/internal/task"
	"github.com/stretchr/testify/require"
)

func TestGlobalPauseSurvivesRestartAndResumeSkipsMissedOccurrence(t *testing.T) {
	ctx := context.Background()
	db, path := claimDB(t)
	roomID := savedRoom(t, db)
	due := futureMinute()
	input := taskInput(roomID, due)
	input.Schedule = schedule.Rule{Kind: schedule.Daily, Timezone: "UTC", LocalTime: due.Format(time.TimeOnly)}
	m := NewManager(db, nil, readyHandler, nil)
	created, err := m.CreateTask(ctx, input)
	require.NoError(t, err)
	require.NoError(t, m.Start(ctx))
	require.Len(t, m.entries, 1)
	require.NoError(t, m.SetSchedulingEnabled(ctx, false))
	require.Empty(t, m.entries)
	require.Empty(t, m.cron.Entries())
	require.Equal(t, due.UnixMilli(), storedDue(t, db, created.ID).Int64)

	// This persisted pause blocks even an in-flight or stale callback that
	// reached the claim transaction after the switch was committed.
	result, err := newClaim(db, due.Add(time.Second)).Claim(ctx, created.ID, due.UnixMilli())
	require.NoError(t, err)
	require.False(t, result.Claimed)
	require.Zero(t, storedExecutions(t, db))
	require.Equal(t, due.UnixMilli(), storedDue(t, db, created.ID).Int64)

	// Changes to task definitions while paused must not add Cron entries.
	input.Name = "edited while paused"
	updated, err := m.UpdateTask(ctx, created.ID, input)
	require.NoError(t, err)
	require.Equal(t, "edited while paused", updated.Name)
	require.Empty(t, m.entries)
	require.NoError(t, m.Rebuild(ctx))
	require.Empty(t, m.entries)
	<-m.Stop().Done()

	// Simulate a restart after the skipped daily occurrence, using a fresh
	// SQLite connection and a controlled clock in both recovery and Cron.
	other, err := openTestDB(path)
	require.NoError(t, err)
	defer func() { require.NoError(t, other.Close()) }()
	restarted := NewManager(other, nil, readyHandler, nil)
	defer func() { <-restarted.Stop().Done() }()
	at := due.Add(time.Hour)
	restarted.now = func() time.Time { return at }
	restarted.recovery.now = func() time.Time { return at }
	require.NoError(t, restarted.Start(ctx))
	require.Empty(t, restarted.entries)
	values, err := settings.NewService(other).Get(ctx)
	require.NoError(t, err)
	require.False(t, values.SchedulingEnabled)
	require.Equal(t, 1, storedExecutions(t, other), "the missed plan is recorded, never sent")
	next := storedDue(t, other, created.ID)
	require.True(t, next.Valid)
	require.Equal(t, due.Add(24*time.Hour).UnixMilli(), next.Int64)
	require.NoError(t, restarted.SetSchedulingEnabled(ctx, true))
	require.Len(t, restarted.entries, 1)
	require.Equal(t, next.Int64, restarted.entries[created.ID].due)
	require.Equal(t, 1, storedExecutions(t, other), "resuming never replays old plans")
	values, err = settings.NewService(other).Get(ctx)
	require.NoError(t, err)
	require.True(t, values.SchedulingEnabled)
	got, err := task.NewService(other).Get(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, "edited while paused", got.Name)
	require.Len(t, got.Messages, 1)
}

func TestGlobalPausePreservesHistoryAndDoesNotChangeTaskFlags(t *testing.T) {
	ctx := context.Background()
	db, _ := claimDB(t)
	roomID := savedRoom(t, db)
	created, err := task.NewService(db).Create(ctx, taskInput(roomID, futureMinute()))
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO executions
		(task_id, trigger_kind, status, created_at, finished_at, task_name_snapshot, platform_room_id_snapshot)
		VALUES (?, 'manual', 'completed', 1, 2, 'saved', '12345')`, created.ID)
	require.NoError(t, err)
	m := NewManager(db, nil, readyHandler, nil)
	defer func() { <-m.Stop().Done() }()
	require.NoError(t, m.Start(ctx))
	require.NoError(t, m.SetSchedulingEnabled(ctx, false))
	require.NoError(t, m.SetSchedulingEnabled(ctx, false))
	require.True(t, storedDue(t, db, created.ID).Valid)
	require.Equal(t, 1, storedExecutions(t, db))
	got, err := task.NewService(db).Get(ctx, created.ID)
	require.NoError(t, err)
	require.True(t, got.Enabled, "global pause must not disable each task")
	require.NoError(t, m.SetSchedulingEnabled(ctx, true))
	require.Len(t, m.entries, 1)
	require.Equal(t, 1, storedExecutions(t, db))
}

func TestResumeWhileRunningReconcilesPausedPlansWithoutHandoff(t *testing.T) {
	ctx := context.Background()
	db, _ := claimDB(t)
	roomID := savedRoom(t, db)
	due := futureMinute()
	input := taskInput(roomID, due)
	input.Schedule = schedule.Rule{Kind: schedule.Daily, Timezone: "UTC", LocalTime: due.Format(time.TimeOnly)}
	handoffs := 0
	m := NewManager(db, nil, func(context.Context, ClaimResult) error {
		handoffs++
		return nil
	}, nil)
	defer func() { <-m.Stop().Done() }()
	created, err := m.CreateTask(ctx, input)
	require.NoError(t, err)
	require.NoError(t, m.Start(ctx))
	require.NoError(t, m.SetSchedulingEnabled(ctx, false))
	at := due.Add(time.Hour)
	m.now = func() time.Time { return at }
	m.recovery.now = func() time.Time { return at }
	late, err := m.recovery.ReconcileLate(ctx, created.ID, due.UnixMilli())
	require.NoError(t, err)
	require.False(t, late, "a stale Cron callback cannot advance a paused cursor")
	require.Zero(t, storedExecutions(t, db))
	require.NoError(t, m.SetSchedulingEnabled(ctx, true))
	require.Equal(t, 1, storedExecutions(t, db))
	require.Equal(t, due.Add(24*time.Hour).UnixMilli(), storedDue(t, db, created.ID).Int64)
	require.Len(t, m.entries, 1)
	require.Zero(t, handoffs)
}

func TestGlobalPauseFailedPersistenceDoesNotUnschedule(t *testing.T) {
	ctx := context.Background()
	db, _ := claimDB(t)
	roomID := savedRoom(t, db)
	m := NewManager(db, nil, readyHandler, nil)
	defer func() { <-m.Stop().Done() }()
	_, err := m.CreateTask(ctx, taskInput(roomID, futureMinute()))
	require.NoError(t, err)
	require.NoError(t, m.Start(ctx))
	require.Len(t, m.entries, 1)
	_, err = db.ExecContext(ctx, `CREATE TRIGGER reject_pause BEFORE INSERT ON app_settings
		BEGIN SELECT RAISE(ABORT, 'test denied write'); END`)
	require.NoError(t, err)
	require.Error(t, m.SetSchedulingEnabled(ctx, false))
	require.Len(t, m.entries, 1)
	values, err := settings.NewService(db).Get(ctx)
	require.NoError(t, err)
	require.True(t, values.SchedulingEnabled)
}

func TestResumeRecoveryFailureLeavesRetryablePersistedSwitch(t *testing.T) {
	ctx := context.Background()
	db, _ := claimDB(t)
	roomID := savedRoom(t, db)
	due := futureMinute()
	input := taskInput(roomID, due)
	input.Schedule = schedule.Rule{Kind: schedule.Daily, Timezone: "UTC", LocalTime: due.Format(time.TimeOnly)}
	m := NewManager(db, nil, readyHandler, nil)
	defer func() { <-m.Stop().Done() }()
	created, err := m.CreateTask(ctx, input)
	require.NoError(t, err)
	require.NoError(t, m.Start(ctx))
	require.NoError(t, m.SetSchedulingEnabled(ctx, false))
	at := due.Add(time.Hour)
	m.now = func() time.Time { return at }
	m.recovery.now = func() time.Time { return at }
	_, err = db.ExecContext(ctx, `CREATE TRIGGER reject_recovery BEFORE INSERT ON executions
		BEGIN SELECT RAISE(ABORT, 'test missed insert failure'); END`)
	require.NoError(t, err)
	require.ErrorIs(t, m.SetSchedulingEnabled(ctx, true), ErrScheduleSync)
	values, err := settings.NewService(db).Get(ctx)
	require.NoError(t, err)
	require.True(t, values.SchedulingEnabled, "the setting commit is not rolled back")
	require.Empty(t, m.entries, "recovery failure must not enable stale timers")
	require.Zero(t, storedExecutions(t, db))
	require.Equal(t, due.UnixMilli(), storedDue(t, db, created.ID).Int64)
	_, err = db.ExecContext(ctx, "DROP TRIGGER reject_recovery")
	require.NoError(t, err)
	_, err = m.Resume(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, storedExecutions(t, db))
	require.Len(t, m.entries, 1)
	require.Equal(t, due.Add(24*time.Hour).UnixMilli(), m.entries[created.ID].due)
}
