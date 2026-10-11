package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dongwlin/dan-tick/internal/diagnostic"
	"github.com/dongwlin/dan-tick/internal/schedule"
	"github.com/dongwlin/dan-tick/internal/storage"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func seedRecoveryTask(t *testing.T, db *bun.DB, roomID int64, rule schedule.Rule, due int64, enabled bool) int64 {
	t.Helper()
	var localDate, weekdays any
	if rule.Kind == schedule.Once {
		localDate = rule.LocalDate
	}
	if rule.Kind == schedule.Weekly {
		weekdays = rule.Weekdays
	}
	var id int64
	require.NoError(t, db.QueryRowContext(context.Background(), `
		INSERT INTO tasks (room_id, name, kind, enabled, schedule_kind, timezone,
			local_date, local_time, weekdays, next_due_at, created_at, updated_at)
		VALUES (?, 'private task', 'single', ?, ?, ?, ?, ?, ?, ?, 1, 1) RETURNING id`,
		roomID, enabled, rule.Kind, rule.Timezone, localDate, rule.LocalTime, weekdays, due).Scan(&id))
	return id
}

func seedActiveExecution(t *testing.T, db *bun.DB, taskID int64, status string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, db.QueryRowContext(context.Background(), `
		INSERT INTO executions (task_id, trigger_kind, status, created_at,
			task_name_snapshot, platform_room_id_snapshot)
		VALUES (?, 'manual', ?, 1, 'private task', '12345') RETURNING id`, taskID, status).Scan(&id))
	return id
}

