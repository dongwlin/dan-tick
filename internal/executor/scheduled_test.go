package executor

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dongwlin/dan-tick/internal/danmaku"
	"github.com/dongwlin/dan-tick/internal/storage"
	"github.com/dongwlin/dan-tick/internal/task"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func addWaitingExecution(t *testing.T, db *bun.DB, taskID int64) int64 {
	t.Helper()
	var id int64
	err := db.QueryRowContext(context.Background(), `
		INSERT INTO executions (task_id, trigger_kind, scheduled_at, status,
			created_at, task_name_snapshot, platform_room_id_snapshot)
		VALUES (?, 'scheduled', ? + (SELECT COUNT(*) FROM executions), 'waiting',
			?, 'Original claimed name', '12345') RETURNING id`,
		taskID, fixedTime.UnixMilli(), fixedTime.UnixMilli()).Scan(&id)
	require.NoError(t, err)
	return id
}

func requireUntouchedWaiting(t *testing.T, db *bun.DB, id int64) {
	t.Helper()
	var status string
	var queued, started, finished sql.NullInt64
	var attempts int
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT status, queued_at, started_at, finished_at,
			(SELECT COUNT(*) FROM send_attempts WHERE execution_id = executions.id)
		FROM executions WHERE id = ?`, id).Scan(&status, &queued, &started, &finished, &attempts))
	require.Equal(t, "waiting", status)
	require.False(t, queued.Valid)
	require.False(t, started.Valid)
	require.False(t, finished.Valid)
	require.Zero(t, attempts)
}

func TestRunScheduledUsesExistingOccurrenceSnapshotsAndCursor(t *testing.T) {
	db := testDB(t)
	created := addSingleTask(t, db)
	id := addWaitingExecution(t, db, created.ID)
	// A later scheduling pause or disable controls future claims, not this
	// already claimed occurrence. Its current valid messages still apply.
	_, err := task.NewService(db).SetEnabled(context.Background(), created.ID, false)
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(), `
		INSERT INTO app_settings (key, value) VALUES ('scheduling_enabled', 'false')`)
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(), `
		UPDATE task_messages SET content = 'current saved message' WHERE task_id = ?`, created.ID)
	require.NoError(t, err)
	calls := 0
	s := &Service{db: db, now: func() time.Time { return fixedTime }, sender: &fakeGate{
		send: func(ctx context.Context, room, message string) (danmaku.Result, error) {
			calls++
			require.Equal(t, "12345", room)
			require.Equal(t, "current saved message", message)
			var trigger, status, pending string
			require.NoError(t, db.QueryRowContext(ctx, `
				SELECT e.trigger_kind, e.status, a.result FROM executions e
				JOIN send_attempts a ON a.execution_id = e.id WHERE e.id = ?`, id).
				Scan(&trigger, &status, &pending))
			require.Equal(t, "scheduled", trigger)
			require.Equal(t, "running", status)
			require.Equal(t, "pending", pending)
			return danmaku.Result{Outcome: danmaku.Accepted}, nil
		},
	}}
	result, err := s.RunScheduled(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, id, result.ExecutionID)
	require.Equal(t, "completed", result.Status)
	require.Equal(t, 1, result.SentCount)
	require.Equal(t, 1, calls)
	var count int
	var name, room, trigger string
	var scheduled int64
	var due sql.NullInt64
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT task_name_snapshot, platform_room_id_snapshot, trigger_kind,
			scheduled_at, (SELECT next_due_at FROM tasks WHERE id = executions.task_id),
			(SELECT COUNT(*) FROM executions) FROM executions WHERE id = ?`, id).
		Scan(&name, &room, &trigger, &scheduled, &due, &count))
	require.Equal(t, "Original claimed name", name)
	require.Equal(t, "12345", room)
	require.Equal(t, "scheduled", trigger)
	require.Equal(t, fixedTime.UnixMilli(), scheduled)
	require.False(t, due.Valid)
	require.Equal(t, 1, count)
	_, outcomes := storedAttempts(t, db, id)
	require.Equal(t, []string{"accepted"}, outcomes)
	_, err = s.RunScheduled(context.Background(), id)
	require.ErrorIs(t, err, ErrExecutionNotWaiting)
	require.Equal(t, 1, calls, "a duplicate handoff cannot resend a completed occurrence")
}

