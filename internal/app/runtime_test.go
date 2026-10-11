package app

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dongwlin/dan-tick/internal/credential"
	"github.com/dongwlin/dan-tick/internal/danmaku"
	"github.com/dongwlin/dan-tick/internal/executor"
	"github.com/dongwlin/dan-tick/internal/schedule"
	"github.com/dongwlin/dan-tick/internal/scheduler"
	"github.com/dongwlin/dan-tick/internal/storage"
	"github.com/dongwlin/dan-tick/internal/task"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

type fakeCredentials struct{}

func (fakeCredentials) Load() (credential.Credentials, error) {
	return credential.Credentials{SESSDATA: "fictional_session", BiliJCT: "fictional_csrf"}, nil
}

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func runtimeDB(t *testing.T) *bun.DB {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "dan-tick.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

func saveTask(t *testing.T, db *bun.DB, due time.Time) task.Task {
	t.Helper()
	var roomID int64
	require.NoError(t, db.QueryRowContext(context.Background(), `
		INSERT INTO rooms (platform_room_id, created_at, updated_at)
		VALUES ('12345', 1, 1) RETURNING id`).Scan(&roomID))
	created, err := task.NewService(db).Create(context.Background(), task.CreateInput{
		RoomID: roomID, Name: "private task name", Kind: task.Single,
		Messages: []string{"private message"}, Enabled: true,
		Schedule: schedule.Rule{Kind: schedule.Once, Timezone: "UTC",
			LocalDate: due.Format(time.DateOnly), LocalTime: due.Format(time.TimeOnly)},
	})
	require.NoError(t, err)
	return created
}

func TestRuntimeStartupRecoversBeforeCronDispatchAndShutdownDrains(t *testing.T) {
	ctx := context.Background()
	db := runtimeDB(t)
	due := time.Now().UTC().Add(4 * time.Second).Truncate(time.Second)
	saved := saveTask(t, db, due)
	var abandonedID int64
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO executions (task_id, trigger_kind, status, created_at,
			queued_at, task_name_snapshot, platform_room_id_snapshot)
		VALUES (?, 'manual', 'queued', 1, 1, 'snapshot', '12345') RETURNING id`, saved.ID).Scan(&abandonedID))
	_, err := db.ExecContext(ctx, `INSERT INTO send_attempts (execution_id, sequence, message_no,
		content_snapshot, attempted_at, result) VALUES (?, 1, 1, 'private message', 1, 'pending')`, abandonedID)
	require.NoError(t, err)

	requestStarted := make(chan struct{}, 1)
	var sent atomic.Int32
	client := danmaku.NewClient(fakeCredentials{}, &http.Client{Transport: transport(func(req *http.Request) (*http.Response, error) {
		sent.Add(1)
		requestStarted <- struct{}{}
		<-req.Context().Done()
		return nil, req.Context().Err()
	})})
	runtime := newRuntime(db, nil, client)
	defer runtime.Close()
	require.NoError(t, runtime.Start(ctx))

	var status, attempt string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT status FROM executions WHERE id = ?", abandonedID).Scan(&status))
	require.Equal(t, "interrupted", status)
	require.NoError(t, db.QueryRowContext(ctx, "SELECT result FROM send_attempts WHERE execution_id = ?", abandonedID).Scan(&attempt))
	require.Equal(t, "unknown", attempt)

	select {
	case <-requestStarted: // A fresh Cron registration reached the real shared executor.
	case <-time.After(7 * time.Second):
		t.Fatal("future scheduled work was not dispatched")
	}
	runtime.Close() // Must cancel in-flight HTTP and join persistence before returning.
	runtime.Close() // A second shutdown is harmless.

	var completedStatus string
	var finished sql.NullInt64
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT status, finished_at FROM executions WHERE task_id = ? AND trigger_kind = 'scheduled'`, saved.ID).
		Scan(&completedStatus, &finished))
	require.Equal(t, "stopped", completedStatus)
	require.True(t, finished.Valid)
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT result FROM send_attempts WHERE execution_id =
			(SELECT id FROM executions WHERE task_id = ? AND trigger_kind = 'scheduled')`, saved.ID).Scan(&attempt))
	require.NotEqual(t, "pending", attempt)
	require.Equal(t, int32(1), sent.Load())
	_, err = runtime.Executor.RunScheduled(ctx, abandonedID)
	require.ErrorIs(t, err, executor.ErrExecutorClosed)
}

func TestRuntimeShutdownStopsNewManualRuns(t *testing.T) {
	ctx := context.Background()
	db := runtimeDB(t)
	saved := saveTask(t, db, time.Now().UTC().Add(time.Hour).Truncate(time.Second))
	client := danmaku.NewClient(fakeCredentials{}, &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"code":0}`))}, nil
	})})
	runtime := newRuntime(db, nil, client)
	require.NoError(t, runtime.Start(ctx))
	runtime.Close()
	_, err := runtime.Executor.RunManual(ctx, saved.ID)
	require.ErrorIs(t, err, executor.ErrExecutorClosed)
	_, err = runtime.Scheduler.SetTaskEnabled(ctx, saved.ID, false)
	require.ErrorIs(t, err, scheduler.ErrSchedulerClosed)
}

