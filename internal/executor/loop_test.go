package executor

import (
	"context"
	"database/sql"
	"errors"
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

type loopClock struct {
	mu    sync.Mutex
	at    time.Time
	waits []time.Duration
}

func intPtr(value int64) *int64 { return &value }

func (c *loopClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *loopClock) Wait(ctx context.Context, delay time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	c.waits = append(c.waits, delay)
	c.at = c.at.Add(delay)
	c.mu.Unlock()
	return ctx.Err()
}

func addLoopTask(t *testing.T, db *bun.DB, messages []string, interval int64, count, duration *int64) task.Task {
	t.Helper()
	created := addSingleTask(t, db)
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `
		UPDATE tasks SET kind = 'loop', interval_seconds = ?,
			max_count = ?, max_duration_seconds = ? WHERE id = ?`,
		interval, count, duration, created.ID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "DELETE FROM task_messages WHERE task_id = ?", created.ID)
	require.NoError(t, err)
	for i, message := range messages {
		_, err = db.ExecContext(ctx, `
			INSERT INTO task_messages (task_id, position, content) VALUES (?, ?, ?)`,
			created.ID, i, message)
		require.NoError(t, err)
	}
	return created
}

func loopService(db *bun.DB, clock *loopClock, send func(context.Context, string, string) (danmaku.Result, error)) *Service {
	return &Service{db: db, sender: &fakeGate{send: send}, now: clock.Now, wait: clock.Wait}
}

func storedAttempts(t *testing.T, db *bun.DB, id int64) (messages, results []string) {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), `
		SELECT content_snapshot, result FROM send_attempts
		WHERE execution_id = ? ORDER BY sequence`, id)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var content, result string
		require.NoError(t, rows.Scan(&content, &result))
		messages = append(messages, content)
		results = append(results, result)
	}
	require.NoError(t, rows.Err())
	return
}

