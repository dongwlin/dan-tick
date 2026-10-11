package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dongwlin/dan-tick/internal/diagnostic"
	"github.com/dongwlin/dan-tick/internal/storage"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

var scheduledTime = time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)

func claimDB(t *testing.T) (*bun.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scheduled.db")
	db, err := storage.Open(context.Background(), path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db, path
}

func claimTask(t *testing.T, db *bun.DB, kind, zone, date, clock string, weekdays any, due int64) int64 {
	t.Helper()
	ctx := context.Background()
	var roomID, taskID int64
	err := db.QueryRowContext(ctx, `
		INSERT INTO rooms (platform_room_id, created_at, updated_at)
		VALUES ('12345', 1, 1) RETURNING id`).Scan(&roomID)
	require.NoError(t, err)
	var localDate any
	if date != "" {
		localDate = date
	}
	err = db.QueryRowContext(ctx, `
		INSERT INTO tasks (room_id, name, kind, enabled, schedule_kind, timezone,
			local_date, local_time, weekdays, next_due_at, created_at, updated_at)
		VALUES (?, 'private task title', 'single', 1, ?, ?, ?, ?, ?, ?, 1, 1)
		RETURNING id`, roomID, kind, zone, localDate, clock, weekdays, due).Scan(&taskID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO task_messages (task_id, position, content)
		VALUES (?, 0, 'private message')`, taskID)
	require.NoError(t, err)
	return taskID
}

func newClaim(db *bun.DB, now time.Time) *ClaimService {
	s := NewClaimService(db, nil)
	s.now = func() time.Time { return now }
	return s
}

func storedDue(t *testing.T, db *bun.DB, taskID int64) sql.NullInt64 {
	t.Helper()
	var due sql.NullInt64
	require.NoError(t, db.QueryRowContext(context.Background(),
		"SELECT next_due_at FROM tasks WHERE id = ?", taskID).Scan(&due))
	return due
}

func storedExecutions(t *testing.T, db *bun.DB) int {
	t.Helper()
	var count int
	require.NoError(t, db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM executions").Scan(&count))
	return count
}

func TestClaimRegistersWaitingAndAdvancesFromPlannedInstant(t *testing.T) {
	db, _ := claimDB(t)
	due := scheduledTime.UnixMilli()
	taskID := claimTask(t, db, "daily", "Asia/Shanghai", "", "17:00:00", nil, due)
	// The callback arrived late; never derive the following occurrence from its arrival.
	now := scheduledTime.Add(2 * time.Hour)
	result, err := newClaim(db, now).Claim(context.Background(), taskID, due)
	require.NoError(t, err)
	require.True(t, result.Claimed)
	require.Positive(t, result.ExecutionID)
	require.Equal(t, due, result.ScheduledAt)
	require.Equal(t, "waiting", result.Status)
	require.Empty(t, result.ErrorCode)
	require.NotNil(t, result.NextDueAt)
	require.Equal(t, scheduledTime.Add(24*time.Hour).UnixMilli(), *result.NextDueAt)
	require.Equal(t, *result.NextDueAt, storedDue(t, db, taskID).Int64)

	var trigger, status, title, room string
	var scheduledAt, createdAt int64
	var startedAt, finishedAt, queuedAt sql.NullInt64
	var errorCode sql.NullString
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT trigger_kind, status, task_name_snapshot, platform_room_id_snapshot,
			scheduled_at, created_at, started_at, finished_at, queued_at, error_code
		FROM executions WHERE id = ?`, result.ExecutionID).Scan(&trigger, &status, &title, &room,
		&scheduledAt, &createdAt, &startedAt, &finishedAt, &queuedAt, &errorCode))
	require.Equal(t, "scheduled", trigger)
	require.Equal(t, "waiting", status)
	require.Equal(t, "private task title", title)
	require.Equal(t, "12345", room)
	require.Equal(t, due, scheduledAt)
	require.Equal(t, now.UnixMilli(), createdAt)
	require.False(t, startedAt.Valid)
	require.False(t, finishedAt.Valid)
	require.False(t, queuedAt.Valid)
	require.False(t, errorCode.Valid)
	require.Equal(t, 1, storedExecutions(t, db))
	var attempts int
	require.NoError(t, db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM send_attempts WHERE execution_id = ?", result.ExecutionID).Scan(&attempts))
	require.Zero(t, attempts) // E1 does not send or prepare a pending HTTP attempt.
}

func TestClaimOneTimeTaskClearsCursor(t *testing.T) {
	db, _ := claimDB(t)
	due := scheduledTime.UnixMilli()
	taskID := claimTask(t, db, "once", "UTC", "2026-10-11", "09:00:00", nil, due)
	result, err := newClaim(db, scheduledTime).Claim(context.Background(), taskID, due)
	require.NoError(t, err)
	require.True(t, result.Claimed)
	require.Equal(t, "waiting", result.Status)
	require.Nil(t, result.NextDueAt)
	require.False(t, storedDue(t, db, taskID).Valid)
}

func TestClaimConflictsBecomeMissedWithoutOverlappingExecution(t *testing.T) {
	for _, activeStatus := range []string{"waiting", "queued", "running"} {
		t.Run(activeStatus, func(t *testing.T) {
			db, _ := claimDB(t)
			due := scheduledTime.UnixMilli()
			taskID := claimTask(t, db, "daily", "UTC", "", "09:00:00", nil, due)
			var activeID int64
			err := db.QueryRowContext(context.Background(), `
				INSERT INTO executions (task_id, trigger_kind, status, created_at,
					task_name_snapshot, platform_room_id_snapshot)
				VALUES (?, 'manual', ?, 1, 'existing', '12345') RETURNING id`, taskID, activeStatus).Scan(&activeID)
			require.NoError(t, err)
			now := scheduledTime.Add(time.Second)
			result, err := newClaim(db, now).Claim(context.Background(), taskID, due)
			require.NoError(t, err)
			require.True(t, result.Claimed)
			require.Equal(t, "missed", result.Status)
			require.Equal(t, "task_busy", result.ErrorCode)
			require.Equal(t, scheduledTime.Add(24*time.Hour).UnixMilli(), storedDue(t, db, taskID).Int64)
			var status, errorCode string
			var finishedAt int64
			require.NoError(t, db.QueryRowContext(context.Background(), `
				SELECT status, error_code, finished_at FROM executions WHERE id = ?`,
				result.ExecutionID).Scan(&status, &errorCode, &finishedAt))
			require.Equal(t, "missed", status)
			require.Equal(t, "task_busy", errorCode)
			require.Equal(t, now.UnixMilli(), finishedAt)
			require.NotEqual(t, activeID, result.ExecutionID)
			var live int
			require.NoError(t, db.QueryRowContext(context.Background(), `
				SELECT COUNT(*) FROM executions WHERE task_id = ?
				AND status IN ('waiting','queued','running')`, taskID).Scan(&live))
			require.Equal(t, 1, live)
		})
	}
}

func TestClaimIgnoresCallbacksWithoutCurrentDuePlan(t *testing.T) {
	for _, tc := range []struct {
		name, change string
		early        bool
		expected     int64
	}{
		{name: "stale identity", change: "UPDATE tasks SET next_due_at = next_due_at + 1000", expected: 1000},
		{name: "disabled", change: "UPDATE tasks SET enabled = 0"},
		{name: "deleted task", change: "UPDATE tasks SET deleted_at = 1"},
		{name: "no due", change: "UPDATE tasks SET next_due_at = NULL"},
		{name: "deleted room", change: "UPDATE rooms SET deleted_at = 1"},
		{name: "early callback", early: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, _ := claimDB(t)
			due := scheduledTime.UnixMilli()
			taskID := claimTask(t, db, "daily", "UTC", "", "09:00:00", nil, due)
			if tc.change != "" {
				_, err := db.ExecContext(context.Background(), tc.change)
				require.NoError(t, err)
			}
			now := scheduledTime
			if tc.early {
				now = now.Add(-time.Millisecond)
			}
			result, err := newClaim(db, now).Claim(context.Background(), taskID, due)
			require.NoError(t, err)
			require.Equal(t, ClaimResult{}, result)
			require.Zero(t, storedExecutions(t, db))
			if tc.change != "UPDATE tasks SET next_due_at = NULL" {
				require.Equal(t, due+tc.expected, storedDue(t, db, taskID).Int64)
			}
		})
	}
}