func TestRunScheduledLoopUsesSharedExecutionChain(t *testing.T) {
	db := testDB(t)
	created := addLoopTask(t, db, []string{"one", "two"}, 20, intPtr(3), nil)
	id := addWaitingExecution(t, db, created.ID)
	clock := &loopClock{at: fixedTime}
	var messages []string
	s := loopService(db, clock, func(ctx context.Context, _, message string) (danmaku.Result, error) {
		messages = append(messages, message)
		return danmaku.Result{Outcome: danmaku.Accepted}, nil
	})
	result, err := s.RunScheduled(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, "completed", result.Status)
	require.Equal(t, 3, result.SentCount)
	require.Equal(t, []string{"one", "two", "one"}, messages)
	stored, outcomes := storedAttempts(t, db, id)
	require.Equal(t, messages, stored)
	require.Equal(t, []string{"accepted", "accepted", "accepted"}, outcomes)
	var due int64
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT next_due_at FROM tasks WHERE id = ?`, created.ID).Scan(&due))
	require.Equal(t, *created.NextDueAt, due)
}

func TestStoppingScheduledLoopPreventsLaterMessages(t *testing.T) {
	db := testDB(t)
	created := addLoopTask(t, db, []string{"one", "two"}, 15, intPtr(3), nil)
	id := addWaitingExecution(t, db, created.ID)
	clock := &loopClock{at: fixedTime}
	waiting := make(chan struct{})
	calls := 0
	s := loopService(db, clock, func(ctx context.Context, _, _ string) (danmaku.Result, error) {
		calls++
		return danmaku.Result{Outcome: danmaku.Accepted}, nil
	})
	s.wait = func(ctx context.Context, _ time.Duration) error {
		close(waiting)
		<-ctx.Done()
		return ctx.Err()
	}
	done := make(chan finishedRun, 1)
	go func() {
		r, err := s.RunScheduled(context.Background(), id)
		done <- finishedRun{r, err}
	}()
	<-waiting
	require.NoError(t, s.StopExecution(id))
	result := <-done
	require.NoError(t, result.err)
	require.Equal(t, "stopped", result.result.Status)
	require.Equal(t, "user_stopped", result.result.ErrorCode)
	require.Equal(t, 1, calls)
	_, outcomes := storedAttempts(t, db, id)
	require.Equal(t, []string{"accepted"}, outcomes)
	require.ErrorIs(t, s.StopExecution(id), ErrNotRunning)
}

func TestConcurrentScheduledTakeoverAcrossServicesSendsOnce(t *testing.T) {
	db := testDB(t)
	var databasePath string
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT file FROM pragma_database_list WHERE name = 'main'`).Scan(&databasePath))
	otherDB, err := storage.Open(context.Background(), databasePath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, otherDB.Close()) })
	created := addSingleTask(t, db)
	id := addWaitingExecution(t, db, created.ID)
	requestStarted := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	var calls atomic.Int32
	gate := &fakeGate{send: func(ctx context.Context, _, _ string) (danmaku.Result, error) {
		if calls.Add(1) == 1 {
			close(requestStarted)
		}
		<-release
		return danmaku.Result{Outcome: danmaku.Accepted}, nil
	}}
	first := &Service{db: db, sender: gate, now: func() time.Time { return fixedTime }}
	second := &Service{db: otherDB, sender: gate, now: func() time.Time { return fixedTime }}
	begin := make(chan struct{})
	done := make(chan finishedRun, 2)
	for _, svc := range []*Service{first, second} {
		go func(svc *Service) {
			<-begin
			r, err := svc.RunScheduled(context.Background(), id)
			done <- finishedRun{r, err}
		}(svc)
	}
	close(begin)
	<-requestStarted
	select {
	case loser := <-done:
		require.ErrorIs(t, loser.err, ErrExecutionNotWaiting)
		require.Equal(t, id, loser.result.ExecutionID)
	case <-time.After(3 * time.Second):
		t.Fatal("duplicate scheduled takeover did not reject the active execution")
	}
	_, err = first.RunManual(context.Background(), created.ID)
	require.ErrorIs(t, err, task.ErrTaskBusy)
	_, err = first.RunScheduled(context.Background(), id)
	require.ErrorIs(t, err, ErrExecutionNotWaiting)
	once.Do(func() { close(release) })
	winner := <-done
	require.NoError(t, winner.err)
	require.Equal(t, "completed", winner.result.Status)
	require.Equal(t, int32(1), calls.Load())
	_, outcomes := storedAttempts(t, db, id)
	require.Equal(t, []string{"accepted"}, outcomes)
}

