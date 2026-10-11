package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dongwlin/dan-tick/internal/diagnostic"
	"github.com/dongwlin/dan-tick/internal/history"
	"github.com/dongwlin/dan-tick/internal/schedule"
	"github.com/dongwlin/dan-tick/internal/task"
	"github.com/stretchr/testify/require"
)

func TestFailedHandoffFinalizesHistoryAndDoesNotBlockNextOccurrence(t *testing.T) {
	ctx := context.Background()
	db, _ := claimDB(t)
	taskID := claimTask(t, db, "daily", "UTC", "", "09:00:00", nil, scheduledTime.UnixMilli())
	dir := filepath.Join(t.TempDir(), "logs")
	log := diagnostic.New(dir)
	defer log.Close()
	var delivered []ClaimResult
	m := NewManager(db, log, func(_ context.Context, result ClaimResult) error {
		delivered = append(delivered, result)
		if len(delivered) == 1 {
			return errors.New("private handoff error")
		}
		return nil
	}, nil)
	defer func() { <-m.Stop().Done() }()
	at := scheduledTime.Add(-time.Second)
	m.now = func() time.Time { return at }
	m.claim.now, m.recovery.now = m.now, m.now
	require.NoError(t, m.Rebuild(ctx))
	reg := m.entries[taskID]
	at = scheduledTime
	m.fire(taskID, reg.due, reg.generation)
	require.Len(t, delivered, 1)
	require.ErrorContains(t, m.LastError(), "private handoff error")
	detail, err := history.NewService(db).Get(ctx, delivered[0].ExecutionID)
	require.NoError(t, err)
	require.Equal(t, "failed", detail.Execution.Status)
	require.Equal(t, "handoff_failed", *detail.Execution.ErrorCode)
	require.Equal(t, at.UnixMilli(), *detail.Execution.FinishedAt)
	require.Empty(t, detail.Attempts)
	require.Zero(t, detail.Execution.SentCount)
	require.Zero(t, detail.Execution.FailedCount)
	require.Equal(t, scheduledTime.Add(24*time.Hour).UnixMilli(), storedDue(t, db, taskID).Int64)

	// A rejected dispatch must release the active-task constraint immediately.
	stored, err := task.NewService(db).Get(ctx, taskID)
	require.NoError(t, err)
	_, err = task.NewService(db).Update(ctx, taskID, task.CreateInput{
		RoomID: stored.RoomID, Name: "edited after rejected handoff", Kind: task.Single,
		Messages: []string{"private message"}, Enabled: true,
		Schedule: schedule.Rule{Kind: schedule.Daily, Timezone: "UTC", LocalTime: "09:00:00"},
	})
	require.NoError(t, err)
	reg = m.entries[taskID]
	at = scheduledTime.Add(24 * time.Hour)
	m.fire(taskID, reg.due, reg.generation)
	require.Len(t, delivered, 2)
	require.Equal(t, "waiting", delivered[1].Status)
	require.Empty(t, delivered[1].ErrorCode)

	log.Close()
	data, err := os.ReadFile(filepath.Join(dir, "dan-tick.jsonl"))
	require.NoError(t, err)
	require.Contains(t, string(data), `"error_category":"handoff_failed"`)
	require.NotContains(t, string(data), "private handoff error")
	require.NotContains(t, string(data), "private message")
}

func TestFailedHandoffDoesNotOverwriteAcceptedOrAttemptedWork(t *testing.T) {
	for _, status := range []string{"queued", "running", "completed", "waiting_with_attempt"} {
		t.Run(status, func(t *testing.T) {
			ctx := context.Background()
			db, _ := claimDB(t)
			taskID := claimTask(t, db, "once", "UTC", "2026-10-11", "09:00:00", nil, scheduledTime.UnixMilli())
			var executionID int64
			storedStatus := strings.TrimSuffix(status, "_with_attempt")
			m := NewManager(db, nil, func(_ context.Context, result ClaimResult) error {
				executionID = result.ExecutionID
				if status == "waiting_with_attempt" {
					_, err := db.ExecContext(ctx, `INSERT INTO send_attempts
						(execution_id, sequence, message_no, content_snapshot, attempted_at, result)
						VALUES (?, 1, 1, 'private message', ?, 'pending')`, executionID, scheduledTime.UnixMilli())
					require.NoError(t, err)
				}
				_, err := db.ExecContext(ctx, "UPDATE executions SET status = ? WHERE id = ?", storedStatus, executionID)
				require.NoError(t, err)
				return errors.New("failure after executor accepted")
			}, nil)
			defer func() { <-m.Stop().Done() }()
			at := scheduledTime.Add(-time.Second)
			m.now = func() time.Time { return at }
			m.claim.now, m.recovery.now = m.now, m.now
			require.NoError(t, m.Rebuild(ctx))
			reg := m.entries[taskID]
			at = scheduledTime
			m.fire(taskID, reg.due, reg.generation)
			var actual string
			var code sql.NullString
			var finished sql.NullInt64
			require.NoError(t, db.QueryRowContext(ctx,
				"SELECT status, error_code, finished_at FROM executions WHERE id = ?", executionID).Scan(&actual, &code, &finished))
			require.Equal(t, storedStatus, actual)
			require.False(t, code.Valid)
			require.False(t, finished.Valid)
		})
	}
}

