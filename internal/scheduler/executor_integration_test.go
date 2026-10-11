package scheduler

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dongwlin/dan-tick/internal/credential"
	"github.com/dongwlin/dan-tick/internal/danmaku"
	"github.com/dongwlin/dan-tick/internal/executor"
	"github.com/dongwlin/dan-tick/internal/history"
	"github.com/dongwlin/dan-tick/internal/schedule"
	"github.com/dongwlin/dan-tick/internal/task"
	"github.com/stretchr/testify/require"
)

type scheduledCredentials struct{}

func (scheduledCredentials) Load() (credential.Credentials, error) {
	return credential.Credentials{SESSDATA: "fake_session", BiliJCT: "fake_csrf"}, nil
}

type scheduledTransport func(*http.Request) (*http.Response, error)

func (f scheduledTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestCronDispatchesExistingOccurrenceToSharedExecutor(t *testing.T) {
	ctx := context.Background()
	db, _ := claimDB(t)
	roomID := savedRoom(t, db)
	due := time.Now().UTC().Add(3 * time.Second).Truncate(time.Second)
	requestStarted := make(chan error, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	var calls atomic.Int32
	client := danmaku.NewClient(scheduledCredentials{}, &http.Client{Transport: scheduledTransport(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		// The send boundary observes the committed scheduled execution and
		// pending attempt; no real HTTP request leaves this process.
		var status, trigger, attempt string
		err := db.QueryRowContext(req.Context(), `
			SELECT e.status, e.trigger_kind, a.result FROM executions e
			JOIN send_attempts a ON a.execution_id = e.id`).Scan(&status, &trigger, &attempt)
		if err == nil && (status != "running" || trigger != "scheduled" || attempt != "pending") {
			err = executor.ErrExecutionNotWaiting
		}
		requestStarted <- err
		select {
		case <-release:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"code":0}`))}, nil
	})})
	runner := executor.NewService(db, danmaku.NewSender(client), nil)
	type outcome struct {
		result executor.ManualResult
		err    error
	}
	completed := make(chan outcome, 1)
	reported := make(chan error, 1)
	m := NewManager(db, nil, func(callbackCtx context.Context, claim ClaimResult) error {
		return runner.StartScheduled(callbackCtx, claim.ExecutionID, func(result executor.ManualResult, err error) {
			completed <- outcome{result, err}
		})
	}, func(err error) { reported <- err })
	defer func() { <-m.Stop().Done() }()
	input := taskInput(roomID, due)
	input.Schedule = schedule.Rule{Kind: schedule.Daily, Timezone: "UTC", LocalTime: due.Format(time.TimeOnly)}
	created, err := m.CreateTask(ctx, input)
	require.NoError(t, err)
	require.NoError(t, m.Start(ctx))
	select {
	case err := <-requestStarted:
		require.NoError(t, err)
	case err := <-reported:
		t.Fatalf("dispatch failed: %v", err)
	case <-time.After(6 * time.Second):
		t.Fatal("scheduled request never started")
	}
	var executionID int64
	require.NoError(t, db.QueryRowContext(ctx, "SELECT id FROM executions WHERE task_id = ?", created.ID).Scan(&executionID))
	_, err = runner.RunManual(ctx, created.ID)
	require.ErrorIs(t, err, task.ErrTaskBusy)
	_, err = runner.RunScheduled(ctx, executionID)
	require.ErrorIs(t, err, executor.ErrExecutionNotWaiting)
	// Pausing future scheduling does not cancel work already accepted.
	require.NoError(t, m.SetSchedulingEnabled(ctx, false))
	releaseOnce.Do(func() { close(release) })
	select {
	case done := <-completed:
		require.NoError(t, done.err)
		require.Equal(t, executionID, done.result.ExecutionID)
		require.Equal(t, "completed", done.result.Status)
		require.Equal(t, 1, done.result.SentCount)
	case <-time.After(3 * time.Second):
		t.Fatal("scheduled execution did not finish")
	}
	detail, err := history.NewService(db).Get(ctx, executionID)
	require.NoError(t, err)
	require.Equal(t, "scheduled", detail.Execution.TriggerKind)
	require.Equal(t, due.UnixMilli(), *detail.Execution.ScheduledAt)
	require.Equal(t, input.Name, detail.Execution.TaskNameSnapshot)
	require.Len(t, detail.Attempts, 1)
	require.Equal(t, "accepted", detail.Attempts[0].Result)
	require.Equal(t, input.Messages[0], detail.Attempts[0].ContentSnapshot)
	require.NotNil(t, detail.Execution.StartedAt)
	require.NotNil(t, detail.Execution.FinishedAt)
	require.Equal(t, due.Add(24*time.Hour).UnixMilli(), storedDue(t, db, created.ID).Int64)
	require.Equal(t, 1, storedExecutions(t, db))
	_, err = runner.RunScheduled(ctx, executionID)
	require.ErrorIs(t, err, executor.ErrExecutionNotWaiting)
	require.Equal(t, int32(1), calls.Load())
	require.Empty(t, reported)
}

func TestExecutorPreparationFailureReleasesScheduledTask(t *testing.T) {
	ctx := context.Background()
	db, _ := claimDB(t)
	taskID := claimTask(t, db, "daily", "UTC", "", "09:00:00", nil, scheduledTime.UnixMilli())
	_, err := db.ExecContext(ctx, `CREATE TRIGGER reject_first_pending BEFORE INSERT ON send_attempts
		BEGIN SELECT RAISE(ABORT, 'injected preparation failure'); END`)
	require.NoError(t, err)
	var calls atomic.Int32
	client := danmaku.NewClient(scheduledCredentials{}, &http.Client{Transport: scheduledTransport(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, context.Canceled
	})})
	runner := executor.NewService(db, danmaku.NewSender(client), nil)
	m := NewManager(db, nil, func(callbackCtx context.Context, claim ClaimResult) error {
		return runner.StartScheduled(callbackCtx, claim.ExecutionID, nil)
	}, nil)
	defer func() { <-m.Stop().Done() }()
	at := scheduledTime.Add(-time.Second)
	m.now = func() time.Time { return at }
	m.claim.now, m.recovery.now = m.now, m.now
	require.NoError(t, m.Rebuild(ctx))
	reg := m.entries[taskID]
	at = scheduledTime
	m.fire(taskID, reg.due, reg.generation)
	require.ErrorContains(t, m.LastError(), "injected preparation failure")
	var status, code string
	var finished sql.NullInt64
	require.NoError(t, db.QueryRowContext(ctx,
		"SELECT status, error_code, finished_at FROM executions WHERE task_id = ?", taskID).Scan(&status, &code, &finished))
	require.Equal(t, "failed", status)
	require.Equal(t, "handoff_failed", code)
	require.True(t, finished.Valid)
	var active, attempts int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM executions WHERE status IN ('waiting', 'queued', 'running')").Scan(&active))
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM send_attempts").Scan(&attempts))
	require.Zero(t, active)
	require.Zero(t, attempts)
	require.Zero(t, calls.Load())
	require.Equal(t, scheduledTime.Add(24*time.Hour).UnixMilli(), storedDue(t, db, taskID).Int64)
}