func TestScheduledTakeoverFailureRollsBackForCoordinator(t *testing.T) {
	for _, failure := range []string{"invalid messages", "deleted room", "pending insert"} {
		t.Run(failure, func(t *testing.T) {
			db := testDB(t)
			created := addSingleTask(t, db)
			id := addWaitingExecution(t, db, created.ID)
			var err error
			switch failure {
			case "invalid messages":
				_, err = db.ExecContext(context.Background(), `DELETE FROM task_messages WHERE task_id = ?`, created.ID)
			case "deleted room":
				_, err = db.ExecContext(context.Background(), `UPDATE rooms SET deleted_at = ? WHERE id = ?`, fixedTime.UnixMilli(), created.RoomID)
			case "pending insert":
				_, err = db.ExecContext(context.Background(), `
					CREATE TRIGGER reject_scheduled_pending BEFORE INSERT ON send_attempts
					BEGIN SELECT RAISE(ABORT, 'injected write failure'); END`)
			}
			require.NoError(t, err)
			calls := 0
			s := &Service{db: db, now: func() time.Time { return fixedTime }, sender: &fakeGate{
				send: func(context.Context, string, string) (danmaku.Result, error) {
					calls++
					return danmaku.Result{Outcome: danmaku.Accepted}, nil
				},
			}}
			result, err := s.RunScheduled(context.Background(), id)
			require.Error(t, err)
			if failure == "invalid messages" {
				require.ErrorIs(t, err, task.ErrInvalidTask)
			}
			if failure == "deleted room" {
				require.ErrorIs(t, err, task.ErrNotFound)
			}
			require.Equal(t, id, result.ExecutionID)
			require.Zero(t, calls)
			requireUntouchedWaiting(t, db, id)
			require.ErrorIs(t, s.StopExecution(id), ErrNotRunning)
			completed := make(chan finishedRun, 1)
			err = s.StartScheduled(context.Background(), id, func(r ManualResult, err error) {
				completed <- finishedRun{r, err}
			})
			require.Error(t, err, "preparation errors must reach the Cron handoff synchronously")
			requireUntouchedWaiting(t, db, id)
			select {
			case <-completed:
				t.Fatal("a failed takeover must not start an execution goroutine")
			default:
			}
			if failure == "pending insert" {
				_, err = db.ExecContext(context.Background(), `DROP TRIGGER reject_scheduled_pending`)
				require.NoError(t, err)
				retried, err := s.RunScheduled(context.Background(), id)
				require.NoError(t, err)
				require.Equal(t, "completed", retried.Status)
				require.Equal(t, 1, calls)
			}
		})
	}
}