func TestOperationsManualRunSurvivesRequestCancellationUntilExplicitStop(t *testing.T) {
	ctx := context.Background()
	db := runtimeDB(t)
	saved := saveTask(t, db, time.Now().UTC().Add(time.Hour).Truncate(time.Second))
	sent := make(chan struct{}, 1)
	client := danmaku.NewClient(fakeCredentials{}, &http.Client{Transport: transport(func(req *http.Request) (*http.Response, error) {
		sent <- struct{}{}
		<-req.Context().Done()
		return nil, req.Context().Err()
	})})
	runtime := newRuntime(db, nil, client)
	defer runtime.Close()
	var changeCount atomic.Int32
	runtime.BindEvents(func(name string) {
		if name == "dan-tick:execution-changed" {
			changeCount.Add(1)
		}
	})
	require.NoError(t, runtime.Start(ctx))
	// A Wails request context can end when the user navigates. The worker is
	// independent and must remain alive until the user explicitly stops it.
	requestCtx, cancel := context.WithCancel(ctx)
	cancel()
	started, err := runtime.Operations.StartManual(requestCtx, saved.ID)
	require.NoError(t, err)
	require.Positive(t, started.ExecutionID)
	select {
	case <-sent:
	case <-time.After(5 * time.Second):
		t.Fatal("manual run did not reach shared sender")
	}
	runs, err := runtime.Operations.LatestExecutions(ctx)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, "running", runs[0].Status)
	require.Equal(t, started.ExecutionID, runs[0].ID)
	require.NoError(t, runtime.Operations.StopExecution(ctx, started.ExecutionID))
	require.Eventually(t, func() bool {
		runs, err := runtime.Operations.LatestExecutions(ctx)
		return err == nil && len(runs) == 1 && runs[0].Status == "stopped"
	}, 5*time.Second, 20*time.Millisecond)
	require.GreaterOrEqual(t, changeCount.Load(), int32(3), "queue, run and terminal changes must be notified")
	_, err = runtime.Operations.StartManual(ctx, 0)
	require.Error(t, err)
}

func TestRuntimeShutdownCancelsManualRunAndPersistsTerminalStatus(t *testing.T) {
	ctx := context.Background()
	db := runtimeDB(t)
	saved := saveTask(t, db, time.Now().UTC().Add(time.Hour).Truncate(time.Second))
	started := make(chan struct{}, 1)
	client := danmaku.NewClient(fakeCredentials{}, &http.Client{Transport: transport(func(req *http.Request) (*http.Response, error) {
		started <- struct{}{}
		<-req.Context().Done()
		return nil, req.Context().Err()
	})})
	runtime := newRuntime(db, nil, client)
	require.NoError(t, runtime.Start(ctx))
	finished := make(chan executor.ManualResult, 1)
	go func() {
		result, _ := runtime.Executor.RunManual(ctx, saved.ID)
		finished <- result
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("manual request was not admitted")
	}
	runtime.Close()
	select {
	case result := <-finished:
		require.Equal(t, "stopped", result.Status)
	case <-time.After(2 * time.Second):
		t.Fatal("manual execution was not joined")
	}
	var status, attempt string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT status FROM executions WHERE task_id = ?", saved.ID).Scan(&status))
	require.Equal(t, "stopped", status)
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT result FROM send_attempts WHERE execution_id =
			(SELECT id FROM executions WHERE task_id = ?)`, saved.ID).Scan(&attempt))
	require.NotEqual(t, "pending", attempt)
}

func TestRuntimeStartupFailureDoesNotBeginCron(t *testing.T) {
	ctx := context.Background()
	db := runtimeDB(t)
	saved := saveTask(t, db, time.Now().UTC().Add(time.Hour).Truncate(time.Second))
	_, err := db.ExecContext(ctx, "UPDATE tasks SET next_due_at = next_due_at + 1000 WHERE id = ?", saved.ID)
	require.NoError(t, err)
	runtime := newRuntime(db, nil, danmaku.NewClient(fakeCredentials{}, nil))
	require.Error(t, runtime.Start(ctx))
	runtime.Close()
	require.ErrorIs(t, runtime.Scheduler.Rebuild(ctx), scheduler.ErrSchedulerClosed)
}
