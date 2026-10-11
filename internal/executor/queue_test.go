package executor

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dongwlin/dan-tick/internal/danmaku"
	"github.com/dongwlin/dan-tick/internal/diagnostic"
	"github.com/dongwlin/dan-tick/internal/schedule"
	"github.com/dongwlin/dan-tick/internal/task"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// blockingGate models the shared account admission boundary; the actual FIFO
// and rate limiting are verified separately in danmaku/sender_queue_test.go.
type blockingGate struct {
	slot    chan struct{}
	started chan string
	release chan struct{}
}

func newBlockingGate() *blockingGate {
	g := &blockingGate{
		slot:    make(chan struct{}, 1),
		started: make(chan string, 3),
		release: make(chan struct{}),
	}
	g.slot <- struct{}{}
	return g
}

func (g *blockingGate) SendWithAdmission(ctx context.Context, roomID, message string, admitted func(context.Context) error) (danmaku.Result, error) {
	select {
	case <-ctx.Done():
		return danmaku.Result{Outcome: danmaku.NotSent}, ctx.Err()
	case <-g.slot:
	}
	defer func() { g.slot <- struct{}{} }()
	if err := ctx.Err(); err != nil {
		return danmaku.Result{Outcome: danmaku.NotSent}, err
	}
	if err := admitted(ctx); err != nil {
		return danmaku.Result{Outcome: danmaku.NotSent}, err
	}
	g.started <- message
	if message == testMessage {
		select {
		case <-ctx.Done():
			return danmaku.Result{Outcome: danmaku.Unknown}, ctx.Err()
		case <-g.release:
		}
	}
	return danmaku.Result{Outcome: danmaku.Accepted}, nil
}

func createSecondTask(t *testing.T, db *bun.DB, roomID int64) task.Task {
	t.Helper()
	created, err := task.NewService(db).Create(context.Background(), task.CreateInput{
		RoomID: roomID, Name: "second task", Kind: task.Single,
		Messages: []string{"second task message"},
		Schedule: schedule.Rule{Kind: schedule.Daily, LocalTime: "22:00:00"},
		Enabled:  true,
	})
	require.NoError(t, err)
	return created
}

func waitForExecutionStatus(t *testing.T, db *bun.DB, taskID int64, expected string) int64 {
	t.Helper()
	end := time.After(3 * time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		var id int64
		var status string
		err := db.QueryRowContext(context.Background(), `
			SELECT id, status FROM executions WHERE task_id = ?`, taskID).Scan(&id, &status)
		if err == nil && status == expected {
			return id
		}
		if err != nil && err != sql.ErrNoRows {
			require.NoError(t, err)
		}
		select {
		case <-end:
			t.Fatalf("task %d never reached %s (last status %q)", taskID, expected, status)
		case <-ticker.C:
		}
	}
}

type finishedRun struct {
	result ManualResult
	err    error
}