func seedAttempt(t *testing.T, db *bun.DB, executionID int64, seq, messageNo int, result string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO send_attempts (execution_id, sequence, message_no, content_snapshot,
			attempted_at, result) VALUES (?, ?, ?, 'private sensitive message', 1, ?)`,
		executionID, seq, messageNo, result)
	require.NoError(t, err)
}

func testRecovery(db *bun.DB, at time.Time, log *diagnostic.Logger) *RecoveryService {
	s := NewRecoveryService(db, log)
	s.now = func() time.Time { return at }
	return s
}

func TestStartupRecoveryReopensAndFinishesAbandonedExecutions(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "recover.db")
	db, err := storage.Open(ctx, path)
	require.NoError(t, err)
	roomID := savedRoom(t, db)
	due := time.Date(2026, 12, 1, 9, 0, 0, 0, time.UTC)
	rule := schedule.Rule{Kind: schedule.Daily, Timezone: "UTC", LocalTime: "09:00:00"}
	var ids []int64
	for _, status := range []string{"waiting", "queued", "running"} {
		taskID := seedRecoveryTask(t, db, roomID, rule, due.UnixMilli(), false)
		ids = append(ids, seedActiveExecution(t, db, taskID, status))
	}
	seedAttempt(t, db, ids[1], 1, 1, "accepted")
	seedAttempt(t, db, ids[1], 2, 2, "pending")
	seedAttempt(t, db, ids[2], 1, 1, "pending")
	// A prior retry on the same logical message must not double-count failures.
	seedAttempt(t, db, ids[2], 2, 1, "pending")
	require.NoError(t, db.Close())

	reopened, err := storage.Open(ctx, path)
	require.NoError(t, err)
	defer func() { require.NoError(t, reopened.Close()) }()
	at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	summary, err := testRecovery(reopened, at, nil).RecoverStartup(ctx)
	require.NoError(t, err)
	require.Equal(t, RecoverySummary{Interrupted: 3, Unknown: 3}, summary)
	for index, id := range ids {
		var status, code string
		var finished int64
		var sent, failed int
		require.NoError(t, reopened.QueryRowContext(ctx, `
			SELECT status, error_code, finished_at, sent_count, failed_count
			FROM executions WHERE id = ?`, id).Scan(&status, &code, &finished, &sent, &failed))
		require.Equal(t, "interrupted", status)
		require.Equal(t, "recovery_interrupted", code)
		require.Equal(t, at.UnixMilli(), finished)
		require.Equal(t, []int{0, 1, 0}[index], sent)
		require.Equal(t, []int{0, 1, 1}[index], failed)
	}
	for _, id := range ids[1:] {
		var pending int
		require.NoError(t, reopened.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM send_attempts WHERE execution_id = ? AND result = 'pending'`, id).Scan(&pending))
		require.Zero(t, pending)
	}
	var unknown int
	require.NoError(t, reopened.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM send_attempts WHERE result = 'unknown'
		AND error_code = 'recovery_unknown'`).Scan(&unknown))
	require.Equal(t, 3, unknown)
	for _, id := range ids {
		var next sql.NullInt64
		require.NoError(t, reopened.QueryRowContext(ctx, `
			SELECT next_due_at FROM tasks WHERE id =
			(SELECT task_id FROM executions WHERE id = ?)`, id).Scan(&next))
		require.Equal(t, due.UnixMilli(), next.Int64) // manual runs do not consume plans
	}
	again, err := testRecovery(reopened, at.Add(time.Minute), nil).RecoverStartup(ctx)
	require.NoError(t, err)
	require.Equal(t, RecoverySummary{}, again)
}

func TestRecoverySkipsYearsOfOfflineOccurrencesWithoutReplay(t *testing.T) {
	ctx := context.Background()
	db, _ := claimDB(t)
	roomID := savedRoom(t, db)
	at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	once := schedule.Rule{Kind: schedule.Once, Timezone: "Asia/Shanghai",
		LocalDate: "2024-06-01", LocalTime: "10:00:00"}
	daily := schedule.Rule{Kind: schedule.Daily, Timezone: "America/New_York", LocalTime: "02:30:00"}
	weekly := schedule.Rule{Kind: schedule.Weekly, Timezone: "Asia/Shanghai", LocalTime: "20:00:00", Weekdays: 1}
	plans := []struct {
		rule schedule.Rule
		due  time.Time
	}{
		{once, time.Date(2024, 6, 1, 2, 0, 0, 0, time.UTC)},
		{daily, time.Date(2023, 11, 5, 7, 30, 0, 0, time.UTC)},
		{weekly, time.Date(2021, 10, 11, 12, 0, 0, 0, time.UTC)},
	}
	var ids []int64
	for _, p := range plans {
		ids = append(ids, seedRecoveryTask(t, db, roomID, p.rule, p.due.UnixMilli(), true))
	}
	// Inactive/removed work must never produce a missed execution.
	disabled := seedRecoveryTask(t, db, roomID, once, plans[0].due.UnixMilli(), false)
	logDir := filepath.Join(t.TempDir(), "logs")
	logger := diagnostic.New(logDir)
	defer logger.Close()
	summary, err := testRecovery(db, at, logger).RecoverStartup(ctx)
	require.NoError(t, err)
	require.Equal(t, RecoverySummary{Missed: 3}, summary)
	require.Equal(t, 3, storedExecutions(t, db))
	for i, id := range ids {
		var status, code, snapshot string
		var scheduled, finished int64
		require.NoError(t, db.QueryRowContext(ctx, `
			SELECT status, error_code, scheduled_at, finished_at,
				task_name_snapshot FROM executions WHERE task_id = ?`, id).
			Scan(&status, &code, &scheduled, &finished, &snapshot))
		require.Equal(t, "missed", status)
		require.Equal(t, "schedule_overdue", code)
		require.Equal(t, plans[i].due.UnixMilli(), scheduled)
		require.Equal(t, at.UnixMilli(), finished)
		require.Equal(t, "private task", snapshot)
		next := storedDue(t, db, id)
		if plans[i].rule.Kind == schedule.Once {
			require.False(t, next.Valid)
		} else {
			want, valid, err := schedule.NextAfter(plans[i].rule, at)
			require.NoError(t, err)
			require.True(t, valid)
			require.Equal(t, want.UnixMilli(), next.Int64)
			require.Greater(t, next.Int64, at.UnixMilli())
		}
	}
	require.Equal(t, plans[0].due.UnixMilli(), storedDue(t, db, disabled).Int64)
	require.Equal(t, RecoverySummary{}, func() RecoverySummary {
		again, err := testRecovery(db, at, logger).RecoverStartup(ctx)
		require.NoError(t, err)
		return again
	}())
	logger.Close()
	data, err := os.ReadFile(filepath.Join(logDir, "dan-tick.jsonl"))
	require.NoError(t, err)
	require.Len(t, strings.Split(strings.TrimSpace(string(data)), "\n"), 3)
	require.NotContains(t, string(data), "private")
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		require.Equal(t, "schedule.missed", record["event"])
		require.Equal(t, "schedule_overdue", record["error_category"])
		require.NotContains(t, record, "message")
	}
}

func TestStartupRecoveryIsAtomicWhenMissedInsertFails(t *testing.T) {
	ctx := context.Background()
	db, _ := claimDB(t)
	roomID := savedRoom(t, db)
	due := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	rule := schedule.Rule{Kind: schedule.Once, Timezone: "UTC",
		LocalDate: "2026-10-11", LocalTime: "09:00:00"}
	taskID := seedRecoveryTask(t, db, roomID, rule, due.UnixMilli(), true)
	runID := seedActiveExecution(t, db, taskID, "running")
	seedAttempt(t, db, runID, 1, 1, "pending")
	logDir := filepath.Join(t.TempDir(), "logs")
	logger := diagnostic.New(logDir)
	_, err := db.ExecContext(ctx, `CREATE TRIGGER fail_miss BEFORE INSERT ON executions
		BEGIN SELECT RAISE(ABORT, 'injected failure'); END`)
	require.NoError(t, err)
	at := due.Add(24 * time.Hour)
	recovery := testRecovery(db, at, logger)
	summary, err := recovery.RecoverStartup(ctx)
	require.Error(t, err)
	require.Equal(t, RecoverySummary{}, summary)
	var status, result string
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT status FROM executions WHERE id = ?`, runID).Scan(&status))
	require.Equal(t, "running", status)
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT result FROM send_attempts WHERE execution_id = ?`, runID).Scan(&result))
	require.Equal(t, "pending", result)
	require.Equal(t, due.UnixMilli(), storedDue(t, db, taskID).Int64)
	require.Equal(t, 1, storedExecutions(t, db))
	logger.Close()
	data, err := os.ReadFile(filepath.Join(logDir, "dan-tick.jsonl"))
	require.NoError(t, err)
	var entry map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(string(data))), &entry))
	require.Equal(t, "recovery.failed", entry["event"])
	require.Equal(t, "persistence", entry["error_category"])
	require.NotContains(t, entry, "error")
	require.NotContains(t, string(data), "private")
	recovery.log = nil
	_, err = db.ExecContext(ctx, "DROP TRIGGER fail_miss")
	require.NoError(t, err)
	summary, err = recovery.RecoverStartup(ctx)
	require.NoError(t, err)
	require.Equal(t, RecoverySummary{Interrupted: 1, Unknown: 1, Missed: 1}, summary)
	require.Equal(t, 2, storedExecutions(t, db))
}

func TestRecoveryRewoundCursorReusesScheduledIdentity(t *testing.T) {
	db, _ := claimDB(t)
	ctx := context.Background()
	roomID := savedRoom(t, db)
	due := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	taskID := seedRecoveryTask(t, db, roomID,
		schedule.Rule{Kind: schedule.Daily, Timezone: "UTC", LocalTime: "09:00:00"}, due.UnixMilli(), true)
	first, err := newClaim(db, due).Claim(ctx, taskID, due.UnixMilli())
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE executions SET status = 'completed', finished_at = ? WHERE id = ?`,
		due.UnixMilli(), first.ExecutionID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "UPDATE tasks SET next_due_at = ? WHERE id = ?", due.UnixMilli(), taskID)
	require.NoError(t, err)
	recovery := testRecovery(db, due.Add(48*time.Hour), nil)
	result, err := recovery.RecoverStartup(ctx)
	require.NoError(t, err)
	require.Equal(t, RecoverySummary{}, result)
	require.Equal(t, 1, storedExecutions(t, db))
	require.Greater(t, storedDue(t, db, taskID).Int64, due.Add(48*time.Hour).UnixMilli())
}