func TestFailedHandoffPersistenceErrorIsReportedAndRemainsRecoverable(t *testing.T) {
	ctx := context.Background()
	db, _ := claimDB(t)
	taskID := claimTask(t, db, "once", "UTC", "2026-10-11", "09:00:00", nil, scheduledTime.UnixMilli())
	dir := filepath.Join(t.TempDir(), "logs")
	log := diagnostic.New(dir)
	defer log.Close()
	_, err := db.ExecContext(ctx, `CREATE TRIGGER reject_handoff_finish BEFORE UPDATE ON executions
		WHEN NEW.status = 'failed' BEGIN SELECT RAISE(ABORT, 'injected handoff persistence failure'); END`)
	require.NoError(t, err)
	var executionID int64
	m := NewManager(db, log, func(_ context.Context, result ClaimResult) error {
		executionID = result.ExecutionID
		return errors.New("executor unavailable")
	}, nil)
	defer func() { <-m.Stop().Done() }()
	at := scheduledTime.Add(-time.Second)
	m.now = func() time.Time { return at }
	m.claim.now, m.recovery.now = m.now, m.now
	require.NoError(t, m.Rebuild(ctx))
	reg := m.entries[taskID]
	at = scheduledTime
	m.fire(taskID, reg.due, reg.generation)
	require.ErrorContains(t, m.LastError(), "executor unavailable")
	require.ErrorContains(t, m.LastError(), "finalize unaccepted")
	var status string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT status FROM executions WHERE id = ?", executionID).Scan(&status))
	require.Equal(t, "waiting", status)
	_, err = db.ExecContext(ctx, "DROP TRIGGER reject_handoff_finish")
	require.NoError(t, err)
	recovery, err := m.recovery.RecoverStartup(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, recovery.Interrupted)
	log.Close()
	data, err := os.ReadFile(filepath.Join(dir, "dan-tick.jsonl"))
	require.NoError(t, err)
	require.Contains(t, string(data), `"event":"execution.persist_failed"`)
	require.NotContains(t, string(data), `"event":"execution.failed"`)
}

func TestCancelledHandoffFinalizesWithoutReusingCancelledContext(t *testing.T) {
	ctx := context.Background()
	db, _ := claimDB(t)
	taskID := claimTask(t, db, "once", "UTC", "2026-10-11", "09:00:00", nil, scheduledTime.UnixMilli())
	var executionID int64
	var m *Manager
	m = NewManager(db, nil, func(callbackCtx context.Context, result ClaimResult) error {
		executionID = result.ExecutionID
		m.cancel()
		return callbackCtx.Err()
	}, nil)
	defer func() { <-m.Stop().Done() }()
	at := scheduledTime.Add(-time.Second)
	m.now = func() time.Time { return at }
	m.claim.now, m.recovery.now = m.now, m.now
	require.NoError(t, m.Rebuild(ctx))
	reg := m.entries[taskID]
	at = scheduledTime
	m.fire(taskID, reg.due, reg.generation)
	detail, err := history.NewService(db).Get(ctx, executionID)
	require.NoError(t, err)
	require.Equal(t, "stopped", detail.Execution.Status)
	require.Equal(t, "cancelled", *detail.Execution.ErrorCode)
	require.NotNil(t, detail.Execution.FinishedAt)
	require.Empty(t, detail.Attempts)
}
