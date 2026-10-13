package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dongwlin/dan-tick/internal/diagnostic"
	"github.com/dongwlin/dan-tick/internal/room"
	"github.com/dongwlin/dan-tick/internal/schedule"
	"github.com/dongwlin/dan-tick/internal/storage"
	"github.com/dongwlin/dan-tick/internal/task"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func futureMinute() time.Time {
	return time.Now().UTC().Add(20 * time.Minute).Truncate(time.Second)
}

func taskInput(roomID int64, due time.Time) task.CreateInput {
	return task.CreateInput{
		RoomID: roomID, Name: "a private task", Kind: task.Single,
		Messages: []string{"do not log message"}, Enabled: true,
		Schedule: schedule.Rule{Kind: schedule.Once, Timezone: "UTC",
			LocalDate: due.Format(time.DateOnly), LocalTime: due.Format(time.TimeOnly)},
	}
}

func savedRoom(t *testing.T, db *bun.DB) int64 {
	t.Helper()
	var id int64
	require.NoError(t, db.QueryRowContext(context.Background(), `
		INSERT INTO rooms (platform_room_id, created_at, updated_at)
		VALUES ('12345', 1, 1) RETURNING id`).Scan(&id))
	return id
}

func readyHandler(context.Context, ClaimResult) error { return nil }

func TestManagerTaskMutationsReplaceAndRemoveCronRegistrations(t *testing.T) {
	db, _ := claimDB(t)
	roomID := savedRoom(t, db)
	m := NewManager(db, nil, readyHandler, nil)
	t.Cleanup(func() { <-m.Stop().Done() })
	firstDue := futureMinute()
	saved, err := m.CreateTask(context.Background(), taskInput(roomID, firstDue))
	require.NoError(t, err)
	require.Len(t, m.entries, 1)
	first := m.entries[saved.ID]
	require.Equal(t, firstDue.UnixMilli(), first.due)
	require.True(t, m.cron.Entry(first.id).Valid())
	require.NoError(t, m.Start(context.Background()))
	require.NoError(t, m.Start(context.Background())) // idempotent

	newDue := firstDue.Add(2 * time.Hour)
	saved, err = m.UpdateTask(context.Background(), saved.ID, taskInput(roomID, newDue))
	require.NoError(t, err)
	second := m.entries[saved.ID]
	require.NotEqual(t, first.id, second.id)
	require.Equal(t, newDue.UnixMilli(), second.due)
	require.False(t, m.cron.Entry(first.id).Valid())
	require.True(t, m.cron.Entry(second.id).Valid())

	// Even edits preserving next_due_at replace the job: stale callbacks carry
	// the old generation and cannot claim an occurrence after resync.
	input := taskInput(roomID, newDue)
	input.Name = "edited title"
	saved, err = m.UpdateTask(context.Background(), saved.ID, input)
	require.NoError(t, err)
	third := m.entries[saved.ID]
	require.NotEqual(t, second.id, third.id)
	require.Equal(t, second.due, third.due)
	require.False(t, m.cron.Entry(second.id).Valid())
	m.fire(saved.ID, first.due, first.generation)
	m.fire(saved.ID, third.due, second.generation)
	require.Zero(t, storedExecutions(t, db))

	saved, err = m.SetTaskEnabled(context.Background(), saved.ID, false)
	require.NoError(t, err)
	require.Nil(t, saved.NextDueAt)
	require.Empty(t, m.entries)
	require.False(t, m.cron.Entry(third.id).Valid())
	saved, err = m.SetTaskEnabled(context.Background(), saved.ID, true)
	require.NoError(t, err)
	require.NotNil(t, saved.NextDueAt)
	require.Len(t, m.entries, 1)
	require.Equal(t, newDue.UnixMilli(), m.entries[saved.ID].due)
	require.NoError(t, m.DeleteTask(context.Background(), saved.ID))
	require.Empty(t, m.entries)
	require.Zero(t, storedExecutions(t, db))
}