func TestClaimConcurrentCallbacksAndReopenAreIdempotent(t *testing.T) {
	db, path := claimDB(t)
	due := scheduledTime.UnixMilli()
	taskID := claimTask(t, db, "daily", "UTC", "", "09:00:00", nil, due)
	secondDB, err := storage.Open(context.Background(), path)
	require.NoError(t, err)
	defer func() { require.NoError(t, secondDB.Close()) }()
	const callers = 14
	results := make(chan ClaimResult, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			conn := db
			if i%2 != 0 {
				conn = secondDB
			}
			r, err := newClaim(conn, scheduledTime).Claim(context.Background(), taskID, due)
			results <- r
			errs <- err
		}(i)
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	claimed := 0
	for result := range results {
		if result.Claimed {
			claimed++
			require.Equal(t, "waiting", result.Status)
		} else {
			require.Equal(t, ClaimResult{}, result)
		}
	}
	require.Equal(t, 1, claimed)
	require.Equal(t, 1, storedExecutions(t, db))

	// A new process (or Service) sees the cursor and persistent unique key,
	// not merely an in-process deduplication map.
	reopened, err := storage.Open(context.Background(), path)
	require.NoError(t, err)
	defer func() { require.NoError(t, reopened.Close()) }()
	result, err := newClaim(reopened, scheduledTime).Claim(context.Background(), taskID, due)
	require.NoError(t, err)
	require.Equal(t, ClaimResult{}, result)
	require.Equal(t, 1, storedExecutions(t, reopened))
}