func TestStartupRecoveryLogsSafeCommittedEventsOnly(t *testing.T) {
	ctx := context.Background()
	db, _ := claimDB(t)
	roomID := savedRoom(t, db)
	due := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	taskID := seedRecoveryTask(t, db, roomID,
		schedule.Rule{Kind: schedule.Once, Timezone: "UTC",
			LocalDate: "2026-10-11", LocalTime: "09:00:00"}, due.UnixMilli(), true)
	runID := seedActiveExecution(t, db, taskID, "running")
	seedAttempt(t, db, runID, 1, 1, "pending")
	logDir := filepath.Join(t.TempDir(), "logs")
	logger := diagnostic.New(logDir)
	at := due.Add(48 * time.Hour)
	summary, err := testRecovery(db, at, logger).RecoverStartup(ctx)
	require.NoError(t, err)
	require.Equal(t, RecoverySummary{Interrupted: 1, Unknown: 1, Missed: 1}, summary)
	logger.Close()
	data, err := os.ReadFile(filepath.Join(logDir, "dan-tick.jsonl"))
	require.NoError(t, err)
	for _, sensitive := range []string{"private", "content", "SESSDATA", "cookie", "response", "request"} {
		require.NotContains(t, string(data), sensitive)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 3)
	want := []struct{ event, code string }{
		{"send.resolved", "recovery_unknown"},
		{"execution.interrupted", "recovery_interrupted"},
		{"schedule.missed", "schedule_overdue"},
	}
	for i, line := range lines {
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		require.Equal(t, want[i].event, entry["event"])
		require.Equal(t, want[i].code, entry["error_category"])
		require.EqualValues(t, taskID, entry["task_id"])
		require.NotContains(t, entry, "error")
	}
	var attemptEntry map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &attemptEntry))
	require.EqualValues(t, runID, attemptEntry["execution_id"])
	require.EqualValues(t, 1, attemptEntry["attempt_sequence"])
}