func TestLoopOrdersMessagesAndHonorsCountAndInterval(t *testing.T) {
	db := testDB(t)
	created := addLoopTask(t, db, []string{"one", "two", "three"}, 20, intPtr(5), nil)
	clock := &loopClock{at: fixedTime}
	var started []time.Time
	var called []string
	s := loopService(db, clock, func(ctx context.Context, room, message string) (danmaku.Result, error) {
		var pending string
		require.NoError(t, db.QueryRowContext(ctx, `
			SELECT result FROM send_attempts WHERE execution_id =
				(SELECT id FROM executions WHERE task_id = ?) AND sequence = ?`,
			created.ID, len(called)+1).Scan(&pending))
		require.Equal(t, "pending", pending, "attempt must commit before each request")
		require.Equal(t, "12345", room)
		started = append(started, clock.Now())
		called = append(called, message)
		return danmaku.Result{Outcome: danmaku.Accepted}, nil
	})
	var progress []int
	s.SetOnChange(func(id, taskID int64) {
		require.Equal(t, created.ID, taskID)
		var count int
		var status string
		require.NoError(t, db.QueryRowContext(context.Background(),
			"SELECT status, sent_count FROM executions WHERE id = ?", id).Scan(&status, &count))
		if status == "running" && count > 0 && (len(progress) == 0 || progress[len(progress)-1] != count) {
			progress = append(progress, count)
		}
	})
	result, err := s.RunManual(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", result.Status)
	require.Equal(t, 5, result.SentCount)
	require.Equal(t, []int{1, 2, 3, 4}, progress, "running counters must notify after each committed nonterminal send")
	require.Zero(t, result.FailedCount)
	require.Equal(t, []string{"one", "two", "three", "one", "two"}, called)
	require.Equal(t, []time.Duration{20 * time.Second, 20 * time.Second, 20 * time.Second, 20 * time.Second}, clock.waits)
	for i, timestamp := range started {
		require.Equal(t, fixedTime.Add(time.Duration(i)*20*time.Second), timestamp)
	}
	messages, outcomes := storedAttempts(t, db, result.ExecutionID)
	require.Equal(t, called, messages)
	require.Equal(t, []string{"accepted", "accepted", "accepted", "accepted", "accepted"}, outcomes)
	var status string
	var sent, failed int
	var finished int64
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT status, sent_count, failed_count, finished_at
		FROM executions WHERE id = ?`, result.ExecutionID).Scan(&status, &sent, &failed, &finished))
	require.Equal(t, "completed", status)
	require.Equal(t, 5, sent)
	require.Zero(t, failed)
	require.Equal(t, fixedTime.Add(80*time.Second).UnixMilli(), finished)
	var due int64
	require.NoError(t, db.QueryRowContext(context.Background(), "SELECT next_due_at FROM tasks WHERE id = ?", created.ID).Scan(&due))
	require.Equal(t, *created.NextDueAt, due)
}

type delayedLoopGate struct {
	clock    *loopClock
	requests []time.Time
}

func (g *delayedLoopGate) SendWithAdmission(ctx context.Context, _, _ string, admitted func(context.Context) error) (danmaku.Result, error) {
	if len(g.requests) == 0 {
		// Other tasks own the account before this task gets its first turn.
		if err := g.clock.Wait(ctx, 50*time.Second); err != nil {
			return danmaku.Result{Outcome: danmaku.NotSent}, err
		}
	} else if delay := g.requests[len(g.requests)-1].Add(15 * time.Second).Sub(g.clock.Now()); delay > 0 {
		if err := g.clock.Wait(ctx, delay); err != nil {
			return danmaku.Result{Outcome: danmaku.NotSent}, err
		}
	}
	if err := admitted(ctx); err != nil {
		return danmaku.Result{Outcome: danmaku.NotSent}, err
	}
	// The real Client reads Keyring after admission, so even admission is too
	// early to start the next task interval. Then the HTTP response takes time.
	if err := g.clock.Wait(ctx, 14*time.Second); err != nil {
		return danmaku.Result{Outcome: danmaku.NotSent}, err
	}
	g.requests = append(g.requests, g.clock.Now())
	if err := g.clock.Wait(ctx, 5*time.Second); err != nil {
		return danmaku.Result{Outcome: danmaku.Unknown}, err
	}
	return danmaku.Result{Outcome: danmaku.Accepted}, nil
}

func TestLoopIntervalSurvivesQueueCredentialAndResponseDelays(t *testing.T) {
	db := testDB(t)
	created := addLoopTask(t, db, []string{"one", "two"}, 60, intPtr(2), nil)
	clock := &loopClock{at: fixedTime}
	gate := &delayedLoopGate{clock: clock}
	s := &Service{db: db, sender: gate, now: clock.Now, wait: clock.Wait}
	result, err := s.RunManual(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", result.Status)
	require.Len(t, gate.requests, 2)
	require.GreaterOrEqual(t, gate.requests[1].Sub(gate.requests[0]), 60*time.Second,
		"queue and credential waits must not shorten the configured HTTP gap")
}

type waitingDurationGate struct{}

func (waitingDurationGate) SendWithAdmission(ctx context.Context, _, _ string, _ func(context.Context) error) (danmaku.Result, error) {
	<-ctx.Done()
	return danmaku.Result{Outcome: danmaku.NotSent}, ctx.Err()
}

func TestLoopConfiguredDurationExpiresNormallyWhileQueued(t *testing.T) {
	db := testDB(t)
	created := addLoopTask(t, db, []string{"one", "two"}, 15, nil, intPtr(1))
	s := &Service{db: db, sender: waitingDurationGate{}, now: time.Now, wait: waitInterval}
	result, err := s.RunManual(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", result.Status)
	require.Equal(t, danmaku.NotSent, result.Outcome)
	require.Empty(t, result.ErrorCode)
	require.Zero(t, result.SentCount)
	var started sql.NullInt64
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT started_at FROM executions WHERE id = ?`, result.ExecutionID).Scan(&started))
	require.False(t, started.Valid, "the configured limit was reached before actual execution")
	var attempt, code string
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT result, error_code FROM send_attempts WHERE execution_id = ?`, result.ExecutionID).Scan(&attempt, &code))
	require.Equal(t, "failed", attempt)
	require.Equal(t, "cancelled", code, "never claim a network timeout for an unsent request")
}

func TestLoopCallerDeadlineDoesNotPretendConfiguredDurationWasReached(t *testing.T) {
	db := testDB(t)
	created := addLoopTask(t, db, []string{"one", "two"}, 15, nil, intPtr(100))
	var calls int
	s := &Service{db: db, now: time.Now, wait: waitInterval, sender: &fakeGate{
		send: func(context.Context, string, string) (danmaku.Result, error) {
			calls++
			return danmaku.Result{Outcome: danmaku.Accepted}, nil
		},
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	result, err := s.RunManual(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, "stopped", result.Status)
	require.Equal(t, "cancelled", result.ErrorCode)
	require.Equal(t, 1, calls)
}

func TestLoopDurationAndCountUseFirstReachedLimit(t *testing.T) {
	for _, tc := range []struct {
		name          string
		count         *int64
		duration      *int64
		want          int
		wantWaits     []time.Duration
		wantFinishSec int64
	}{
		{"duration only", nil, intPtr(35), 3, []time.Duration{15 * time.Second, 15 * time.Second, 5 * time.Second}, 35},
		{"duration sooner than count", intPtr(10), intPtr(22), 2, []time.Duration{15 * time.Second, 7 * time.Second}, 22},
		{"count sooner than duration", intPtr(2), intPtr(100), 2, []time.Duration{15 * time.Second}, 15},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := testDB(t)
			created := addLoopTask(t, db, []string{"A", "B"}, 15, tc.count, tc.duration)
			clock := &loopClock{at: fixedTime}
			calls := 0
			s := loopService(db, clock, func(ctx context.Context, room, msg string) (danmaku.Result, error) {
				calls++
				return danmaku.Result{Outcome: danmaku.Accepted}, nil
			})
			got, err := s.RunManual(context.Background(), created.ID)
			require.NoError(t, err)
			require.Equal(t, "completed", got.Status)
			require.Equal(t, tc.want, calls)
			require.Equal(t, tc.want, got.SentCount)
			require.Equal(t, tc.wantWaits, clock.waits)
			require.Equal(t, fixedTime.Add(time.Duration(tc.wantFinishSec)*time.Second), clock.Now())
			messages, results := storedAttempts(t, db, got.ExecutionID)
			require.Len(t, messages, tc.want)
			require.Len(t, results, tc.want)
		})
	}
}

func TestLoopDeadlineCancelsInFlightRequest(t *testing.T) {
	db := testDB(t)
	created := addLoopTask(t, db, []string{"one", "two"}, 15, nil, intPtr(1))
	s := &Service{
		db: db, now: time.Now, wait: waitInterval,
		sender: &fakeGate{send: func(ctx context.Context, room, message string) (danmaku.Result, error) {
			<-ctx.Done()
			return danmaku.Result{Outcome: danmaku.Unknown}, errors.Join(danmaku.ErrUncertain, danmaku.ErrTimeout)
		}},
	}
	result, err := s.RunManual(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, "failed", result.Status)
	require.Equal(t, "timeout", result.ErrorCode)
	require.Zero(t, result.SentCount)
	require.Equal(t, 1, result.FailedCount)
	_, outcomes := storedAttempts(t, db, result.ExecutionID)
	require.Equal(t, []string{"unknown"}, outcomes)
}

func TestLoopRefusesMissingStopCondition(t *testing.T) {
	db := testDB(t)
	created := addSingleTask(t, db)
	_, err := task.NewService(db).Create(context.Background(), task.CreateInput{
		RoomID: created.RoomID,
		Name:   "invalid loop", Kind: task.Loop,
		Messages: []string{"A"}, IntervalSeconds: intPtr(15),
		Schedule: schedule.Rule{Kind: schedule.Daily, LocalTime: "22:00:00"},
		Enabled:  true,
	})
	require.ErrorIs(t, err, task.ErrInvalidTask)
}

func TestLoopFailureOrUnknownPreventsRemainingSends(t *testing.T) {
	for _, tc := range []struct {
		name    string
		outcome danmaku.Outcome
		sendErr error
		code    string
		attempt string
	}{
		{"rejected", danmaku.Rejected, danmaku.ErrRateLimited, "rate_limited", "failed"},
		{"unknown", danmaku.Unknown, errors.Join(danmaku.ErrUncertain, danmaku.ErrTimeout), "timeout", "unknown"},
		{"not sent", danmaku.NotSent, errors.New("sensitive provider text"), "send_failure", "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := testDB(t)
			created := addLoopTask(t, db, []string{"a", "b", "c"}, 15, intPtr(12), nil)
			clock := &loopClock{at: fixedTime}
			calls := 0
			s := loopService(db, clock, func(ctx context.Context, room, msg string) (danmaku.Result, error) {
				calls++
				if calls == 1 {
					return danmaku.Result{Outcome: danmaku.Accepted}, nil
				}
				return danmaku.Result{Outcome: tc.outcome}, tc.sendErr
			})
			got, err := s.RunManual(context.Background(), created.ID)
			require.NoError(t, err)
			require.Equal(t, "failed", got.Status)
			require.Equal(t, tc.code, got.ErrorCode)
			require.Equal(t, 1, got.SentCount)
			require.Equal(t, 1, got.FailedCount)
			require.Equal(t, 2, calls)
			messages, outcomes := storedAttempts(t, db, got.ExecutionID)
			require.Equal(t, []string{"a", "b"}, messages)
			require.Equal(t, []string{"accepted", tc.attempt}, outcomes)
		})
	}
}

func TestLoopPendingWriteFailurePreventsNextRequest(t *testing.T) {
	db := testDB(t)
	created := addLoopTask(t, db, []string{"first", "second"}, 15, intPtr(2), nil)
	_, err := db.ExecContext(context.Background(), `
		CREATE TRIGGER reject_second BEFORE INSERT ON send_attempts
		WHEN NEW.sequence = 2
		BEGIN SELECT RAISE(ABORT, 'injected failure'); END`)
	require.NoError(t, err)
	clock := &loopClock{at: fixedTime}
	calls := 0
	s := loopService(db, clock, func(ctx context.Context, room, message string) (danmaku.Result, error) {
		calls++
		return danmaku.Result{Outcome: danmaku.Accepted}, nil
	})
	got, err := s.RunManual(context.Background(), created.ID)
	require.Error(t, err)
	require.Positive(t, got.ExecutionID)
	require.Equal(t, 1, calls)
	messages, results := storedAttempts(t, db, got.ExecutionID)
	require.Equal(t, []string{"first"}, messages)
	require.Equal(t, []string{"accepted"}, results)
	var status string
	var sent int
	require.NoError(t, db.QueryRowContext(context.Background(),
		"SELECT status, sent_count FROM executions WHERE id = ?", got.ExecutionID).Scan(&status, &sent))
	require.Equal(t, "running", status, "E3 will resolve an unfinished run without replaying it")
	require.Equal(t, 1, sent)
}

func TestLoopWaitFailureDoesNotBypassInterval(t *testing.T) {
	db := testDB(t)
	created := addLoopTask(t, db, []string{"a", "b"}, 15, intPtr(3), nil)
	clock := &loopClock{at: fixedTime}
	calls := 0
	s := loopService(db, clock, func(ctx context.Context, room, message string) (danmaku.Result, error) {
		calls++
		return danmaku.Result{Outcome: danmaku.Accepted}, nil
	})
	s.wait = func(ctx context.Context, delay time.Duration) error {
		return errors.New("private timer diagnostics")
	}
	got, err := s.RunManual(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, "failed", got.Status)
	require.Equal(t, "wait_failure", got.ErrorCode)
	require.Equal(t, 1, calls)
	require.Equal(t, 1, got.SentCount)
	require.Zero(t, got.FailedCount)
	_, outcomes := storedAttempts(t, db, got.ExecutionID)
	require.Equal(t, []string{"accepted"}, outcomes)
}

func TestStopExecutionWhileWaitingDoesNotSendNextMessage(t *testing.T) {
	db := testDB(t)
	created := addLoopTask(t, db, []string{"a", "b"}, 15, intPtr(3), nil)
	clock := &loopClock{at: fixedTime}
	waiting := make(chan struct{})
	calls := 0
	s := loopService(db, clock, func(ctx context.Context, room, msg string) (danmaku.Result, error) {
		calls++
		return danmaku.Result{Outcome: danmaku.Accepted}, nil
	})
	s.wait = func(ctx context.Context, delay time.Duration) error {
		close(waiting)
		<-ctx.Done()
		return ctx.Err()
	}
	require.ErrorIs(t, s.StopExecution(999), ErrNotRunning)
	done := make(chan struct {
		r   ManualResult
		err error
	}, 1)
	go func() {
		result, err := s.RunManual(context.Background(), created.ID)
		done <- struct {
			r   ManualResult
			err error
		}{result, err}
	}()
	<-waiting
	var id int64
	require.NoError(t, db.QueryRowContext(context.Background(), "SELECT id FROM executions WHERE task_id = ?", created.ID).Scan(&id))
	require.NoError(t, s.StopExecution(id))
	got := <-done
	require.NoError(t, got.err)
	require.Equal(t, "stopped", got.r.Status)
	require.Equal(t, "user_stopped", got.r.ErrorCode)
	require.Equal(t, 1, got.r.SentCount)
	require.Zero(t, got.r.FailedCount)
	require.Equal(t, 1, calls)
	_, statuses := storedAttempts(t, db, got.r.ExecutionID)
	require.Equal(t, []string{"accepted"}, statuses)
	require.ErrorIs(t, s.StopExecution(id), ErrNotRunning)
}

func TestStopExecutionDuringRequestPersistsUnknownAndNoFutureAttempt(t *testing.T) {
	db := testDB(t)
	created := addLoopTask(t, db, []string{"a", "b"}, 15, intPtr(4), nil)
	clock := &loopClock{at: fixedTime}
	requestStarted := make(chan struct{})
	s := loopService(db, clock, func(ctx context.Context, room, msg string) (danmaku.Result, error) {
		close(requestStarted)
		<-ctx.Done()
		return danmaku.Result{Outcome: danmaku.Unknown}, errors.Join(danmaku.ErrUncertain, context.Canceled)
	})
	done := make(chan struct {
		r   ManualResult
		err error
	}, 1)
	go func() {
		result, err := s.RunManual(context.Background(), created.ID)
		done <- struct {
			r   ManualResult
			err error
		}{result, err}
	}()
	<-requestStarted
	var id int64
	require.NoError(t, db.QueryRowContext(context.Background(), "SELECT id FROM executions WHERE task_id = ?", created.ID).Scan(&id))
	require.NoError(t, s.StopExecution(id))
	got := <-done
	require.NoError(t, got.err)
	require.Equal(t, "stopped", got.r.Status)
	require.Equal(t, "user_stopped", got.r.ErrorCode)
	require.Zero(t, got.r.SentCount)
	require.Equal(t, 1, got.r.FailedCount)
	_, statuses := storedAttempts(t, db, got.r.ExecutionID)
	require.Equal(t, []string{"unknown"}, statuses)
}

func TestStoppingEmitsSafeDiagnosticEvent(t *testing.T) {
	db := testDB(t)
	created := addLoopTask(t, db, []string{testMessage}, 15, intPtr(3), nil)
	clock := &loopClock{at: fixedTime}
	log := diagnostic.New(t.TempDir())
	defer log.Close()
	s := loopService(db, clock, func(ctx context.Context, room, msg string) (danmaku.Result, error) {
		return danmaku.Result{Outcome: danmaku.Accepted}, nil
	})
	s.log = log
	s.wait = func(ctx context.Context, delay time.Duration) error {
		var id int64
		require.NoError(t, db.QueryRowContext(ctx,
			"SELECT id FROM executions WHERE task_id = ?", created.ID).Scan(&id))
		require.NoError(t, s.StopExecution(id))
		return ctx.Err()
	}
	got, err := s.RunManual(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, "stopped", got.Status)
	data, err := os.ReadFile(filepath.Join(log.Status().Directory, "dan-tick.jsonl"))
	require.NoError(t, err)
	require.Contains(t, string(data), `"event":"execution.stopped"`)
	require.Contains(t, string(data), `"error_category":"user_stopped"`)
	require.Contains(t, string(data), `"attempt_sequence":1`)
	require.NotContains(t, string(data), testMessage)
	require.NotContains(t, string(data), testSession)
}