func TestManagerRebuildReadsDatabaseAndRemovesRoomDeletedTasks(t *testing.T) {
	db, path := claimDB(t)
	roomID := savedRoom(t, db)
	inputs := []time.Time{futureMinute(), futureMinute().Add(24 * time.Hour)}
	for _, due := range inputs {
		_, err := task.NewService(db).Create(context.Background(), taskInput(roomID, due))
		require.NoError(t, err)
	}
	// Verify reconstruction after a fresh connection, not from a process map.
	other, err := openTestDB(path)
	require.NoError(t, err)
	defer func() { require.NoError(t, other.Close()) }()
	m := NewManager(other, nil, readyHandler, nil)
	defer func() { <-m.Stop().Done() }()
	require.NoError(t, m.Start(context.Background()))
	require.Len(t, m.entries, 2)
	require.NoError(t, room.NewService(db, nil).Delete(context.Background(), roomID))
	require.NoError(t, m.Rebuild(context.Background()))
	require.Empty(t, m.entries)
	require.Empty(t, m.cron.Entries())
}

func openTestDB(path string) (*bun.DB, error) {
	return storage.Open(context.Background(), path)
}

func TestManagerCronClaimsOnceAndHandsOffOnlyWaitingExecution(t *testing.T) {
	db, _ := claimDB(t)
	roomID := savedRoom(t, db)
	due := time.Now().UTC().Add(3 * time.Second).Truncate(time.Second)
	handoffs := make(chan ClaimResult, 3)
	asyncErrors := make(chan error, 3)
	m := NewManager(db, nil, func(_ context.Context, result ClaimResult) error {
		handoffs <- result
		return nil
	}, func(err error) { asyncErrors <- err })
	defer func() { <-m.Stop().Done() }()
	taskRow, err := m.CreateTask(context.Background(), taskInput(roomID, due))
	require.NoError(t, err)
	require.NoError(t, m.Start(context.Background()))
	select {
	case result := <-handoffs:
		require.True(t, result.Claimed)
		require.Equal(t, "waiting", result.Status)
		require.Equal(t, due.UnixMilli(), result.ScheduledAt)
		require.Nil(t, result.NextDueAt)
		require.False(t, storedDue(t, db, taskRow.ID).Valid)
	case err := <-asyncErrors:
		t.Fatalf("scheduled callback error: %v", err)
	case <-time.After(6 * time.Second):
		t.Fatal("Cron never claimed the scheduled occurrence")
	}
	require.Zero(t, len(asyncErrors))
	require.Zero(t, len(handoffs))
	require.Equal(t, 1, storedExecutions(t, db))
	require.Empty(t, m.entries) // completed one-time registration removed
}

func TestManagerCronDailyReRegistersFromPersistedNextDue(t *testing.T) {
	db, _ := claimDB(t)
	roomID := savedRoom(t, db)
	due := time.Now().UTC().Add(3 * time.Second).Truncate(time.Second)
	created, err := task.NewService(db).Create(context.Background(), task.CreateInput{
		RoomID: roomID, Name: "daily", Kind: task.Single, Messages: []string{"secret"}, Enabled: true,
		Schedule: schedule.Rule{Kind: schedule.Daily, Timezone: "UTC", LocalTime: due.Format(time.TimeOnly)},
	})
	require.NoError(t, err)
	require.Equal(t, due.UnixMilli(), *created.NextDueAt)
	handoffs := make(chan ClaimResult, 2)
	m := NewManager(db, nil, func(_ context.Context, result ClaimResult) error {
		handoffs <- result
		return nil
	}, func(err error) { t.Errorf("unexpected callback error: %v", err) })
	defer func() { <-m.Stop().Done() }()
	require.NoError(t, m.Start(context.Background()))
	select {
	case result := <-handoffs:
		require.Equal(t, "waiting", result.Status)
		next := due.Add(24 * time.Hour).UnixMilli()
		require.Equal(t, next, *result.NextDueAt)
		require.Equal(t, next, m.entries[created.ID].due)
		require.Equal(t, next, storedDue(t, db, created.ID).Int64)
	case <-time.After(6 * time.Second):
		t.Fatal("Cron did not trigger the daily plan")
	}
}