func TestQueuedExecutionStartsOnlyAfterSharedAdmission(t *testing.T) {
	db := testDB(t)
	first := addSingleTask(t, db)
	second := createSecondTask(t, db, first.RoomID)
	gate := newBlockingGate()
	logger := diagnostic.New(t.TempDir())
	defer logger.Close()
	s := &Service{db: db, sender: gate, now: func() time.Time { return fixedTime }, log: logger}
	doneFirst := make(chan finishedRun, 1)
	doneSecond := make(chan finishedRun, 1)
	go func() {
		r, err := s.RunManual(context.Background(), first.ID)
		doneFirst <- finishedRun{r, err}
	}()
	require.Equal(t, testMessage, <-gate.started)
	firstID := waitForExecutionStatus(t, db, first.ID, "running")

	go func() {
		r, err := s.RunManual(context.Background(), second.ID)
		doneSecond <- finishedRun{r, err}
	}()
	secondID := waitForExecutionStatus(t, db, second.ID, "queued")
	var queuedAt, startedAt sql.NullInt64
	var pending string
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT queued_at, started_at FROM executions WHERE id = ?`, secondID).
		Scan(&queuedAt, &startedAt))
	require.True(t, queuedAt.Valid)
	require.False(t, startedAt.Valid, "queued is not running yet")
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT result FROM send_attempts WHERE execution_id = ?`, secondID).Scan(&pending))
	require.Equal(t, "pending", pending)

	_, err := s.RunManual(context.Background(), first.ID)
	require.ErrorIs(t, err, task.ErrTaskBusy)
	otherInstance := &Service{db: db, sender: gate, now: func() time.Time { return fixedTime }}
	_, err = otherInstance.RunManual(context.Background(), second.ID)
	require.ErrorIs(t, err, task.ErrTaskBusy, "database uniqueness must survive different Service instances")

	close(gate.release)
	firstRun := <-doneFirst
	require.NoError(t, firstRun.err)
	require.Equal(t, "completed", firstRun.result.Status)
	require.Equal(t, firstID, firstRun.result.ExecutionID)
	require.Equal(t, "second task message", <-gate.started)
	secondRun := <-doneSecond
	require.NoError(t, secondRun.err)
	require.Equal(t, "completed", secondRun.result.Status)
	require.Equal(t, secondID, secondRun.result.ExecutionID)
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT queued_at, started_at FROM executions WHERE id = ?`, secondID).
		Scan(&queuedAt, &startedAt))
	require.True(t, queuedAt.Valid)
	require.True(t, startedAt.Valid)
	data, err := os.ReadFile(filepath.Join(logger.Status().Directory, "dan-tick.jsonl"))
	require.NoError(t, err)
	require.Contains(t, string(data), `"event":"execution.queued"`)
	require.Contains(t, string(data), `"event":"execution.started"`)
	require.Contains(t, string(data), `"event":"execution.conflict"`)
	require.Contains(t, string(data), `"error_category":"task_busy"`)
	require.NotContains(t, string(data), testMessage)
	require.NotContains(t, string(data), "second task message")
	require.NotContains(t, string(data), testSession)
}

func TestStoppingQueuedRunDoesNotSendAndAllowsFutureRun(t *testing.T) {
	db := testDB(t)
	first := addSingleTask(t, db)
	second := createSecondTask(t, db, first.RoomID)
	gate := newBlockingGate()
	s := &Service{db: db, sender: gate, now: func() time.Time { return fixedTime }}
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(gate.release) }) })
	firstDone := make(chan finishedRun, 1)
	secondDone := make(chan finishedRun, 1)
	go func() {
		r, err := s.RunManual(context.Background(), first.ID)
		firstDone <- finishedRun{r, err}
	}()
	require.Equal(t, testMessage, <-gate.started)
	go func() {
		r, err := s.RunManual(context.Background(), second.ID)
		secondDone <- finishedRun{r, err}
	}()
	id := waitForExecutionStatus(t, db, second.ID, "queued")
	require.NoError(t, s.StopExecution(id))
	stopped := <-secondDone
	require.NoError(t, stopped.err)
	require.Equal(t, "stopped", stopped.result.Status)
	require.Equal(t, "user_stopped", stopped.result.ErrorCode)
	require.Zero(t, stopped.result.SentCount)
	require.Equal(t, 1, stopped.result.FailedCount)
	var startedAt sql.NullInt64
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT started_at FROM executions WHERE id = ?`, id).Scan(&startedAt))
	require.False(t, startedAt.Valid)
	_, outcomes := storedAttempts(t, db, id)
	require.Equal(t, []string{"failed"}, outcomes)
	once.Do(func() { close(gate.release) })
	require.NoError(t, (<-firstDone).err)
	select {
	case unexpected := <-gate.started:
		require.NotEqual(t, "second task message", unexpected)
	default:
	}
	repeated, err := s.RunManual(context.Background(), second.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", repeated.Status)
	require.NotEqual(t, id, repeated.ExecutionID)
}

func TestAdmissionWriteFailurePreventsSend(t *testing.T) {
	db := testDB(t)
	created := addSingleTask(t, db)
	_, err := db.ExecContext(context.Background(), `
		CREATE TRIGGER reject_admission BEFORE UPDATE ON executions
		WHEN NEW.status = 'running'
		BEGIN SELECT RAISE(ABORT, 'provider-private diagnostic'); END`)
	require.NoError(t, err)
	calls := 0
	s := &Service{
		db:  db,
		now: func() time.Time { return fixedTime },
		sender: &fakeGate{send: func(context.Context, string, string) (danmaku.Result, error) {
			calls++
			return danmaku.Result{Outcome: danmaku.Accepted}, nil
		}},
	}
	got, err := s.RunManual(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, "failed", got.Status)
	require.Equal(t, "persistence", got.ErrorCode)
	require.Zero(t, got.SentCount)
	require.Equal(t, 1, got.FailedCount)
	require.Zero(t, calls, "a failed queued->running transition cannot send")
	_, results := storedAttempts(t, db, got.ExecutionID)
	require.Equal(t, []string{"failed"}, results)
	var status string
	var startedAt sql.NullInt64
	require.NoError(t, db.QueryRowContext(context.Background(),
		"SELECT status, started_at FROM executions WHERE id = ?", got.ExecutionID).
		Scan(&status, &startedAt))
	require.Equal(t, "failed", status)
	require.False(t, startedAt.Valid)
}