func TestStartScheduledCommitsBeforeReturningAndCanStopQueuedWork(t *testing.T) {
	db := testDB(t)
	created := addSingleTask(t, db)
	id := addWaitingExecution(t, db, created.ID)
	s := &Service{db: db, sender: waitingDurationGate{}, now: func() time.Time { return fixedTime }}
	done := make(chan finishedRun, 1)
	require.NoError(t, s.StartScheduled(context.Background(), id, func(r ManualResult, err error) {
		done <- finishedRun{r, err}
	}))
	var status, pending string
	var queued, started sql.NullInt64
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT e.status, e.queued_at, e.started_at, a.result FROM executions e
		JOIN send_attempts a ON a.execution_id = e.id WHERE e.id = ?`, id).
		Scan(&status, &queued, &started, &pending))
	require.Equal(t, "queued", status)
	require.True(t, queued.Valid)
	require.False(t, started.Valid)
	require.Equal(t, "pending", pending)
	require.ErrorIs(t, s.StartScheduled(context.Background(), id, nil), ErrExecutionNotWaiting)
	require.NoError(t, s.StopExecution(id), "the cancellation handle must also exist before returning")
	result := <-done
	require.NoError(t, result.err)
	require.Equal(t, id, result.result.ExecutionID)
	require.Equal(t, "stopped", result.result.Status)
	require.Equal(t, "user_stopped", result.result.ErrorCode)
	require.ErrorIs(t, s.StopExecution(id), ErrNotRunning)
}

func TestStartScheduledReportsResultPersistenceError(t *testing.T) {
	db := testDB(t)
	created := addSingleTask(t, db)
	id := addWaitingExecution(t, db, created.ID)
	_, err := db.ExecContext(context.Background(), `
		CREATE TRIGGER reject_scheduled_result BEFORE UPDATE ON executions
		WHEN NEW.status = 'completed'
		BEGIN SELECT RAISE(ABORT, 'injected result failure'); END`)
	require.NoError(t, err)
	s := &Service{db: db, now: func() time.Time { return fixedTime }, sender: &fakeGate{
		send: func(context.Context, string, string) (danmaku.Result, error) {
			return danmaku.Result{Outcome: danmaku.Accepted}, nil
		},
	}}
	done := make(chan finishedRun, 1)
	require.NoError(t, s.StartScheduled(context.Background(), id, func(r ManualResult, err error) {
		done <- finishedRun{r, err}
	}))
	result := <-done
	require.Error(t, result.err)
	require.Equal(t, id, result.result.ExecutionID)
	var status, pending string
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT e.status, a.result FROM executions e
		JOIN send_attempts a ON a.execution_id = e.id WHERE e.id = ?`, id).Scan(&status, &pending))
	require.Equal(t, "running", status)
	require.Equal(t, "pending", pending)
	require.ErrorIs(t, s.StartScheduled(context.Background(), id, nil), ErrExecutionNotWaiting)
	require.ErrorIs(t, s.StopExecution(id), ErrNotRunning)
}

func TestStartScheduledAllowsNoCompletionCallback(t *testing.T) {
	db := testDB(t)
	created := addSingleTask(t, db)
	id := addWaitingExecution(t, db, created.ID)
	s := &Service{db: db, now: func() time.Time { return fixedTime }, sender: &fakeGate{
		send: func(context.Context, string, string) (danmaku.Result, error) {
			return danmaku.Result{Outcome: danmaku.Accepted}, nil
		},
	}}
	require.NoError(t, s.StartScheduled(context.Background(), id, nil))
	require.Equal(t, id, waitForExecutionStatus(t, db, created.ID, "completed"))
}