func TestRecoverySkipsNonexistentSpringDSTTime(t *testing.T) {
	db, _ := claimDB(t)
	roomID := savedRoom(t, db)
	rule := schedule.Rule{Kind: schedule.Daily, Timezone: "America/New_York", LocalTime: "02:30:00"}
	// On 2027-03-14 clocks skip 02:30 in New York.
	due := time.Date(2027, 3, 13, 7, 30, 0, 0, time.UTC)
	id := seedRecoveryTask(t, db, roomID, rule, due.UnixMilli(), true)
	at := time.Date(2027, 3, 14, 7, 0, 0, 0, time.UTC)
	summary, err := testRecovery(db, at, nil).RecoverAfterWake(context.Background())
	require.NoError(t, err)
	require.Equal(t, RecoverySummary{Missed: 1}, summary)
	require.Equal(t, time.Date(2027, 3, 15, 6, 30, 0, 0, time.UTC).UnixMilli(),
		storedDue(t, db, id).Int64)
}

func TestManagerStartAutomaticallyRecoversBeforeCronRegistration(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "start.db")
	db, err := storage.Open(ctx, path)
	require.NoError(t, err)
	roomID := savedRoom(t, db)
	due := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	taskID := seedRecoveryTask(t, db, roomID,
		schedule.Rule{Kind: schedule.Daily, Timezone: "UTC", LocalTime: "09:00:00"},
		due.UnixMilli(), true)
	activeID := seedActiveExecution(t, db, taskID, "queued")
	seedAttempt(t, db, activeID, 1, 1, "pending")
	require.NoError(t, db.Close())

	reopened, err := storage.Open(ctx, path)
	require.NoError(t, err)
	defer func() { require.NoError(t, reopened.Close()) }()
	handoffs := make(chan ClaimResult, 1)
	m := NewManager(reopened, nil, func(_ context.Context, result ClaimResult) error {
		handoffs <- result
		return nil
	}, nil)
	defer func() { <-m.Stop().Done() }()
	// Start exercises the actual recovery-before-registration integration.
	now := due.Add(72 * time.Hour)
	m.now = func() time.Time { return now }
	m.recovery.now = func() time.Time { return now }
	require.NoError(t, m.Start(ctx))
	require.Len(t, m.entries, 1)
	require.Greater(t, m.entries[taskID].due, now.UnixMilli())
	require.Empty(t, handoffs)
	var status, attempt string
	require.NoError(t, reopened.QueryRowContext(ctx, "SELECT status FROM executions WHERE id = ?", activeID).Scan(&status))
	require.Equal(t, "interrupted", status)
	require.NoError(t, reopened.QueryRowContext(ctx,
		"SELECT result FROM send_attempts WHERE execution_id = ?", activeID).Scan(&attempt))
	require.Equal(t, "unknown", attempt)
	require.Equal(t, 2, storedExecutions(t, reopened))
	require.NoError(t, m.Start(ctx))
	require.Equal(t, 2, storedExecutions(t, reopened))
}