func TestManagerReRegistersDSTFoldFromPersistedCalendar(t *testing.T) {
	db, _ := claimDB(t)
	// On November 5, 2028, New York's 01:30 happens at both 05:30Z
	// and 06:30Z. A3 picks the first and skips the second on the same day.
	due := time.Date(2028, 11, 5, 5, 30, 0, 0, time.UTC).UnixMilli()
	taskID := claimTask(t, db, "daily", "America/New_York", "", "01:30:00", nil, due)
	handoffs := make(chan ClaimResult, 2)
	m := NewManager(db, nil, func(_ context.Context, result ClaimResult) error {
		handoffs <- result
		return nil
	}, nil)
	defer func() { <-m.Stop().Done() }()
	m.claim.now = func() time.Time { return time.UnixMilli(due) }
	require.NoError(t, m.Rebuild(context.Background()))
	first := m.entries[taskID]
	m.fire(taskID, due, first.generation)
	result := <-handoffs
	require.Equal(t, "waiting", result.Status)
	next := time.Date(2028, 11, 6, 6, 30, 0, 0, time.UTC).UnixMilli()
	require.Equal(t, next, storedDue(t, db, taskID).Int64)
	require.Equal(t, next, m.entries[taskID].due)
	m.fire(taskID, due, first.generation) // retired timer cannot re-claim
	require.Len(t, handoffs, 0)
	require.Equal(t, 1, storedExecutions(t, db))
}

func TestManagerWeeklyRecurrenceKeepsCalendarWeekday(t *testing.T) {
	db, _ := claimDB(t)
	// 2028-11-06 is a Monday, with New York in standard time.
	due := time.Date(2028, 11, 6, 6, 30, 0, 0, time.UTC).UnixMilli()
	taskID := claimTask(t, db, "weekly", "America/New_York", "", "01:30:00", 1, due)
	handoffs := make(chan ClaimResult, 1)
	m := NewManager(db, nil, func(_ context.Context, result ClaimResult) error {
		handoffs <- result
		return nil
	}, nil)
	defer func() { <-m.Stop().Done() }()
	m.claim.now = func() time.Time { return time.UnixMilli(due) }
	require.NoError(t, m.Rebuild(context.Background()))
	m.fire(taskID, due, m.entries[taskID].generation)
	result := <-handoffs
	require.Equal(t, "waiting", result.Status)
	next := time.Date(2028, 11, 13, 6, 30, 0, 0, time.UTC).UnixMilli()
	require.Equal(t, next, *result.NextDueAt)
	require.Equal(t, next, m.entries[taskID].due)
}

func TestManagerClaimFailurePreservesCursorAndReportsAsyncError(t *testing.T) {
	db, _ := claimDB(t)
	due := futureMinute().UnixMilli()
	taskID := claimTask(t, db, "once", "UTC", time.UnixMilli(due).UTC().Format(time.DateOnly),
		time.UnixMilli(due).UTC().Format(time.TimeOnly), nil, due)
	reported := make(chan error, 1)
	handed := make(chan ClaimResult, 1)
	m := NewManager(db, nil, func(_ context.Context, result ClaimResult) error {
		handed <- result
		return nil
	}, func(err error) { reported <- err })
	defer func() { <-m.Stop().Done() }()
	m.claim.now = func() time.Time { return time.UnixMilli(due) }
	require.NoError(t, m.Rebuild(context.Background()))
	_, err := db.ExecContext(context.Background(), `CREATE TRIGGER reject_schedule
		BEFORE INSERT ON executions BEGIN SELECT RAISE(ABORT, 'injected failure'); END`)
	require.NoError(t, err)
	m.fire(taskID, due, m.entries[taskID].generation)
	select {
	case err := <-reported:
		require.ErrorContains(t, err, "claim scheduled occurrence")
	default:
		t.Fatal("callback failure was not reported")
	}
	require.NotNil(t, m.LastError())
	require.Empty(t, handed)
	require.Zero(t, storedExecutions(t, db))
	require.Equal(t, due, storedDue(t, db, taskID).Int64)
}

func TestManagerCommitErrorRebuildAndOverdueAreExplicit(t *testing.T) {
	db, _ := claimDB(t)
	roomID := savedRoom(t, db)
	due := futureMinute()
	m := NewManager(db, nil, readyHandler, nil)
	<-m.Stop().Done()
	saved, err := m.CreateTask(context.Background(), taskInput(roomID, due))
	require.ErrorIs(t, err, ErrScheduleSync)
	require.ErrorIs(t, err, ErrSchedulerClosed)
	require.Positive(t, saved.ID) // persistence already committed
	require.Equal(t, due.UnixMilli(), storedDue(t, db, saved.ID).Int64)
	require.Zero(t, storedExecutions(t, db))

	replacement := NewManager(db, nil, readyHandler, nil)
	defer func() { <-replacement.Stop().Done() }()
	require.NoError(t, replacement.Rebuild(context.Background()))
	require.Len(t, replacement.entries, 1)
	require.NoError(t, replacement.Start(context.Background()))

	// Simulate sleep/restart across a due time. E3 owns missed/late recovery,
	// so neither Start nor Rebuild may implicitly trigger overdue sends.
	replacement.now = func() time.Time { return due.Add(time.Minute) }
	require.ErrorIs(t, replacement.Rebuild(context.Background()), ErrRecoveryNeeded)
	require.Empty(t, replacement.entries)
	require.ErrorIs(t, replacement.LastError(), ErrRecoveryNeeded)
	require.Zero(t, storedExecutions(t, db))
	replacement.now = time.Now
	require.NoError(t, replacement.Rebuild(context.Background()))
	require.NoError(t, replacement.LastError())
}