func TestClaimEmitsOnlySafeScheduledEvents(t *testing.T) {
	db, _ := claimDB(t)
	due := scheduledTime.UnixMilli()
	taskID := claimTask(t, db, "daily", "UTC", "", "09:00:00", nil, due)
	var activeID int64
	require.NoError(t, db.QueryRowContext(context.Background(), `
		INSERT INTO executions (task_id, trigger_kind, status, created_at,
			task_name_snapshot, platform_room_id_snapshot)
		VALUES (?, 'manual', 'running', 1, 'existing', '12345') RETURNING id`, taskID).Scan(&activeID))
	dir := filepath.Join(t.TempDir(), "logs")
	logger := diagnostic.New(dir)
	claim := newClaim(db, scheduledTime)
	claim.log = logger
	conflict, err := claim.Claim(context.Background(), taskID, due)
	require.NoError(t, err)
	require.Equal(t, "missed", conflict.Status)
	_, err = db.ExecContext(context.Background(),
		"UPDATE executions SET status = 'completed', finished_at = ? WHERE id = ?", due, activeID)
	require.NoError(t, err)
	next := *conflict.NextDueAt
	claim.now = func() time.Time { return time.UnixMilli(next) }
	accepted, err := claim.Claim(context.Background(), taskID, next)
	require.NoError(t, err)
	require.Equal(t, "waiting", accepted.Status)
	// A stale callback must not appear as another successful claim.
	ignored, err := claim.Claim(context.Background(), taskID, next)
	require.NoError(t, err)
	require.False(t, ignored.Claimed)
	logger.Close()
	data, err := os.ReadFile(filepath.Join(dir, "dan-tick.jsonl"))
	require.NoError(t, err)
	for _, sensitive := range []string{"private", "message", "content", "SESSDATA", "platform_room_id"} {
		require.NotContains(t, string(data), sensitive)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 2)
	for i, line := range lines {
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		require.Equal(t, "scheduler", record["component"])
		require.EqualValues(t, taskID, record["task_id"])
		require.NotContains(t, record, "error")
		if i == 0 {
			require.Equal(t, "schedule.conflict", record["event"])
			require.Equal(t, "missed", record["result"])
			require.Equal(t, "task_busy", record["error_category"])
			require.EqualValues(t, due, record["scheduled_at"])
			require.EqualValues(t, conflict.ExecutionID, record["execution_id"])
		} else {
			require.Equal(t, "schedule.claimed", record["event"])
			require.Equal(t, "waiting", record["result"])
			require.NotContains(t, record, "error_category")
			require.EqualValues(t, next, record["scheduled_at"])
			require.EqualValues(t, accepted.ExecutionID, record["execution_id"])
		}
	}
}