func TestStartupInterruptsClaimedScheduledWaitingWithoutReplaying(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "claimed.db")
	db, err := storage.Open(ctx, path)
	require.NoError(t, err)
	roomID := savedRoom(t, db)
	due := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	taskID := seedRecoveryTask(t, db, roomID,
		schedule.Rule{Kind: schedule.Daily, Timezone: "UTC", LocalTime: "09:00:00"},
		due.UnixMilli(), true)
	claim, err := newClaim(db, due).Claim(ctx, taskID, due.UnixMilli())
	require.NoError(t, err)
	require.True(t, claim.Claimed)
	require.Equal(t, "waiting", claim.Status)
	originalNext := *claim.NextDueAt
	require.NoError(t, db.Close())

	reopened, err := storage.Open(ctx, path)
	require.NoError(t, err)
	defer func() { require.NoError(t, reopened.Close()) }()
	summary, err := testRecovery(reopened, due.Add(time.Hour), nil).RecoverStartup(ctx)
	require.NoError(t, err)
	require.Equal(t, RecoverySummary{Interrupted: 1}, summary)
	var status string
	var finished int64
	require.NoError(t, reopened.QueryRowContext(ctx, `
		SELECT status, finished_at FROM executions WHERE id = ?`, claim.ExecutionID).
		Scan(&status, &finished))
	require.Equal(t, "interrupted", status)
	require.Equal(t, due.Add(time.Hour).UnixMilli(), finished)
	require.Equal(t, originalNext, storedDue(t, reopened, taskID).Int64)
	require.Equal(t, 1, storedExecutions(t, reopened))
	var attempts int
	require.NoError(t, reopened.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM send_attempts WHERE execution_id = ?", claim.ExecutionID).Scan(&attempts))
	require.Zero(t, attempts)
	again, err := testRecovery(reopened, due.Add(2*time.Hour), nil).RecoverStartup(ctx)
	require.NoError(t, err)
	require.Equal(t, RecoverySummary{}, again)
}

func TestStartupDoesNotCatchUpEvenWithinLiveCallbackGrace(t *testing.T) {
	db, _ := claimDB(t)
	roomID := savedRoom(t, db)
	due := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	taskID := seedRecoveryTask(t, db, roomID,
		schedule.Rule{Kind: schedule.Once, Timezone: "UTC",
			LocalDate: "2026-10-11", LocalTime: "09:00:00"}, due.UnixMilli(), true)
	summary, err := testRecovery(db, due.Add(time.Millisecond), nil).RecoverStartup(context.Background())
	require.NoError(t, err)
	require.Equal(t, RecoverySummary{Missed: 1}, summary)
	var status string
	require.NoError(t, db.QueryRowContext(context.Background(),
		"SELECT status FROM executions WHERE task_id = ?", taskID).Scan(&status))
	require.Equal(t, "missed", status)
	require.False(t, storedDue(t, db, taskID).Valid)
}