func TestManagerHandoffErrorDoesNotRetryOrRevealMessage(t *testing.T) {
	db, _ := claimDB(t)
	roomID := savedRoom(t, db)
	due := time.Now().UTC().Add(3 * time.Second).Truncate(time.Second)
	logDir := filepath.Join(t.TempDir(), "logs")
	logger := diagnostic.New(logDir)
	defer logger.Close()
	reported := make(chan error, 2)
	handoffCalls := make(chan struct{}, 2)
	m := NewManager(db, logger, func(_ context.Context, _ ClaimResult) error {
		handoffCalls <- struct{}{}
		return errors.New("executor unavailable")
	}, func(err error) { reported <- err })
	defer func() { <-m.Stop().Done() }()
	_, err := m.CreateTask(context.Background(), taskInput(roomID, due))
	require.NoError(t, err)
	require.NoError(t, m.Start(context.Background()))
	select {
	case err := <-reported:
		require.ErrorContains(t, err, "executor unavailable")
	case <-time.After(6 * time.Second):
		t.Fatal("handoff failure not reported")
	}
	require.Len(t, handoffCalls, 1)
	require.ErrorContains(t, m.LastError(), "executor unavailable")
	require.Equal(t, 1, storedExecutions(t, db))
	require.Empty(t, m.entries)
	logger.Close()
	data, err := os.ReadFile(filepath.Join(logDir, "dan-tick.jsonl"))
	require.NoError(t, err)
	require.NotContains(t, string(data), "do not log message")
	require.NotContains(t, string(data), "executor unavailable")
	require.NotContains(t, strings.ToLower(string(data)), "cookie")
}

func TestManagerInvalidStoredPlanCanBeCorrectedAndRebuilt(t *testing.T) {
	db, _ := claimDB(t)
	roomID := savedRoom(t, db)
	due := futureMinute()
	saved, err := task.NewService(db).Create(context.Background(), taskInput(roomID, due))
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(), "UPDATE tasks SET timezone = 'Not/AZone' WHERE id = ?", saved.ID)
	require.NoError(t, err)
	logDir := filepath.Join(t.TempDir(), "logs")
	logger := diagnostic.New(logDir)
	defer logger.Close()
	m := NewManager(db, logger, readyHandler, nil)
	defer func() { <-m.Stop().Done() }()
	require.ErrorContains(t, m.Start(context.Background()), "invalid persisted")
	require.Empty(t, m.entries)
	require.NotNil(t, m.LastError())
	_, err = db.ExecContext(context.Background(), "UPDATE tasks SET timezone = 'UTC' WHERE id = ?", saved.ID)
	require.NoError(t, err)
	require.NoError(t, m.Start(context.Background()))
	require.Len(t, m.entries, 1)
	logger.Close()
	data, err := os.ReadFile(filepath.Join(logDir, "dan-tick.jsonl"))
	require.NoError(t, err)
	require.NotContains(t, string(data), "private")
	require.NotContains(t, string(data), "Not/AZone")
	var record map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(string(data))), &record))
	require.Equal(t, "schedule.sync_failed", record["event"])
	require.Equal(t, "schedule_sync", record["error_category"])
	require.NotContains(t, record, "error")
}

func TestManagerNeedsHandoffAndCannotRestart(t *testing.T) {
	db, _ := claimDB(t)
	m := NewManager(db, nil, nil, nil)
	require.ErrorIs(t, m.Start(context.Background()), ErrNoHandoff)
	<-m.Stop().Done()
	require.ErrorIs(t, m.Start(context.Background()), ErrSchedulerClosed)
	require.ErrorIs(t, m.Rebuild(context.Background()), ErrSchedulerClosed)
	require.Zero(t, storedExecutions(t, db))
}