func TestClaimInsertFailureRollsBackCursorAndCanRetry(t *testing.T) {
	db, _ := claimDB(t)
	due := scheduledTime.UnixMilli()
	taskID := claimTask(t, db, "daily", "UTC", "", "09:00:00", nil, due)
	_, err := db.ExecContext(context.Background(), `CREATE TRIGGER reject_claim BEFORE INSERT ON executions
		BEGIN SELECT RAISE(ABORT, 'injected failure'); END`)
	require.NoError(t, err)
	result, err := newClaim(db, scheduledTime).Claim(context.Background(), taskID, due)
	require.Error(t, err)
	require.Equal(t, ClaimResult{}, result)
	require.Equal(t, due, storedDue(t, db, taskID).Int64)
	require.Zero(t, storedExecutions(t, db))
	_, err = db.ExecContext(context.Background(), "DROP TRIGGER reject_claim")
	require.NoError(t, err)
	result, err = newClaim(db, scheduledTime).Claim(context.Background(), taskID, due)
	require.NoError(t, err)
	require.True(t, result.Claimed)
}

func TestClaimPersistentUniqueKeyPreventsReclaimAfterCursorRollback(t *testing.T) {
	db, _ := claimDB(t)
	due := scheduledTime.UnixMilli()
	taskID := claimTask(t, db, "daily", "UTC", "", "09:00:00", nil, due)
	first, err := newClaim(db, scheduledTime).Claim(context.Background(), taskID, due)
	require.NoError(t, err)
	// Simulate an external corruption/clock-reset repair that rewound the cursor.
	_, err = db.ExecContext(context.Background(), `UPDATE tasks SET next_due_at = ? WHERE id = ?`, due, taskID)
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(), `UPDATE executions SET status = 'completed', finished_at = ? WHERE id = ?`, due, first.ExecutionID)
	require.NoError(t, err)
	result, err := newClaim(db, scheduledTime).Claim(context.Background(), taskID, due)
	require.Error(t, err) // unique scheduled identity rejects a second row
	require.Equal(t, ClaimResult{}, result)
	require.Equal(t, due, storedDue(t, db, taskID).Int64)
	require.Equal(t, 1, storedExecutions(t, db))
}

func TestClaimDSTRepeatedHourUsesOnlyFirstOccurrence(t *testing.T) {
	db, _ := claimDB(t)
	// 01:30 America/New_York occurs twice on the fall-back day. The first
	// occurrence is 05:30Z; the next scheduled day is 06:30Z.
	due := time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC).UnixMilli()
	taskID := claimTask(t, db, "daily", "America/New_York", "", "01:30:00", nil, due)
	result, err := newClaim(db, time.UnixMilli(due)).Claim(context.Background(), taskID, due)
	require.NoError(t, err)
	require.True(t, result.Claimed)
	require.Equal(t, time.Date(2026, 11, 2, 6, 30, 0, 0, time.UTC).UnixMilli(), *result.NextDueAt)
	require.NotEqual(t, time.Date(2026, 11, 1, 6, 30, 0, 0, time.UTC).UnixMilli(), *result.NextDueAt)
}

func TestClaimInvalidInputDoesNotTouchDatabase(t *testing.T) {
	db, _ := claimDB(t)
	for _, pair := range [][2]int64{{0, 1}, {1, 0}, {-1, 1}} {
		result, err := newClaim(db, scheduledTime).Claim(context.Background(), pair[0], pair[1])
		require.ErrorIs(t, err, ErrInvalidOccurrence)
		require.Equal(t, ClaimResult{}, result)
	}
}