func TestManagerLiveCronGraceAndOverdue(t *testing.T) {
	for _, tc := range []struct {
		name    string
		delay   time.Duration
		outcome string
		handoff int
	}{
		{"on time", 0, "waiting", 1},
		{"within grace", CallbackGrace, "waiting", 1},
		{"past grace", CallbackGrace + time.Millisecond, "missed", 0},
		{"long sleep", 365 * 24 * time.Hour, "missed", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, _ := claimDB(t)
			roomID := savedRoom(t, db)
			due := time.Date(2028, 11, 1, 9, 0, 0, 0, time.UTC)
			taskID := seedRecoveryTask(t, db, roomID,
				schedule.Rule{Kind: schedule.Daily, Timezone: "UTC", LocalTime: "09:00:00"},
				due.UnixMilli(), true)
			delivered := make(chan ClaimResult, 2)
			m := NewManager(db, nil, func(_ context.Context, result ClaimResult) error {
				delivered <- result
				return nil
			}, nil)
			defer func() { <-m.Stop().Done() }()
			at := due.Add(tc.delay)
			m.now = func() time.Time { return due.Add(-time.Minute) } // allows registration
			m.claim.now = func() time.Time { return at }
			m.recovery.now = func() time.Time { return at }
			require.NoError(t, m.Rebuild(context.Background()))
			reg := m.entries[taskID]
			m.fire(taskID, due.UnixMilli(), reg.generation)
			require.Len(t, delivered, tc.handoff)
			var status string
			require.NoError(t, db.QueryRowContext(context.Background(),
				"SELECT status FROM executions WHERE task_id = ?", taskID).Scan(&status))
			require.Equal(t, tc.outcome, status)
			require.Greater(t, storedDue(t, db, taskID).Int64, at.UnixMilli())
			// A stale wakeup cannot replay the prior occurrence.
			m.fire(taskID, due.UnixMilli(), reg.generation)
			require.Equal(t, 1, storedExecutions(t, db))
			require.Len(t, delivered, tc.handoff)
		})
	}
}

func TestResumeDoesNotInterruptStillRunningExecution(t *testing.T) {
	db, _ := claimDB(t)
	ctx := context.Background()
	roomID := savedRoom(t, db)
	base := time.Date(2028, 11, 1, 9, 0, 0, 0, time.UTC)
	due := base.Add(time.Minute)
	taskID := seedRecoveryTask(t, db, roomID,
		schedule.Rule{Kind: schedule.Daily, Timezone: "UTC", LocalTime: "09:01:00"},
		due.UnixMilli(), true)
	m := NewManager(db, nil, readyHandler, nil)
	defer func() { <-m.Stop().Done() }()
	now := base
	m.now = func() time.Time { return now }
	m.recovery.now = func() time.Time { return now }
	require.NoError(t, m.Start(ctx))
	id := seedActiveExecution(t, db, taskID, "running") // started in this process
	seedAttempt(t, db, id, 1, 1, "pending")
	now = base.Add(3 * time.Hour) // simulated wake
	result, err := m.Resume(ctx)
	require.NoError(t, err)
	require.Equal(t, RecoverySummary{Missed: 1}, result)
	var status, pending string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT status FROM executions WHERE id = ?", id).Scan(&status))
	require.Equal(t, "running", status)
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT result FROM send_attempts WHERE execution_id = ?`, id).Scan(&pending))
	require.Equal(t, "pending", pending)
	require.Equal(t, 2, storedExecutions(t, db))
	require.Greater(t, storedDue(t, db, taskID).Int64, now.UnixMilli())
	require.Len(t, m.entries, 1)
	require.NoError(t, m.LastError())
	again, err := m.Resume(ctx)
	require.NoError(t, err)
	require.Equal(t, RecoverySummary{}, again)
}