func TestStartScheduledParentCancellationWhileQueuedDoesNotSend(t *testing.T) {
	db := testDB(t)
	first := addSingleTask(t, db)
	second := createSecondTask(t, db, first.RoomID)
	_, err := db.ExecContext(context.Background(), `
		UPDATE tasks SET kind = 'loop', interval_seconds = 15, max_count = 3 WHERE id = ?`, second.ID)
	require.NoError(t, err)
	id := addWaitingExecution(t, db, second.ID)
	gate := newBlockingGate()
	s := &Service{db: db, sender: gate, now: func() time.Time { return fixedTime }}
	firstDone := make(chan struct{})
	var firstResult finishedRun
	var release sync.Once
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	t.Cleanup(func() {
		release.Do(func() { close(gate.release) })
		cancelFirst()
		<-firstDone
	})
	go func() {
		r, err := s.RunManual(firstCtx, first.ID)
		firstResult = finishedRun{r, err}
		close(firstDone)
	}()
	require.Equal(t, testMessage, <-gate.started)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan finishedRun, 2)
	var callbacks atomic.Int32
	require.NoError(t, s.StartScheduled(ctx, id, func(r ManualResult, err error) {
		callbacks.Add(1)
		done <- finishedRun{r, err}
	}))
	require.Equal(t, id, waitForExecutionStatus(t, db, second.ID, "queued"))
	cancel()
	var result finishedRun
	select {
	case result = <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("queued scheduled work did not finish after parent cancellation")
	}
	require.NoError(t, result.err)
	require.Equal(t, "stopped", result.result.Status)
	require.Equal(t, "cancelled", result.result.ErrorCode)
	require.Equal(t, danmaku.NotSent, result.result.Outcome)
	require.Equal(t, int32(1), callbacks.Load())
	require.ErrorIs(t, s.StopExecution(id), ErrNotRunning)
	var status, code string
	var started, finished sql.NullInt64
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT status, error_code, started_at, finished_at FROM executions WHERE id = ?`, id).
		Scan(&status, &code, &started, &finished))
	require.Equal(t, "stopped", status)
	require.Equal(t, "cancelled", code)
	require.False(t, started.Valid)
	require.True(t, finished.Valid)
	_, outcomes := storedAttempts(t, db, id)
	require.Equal(t, []string{"failed"}, outcomes, "the queued pending attempt must settle without HTTP")
	release.Do(func() { close(gate.release) })
	<-firstDone
	require.NoError(t, firstResult.err)
	select {
	case message := <-gate.started:
		t.Fatalf("canceled scheduled loop sent a message: %q", message)
	case <-done:
		t.Fatal("scheduled completion callback ran more than once")
	default:
	}
}

func TestStartScheduledParentCancellationDuringHTTPPreservesUnknown(t *testing.T) {
	db := testDB(t)
	created := addLoopTask(t, db, []string{"one", "two"}, 15, intPtr(3), nil)
	id := addWaitingExecution(t, db, created.ID)
	requestStarted := make(chan struct{})
	var calls atomic.Int32
	s := &Service{db: db, now: func() time.Time { return fixedTime }, sender: &fakeGate{
		send: func(ctx context.Context, _, _ string) (danmaku.Result, error) {
			if calls.Add(1) == 1 {
				close(requestStarted)
			}
			<-ctx.Done()
			return danmaku.Result{Outcome: danmaku.Unknown}, errors.Join(danmaku.ErrUncertain, ctx.Err())
		},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan finishedRun, 2)
	var callbacks atomic.Int32
	require.NoError(t, s.StartScheduled(ctx, id, func(r ManualResult, err error) {
		callbacks.Add(1)
		done <- finishedRun{r, err}
	}))
	<-requestStarted
	cancel()
	var result finishedRun
	select {
	case result = <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("in-flight scheduled work did not finish after parent cancellation")
	}
	require.NoError(t, result.err)
	require.Equal(t, "stopped", result.result.Status)
	require.Equal(t, "cancelled", result.result.ErrorCode)
	require.Equal(t, danmaku.Unknown, result.result.Outcome)
	require.Zero(t, result.result.SentCount)
	require.Equal(t, 1, result.result.FailedCount)
	require.Equal(t, int32(1), calls.Load(), "an unknown in-flight result cannot continue or retry the loop")
	require.Equal(t, int32(1), callbacks.Load())
	require.ErrorIs(t, s.StopExecution(id), ErrNotRunning)
	var status, code string
	var finished sql.NullInt64
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT status, error_code, finished_at FROM executions WHERE id = ?`, id).
		Scan(&status, &code, &finished))
	require.Equal(t, "stopped", status)
	require.Equal(t, "cancelled", code)
	require.True(t, finished.Valid)
	_, outcomes := storedAttempts(t, db, id)
	require.Equal(t, []string{"unknown"}, outcomes)
	var attemptCode string
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT error_code FROM send_attempts WHERE execution_id = ?`, id).Scan(&attemptCode))
	require.Equal(t, "cancelled", attemptCode)
	select {
	case <-done:
		t.Fatal("scheduled completion callback ran more than once")
	default:
	}
}

func TestScheduledTakeoverRejectsMissingManualOrPreviouslyAttemptedRows(t *testing.T) {
	db := testDB(t)
	created := addSingleTask(t, db)
	calls := 0
	s := &Service{db: db, now: func() time.Time { return fixedTime }, sender: &fakeGate{
		send: func(context.Context, string, string) (danmaku.Result, error) {
			calls++
			return danmaku.Result{Outcome: danmaku.Accepted}, nil
		},
	}}
	for _, id := range []int64{0, -1, 999} {
		_, err := s.RunScheduled(context.Background(), id)
		require.ErrorIs(t, err, ErrExecutionNotFound)
	}
	manual, err := s.RunManual(context.Background(), created.ID)
	require.NoError(t, err)
	_, err = s.RunScheduled(context.Background(), manual.ExecutionID)
	require.ErrorIs(t, err, ErrNotScheduled)
	id := addWaitingExecution(t, db, created.ID)
	require.NoError(t, insertPending(context.Background(), db, id, 1, "unknown old attempt", fixedTime.UnixMilli()))
	_, err = s.RunScheduled(context.Background(), id)
	require.ErrorIs(t, err, ErrExecutionNotWaiting)
	require.Equal(t, 1, calls, "a waiting row with an existing attempt must never be replayed")
	_, outcomes := storedAttempts(t, db, id)
	require.Equal(t, []string{"pending"}, outcomes)
}
