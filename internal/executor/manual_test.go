package executor

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dongwlin/dan-tick/internal/credential"
	"github.com/dongwlin/dan-tick/internal/danmaku"
	"github.com/dongwlin/dan-tick/internal/diagnostic"
	"github.com/dongwlin/dan-tick/internal/schedule"
	"github.com/dongwlin/dan-tick/internal/storage"
	"github.com/dongwlin/dan-tick/internal/task"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

var fixedTime = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

const (
	testMessage = "private example message"
	testSession = "session_private_value"
	testCSRF    = "csrf_private_value"
)

type fakeCredentials struct{ err error }

func (f fakeCredentials) Load() (credential.Credentials, error) {
	if f.err != nil {
		return credential.Credentials{}, f.err
	}
	return credential.Credentials{SESSDATA: testSession, BiliJCT: testCSRF}, nil
}

type rewriteTransport struct{ target *url.URL }

func (tr rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	copyReq := req.Clone(req.Context())
	u := *req.URL
	u.Scheme, u.Host = tr.target.Scheme, tr.target.Host
	copyReq.URL = &u
	return http.DefaultTransport.RoundTrip(copyReq)
}

func testSender(t *testing.T, handler http.HandlerFunc, creds fakeCredentials) (*danmaku.Sender, func()) {
	t.Helper()
	server := httptest.NewServer(handler)
	target, err := url.Parse(server.URL)
	require.NoError(t, err)
	client := danmaku.NewClient(creds, &http.Client{Transport: rewriteTransport{target: target}})
	return danmaku.NewSender(client), server.Close
}

func testDB(t *testing.T) *bun.DB {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "execution.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

func addSingleTask(t *testing.T, db *bun.DB) task.Task {
	t.Helper()
	ctx := context.Background()
	var roomID int64
	err := db.QueryRowContext(ctx, `
		INSERT INTO rooms (platform_room_id, created_at, updated_at)
		VALUES ('12345', ?, ?) RETURNING id`, fixedTime.UnixMilli(), fixedTime.UnixMilli()).Scan(&roomID)
	require.NoError(t, err)
	svc := task.NewService(db)
	created, err := svc.Create(ctx, task.CreateInput{
		RoomID: roomID, Name: "Morning greetings", Kind: task.Single,
		Messages: []string{testMessage},
		Schedule: schedule.Rule{Kind: schedule.Daily, LocalTime: "22:00:00"},
		Enabled:  true,
	})
	require.NoError(t, err)
	return created
}

func testExecutor(db *bun.DB, sender *danmaku.Sender, log *diagnostic.Logger) *Service {
	executor := NewService(db, sender, log)
	executor.now = func() time.Time { return fixedTime }
	return executor
}

func checkStored(t *testing.T, db *bun.DB, r ManualResult, wantAttempt string, due *int64) {
	t.Helper()
	var status, trigger, name, externalID string
	var scheduledAt, finishedAt, startedAt sql.NullInt64
	var sent, failed int
	var code sql.NullString
	err := db.QueryRowContext(context.Background(), `
		SELECT status, trigger_kind, task_name_snapshot, platform_room_id_snapshot,
			scheduled_at, started_at, finished_at, sent_count, failed_count, error_code
		FROM executions WHERE id = ?`, r.ExecutionID).Scan(
		&status, &trigger, &name, &externalID, &scheduledAt, &startedAt, &finishedAt, &sent, &failed, &code)
	require.NoError(t, err)
	require.Equal(t, r.Status, status)
	require.Equal(t, "manual", trigger)
	require.Equal(t, "Morning greetings", name)
	require.Equal(t, "12345", externalID)
	require.False(t, scheduledAt.Valid)
	require.Equal(t, fixedTime.UnixMilli(), startedAt.Int64)
	require.Equal(t, fixedTime.UnixMilli(), finishedAt.Int64)
	require.Equal(t, r.SentCount, sent)
	require.Equal(t, r.FailedCount, failed)
	require.Equal(t, r.ErrorCode, code.String)

	var attemptResult, content string
	var seq, messageNo int
	var attemptedAt int64
	var attemptCode sql.NullString
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT sequence, message_no, content_snapshot, attempted_at, result, error_code
		FROM send_attempts WHERE execution_id = ?`, r.ExecutionID).Scan(
		&seq, &messageNo, &content, &attemptedAt, &attemptResult, &attemptCode))
	require.Equal(t, 1, seq)
	require.Equal(t, 1, messageNo)
	require.Equal(t, testMessage, content)
	require.Equal(t, fixedTime.UnixMilli(), attemptedAt)
	require.Equal(t, wantAttempt, attemptResult)
	require.Equal(t, r.ErrorCode, attemptCode.String)
	var count int
	require.NoError(t, db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM send_attempts WHERE execution_id = ?", r.ExecutionID).Scan(&count))
	require.Equal(t, 1, count)
	var after sql.NullInt64
	require.NoError(t, db.QueryRowContext(context.Background(),
		"SELECT next_due_at FROM tasks WHERE id = (SELECT task_id FROM executions WHERE id = ?)", r.ExecutionID).Scan(&after))
	if due == nil {
		require.False(t, after.Valid)
	} else {
		require.Equal(t, *due, after.Int64)
	}
}

func TestRunManualPersistsPendingBeforeHTTPAndFinalResults(t *testing.T) {
	for _, tc := range []struct {
		name, reply, status, attempt, code string
		httpStatus                         int
		sent, failed                       int
	}{
		{"accepted", `{"code":0}`, "completed", "accepted", "", 200, 1, 0},
		{"authentication", `{"code":-101,"message":"private upstream response"}`, "failed", "failed", "authentication", 200, 0, 1},
		{"rate limited", `{"code":10031}`, "failed", "failed", "rate_limited", 200, 0, 1},
		{"uncertain HTTP", "private upstream response", "failed", "unknown", "http_status", 503, 0, 1},
		{"uncertain response", "private upstream response", "failed", "unknown", "invalid_response", 200, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := testDB(t)
			created := addSingleTask(t, db)
			pendingSeen := false
			sender, closeServer := testSender(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/msg/send" {
					t.Errorf("unexpected HTTP request: %s %s", r.Method, r.URL.Path)
				}
				var status, result string
				err := db.QueryRowContext(r.Context(), `
					SELECT e.status, a.result FROM executions e
					JOIN send_attempts a ON a.execution_id = e.id
					WHERE e.task_id = ?`, created.ID).Scan(&status, &result)
				if err != nil {
					t.Errorf("read pending before HTTP: %v", err)
				}
				pendingSeen = status == "running" && result == "pending"
				w.WriteHeader(tc.httpStatus)
				_, _ = w.Write([]byte(tc.reply))
			}, fakeCredentials{})
			defer closeServer()
			logDir := t.TempDir()
			log := diagnostic.New(logDir)
			defer log.Close()
			result, err := testExecutor(db, sender, log).RunManual(context.Background(), created.ID)
			require.NoError(t, err)
			require.True(t, pendingSeen, "HTTP must not start until pending is committed")
			require.Positive(t, result.ExecutionID)
			require.Equal(t, tc.status, result.Status)
			require.Equal(t, tc.code, result.ErrorCode)
			require.Equal(t, tc.sent, result.SentCount)
			require.Equal(t, tc.failed, result.FailedCount)
			checkStored(t, db, result, tc.attempt, created.NextDueAt)

			data, err := os.ReadFile(filepath.Join(logDir, "dan-tick.jsonl"))
			require.NoError(t, err)
			for _, secret := range []string{testMessage, testSession, testCSRF, "private upstream response"} {
				require.NotContains(t, string(data), secret)
			}
			require.Contains(t, string(data), `"execution_id":`)
			require.Contains(t, string(data), `"task_id":`)
			require.Contains(t, string(data), `"event":"execution.started"`)
			require.Contains(t, string(data), `"event":"send.resolved"`)
		})
	}
}

func TestRunManualSurvivesDatabaseReopenWithoutChangingPlan(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.db")
	db, err := storage.Open(context.Background(), path)
	require.NoError(t, err)
	created := addSingleTask(t, db)
	// A disabled task may be invoked manually. That does not re-enable its plan.
	_, err = task.NewService(db).SetEnabled(context.Background(), created.ID, false)
	require.NoError(t, err)
	sender, closeServer := testSender(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0}`))
	}, fakeCredentials{})
	defer closeServer()
	result, err := testExecutor(db, sender, nil).RunManual(context.Background(), created.ID)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	reopened, err := storage.Open(context.Background(), path)
	require.NoError(t, err)
	defer reopened.Close()
	checkStored(t, reopened, result, "accepted", nil)
}

func TestRunManualConflictAndInvalidTaskDoNotSend(t *testing.T) {
	db := testDB(t)
	created := addSingleTask(t, db)
	arrived, release := make(chan struct{}), make(chan struct{})
	sender, closeServer := testSender(t, func(w http.ResponseWriter, r *http.Request) {
		close(arrived)
		<-release
		_, _ = w.Write([]byte(`{"code":0}`))
	}, fakeCredentials{})
	defer closeServer()
	s := testExecutor(db, sender, nil)
	done := make(chan error, 1)
	go func() {
		_, err := s.RunManual(context.Background(), created.ID)
		done <- err
	}()
	<-arrived
	_, err := s.RunManual(context.Background(), created.ID)
	require.ErrorIs(t, err, task.ErrTaskBusy)
	_, err = s.RunManual(context.Background(), -1)
	require.ErrorIs(t, err, task.ErrNotFound)
	_, err = s.RunManual(context.Background(), 987654)
	require.ErrorIs(t, err, task.ErrNotFound)
	close(release)
	require.NoError(t, <-done)
	var executions int
	require.NoError(t, db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM executions").Scan(&executions))
	require.Equal(t, 1, executions)

	// Corrupt / deleted tasks cannot initiate a new execution.
	_, err = db.ExecContext(context.Background(),
		"UPDATE tasks SET deleted_at = ? WHERE id = ?", fixedTime.UnixMilli(), created.ID)
	require.NoError(t, err)
	_, err = s.RunManual(context.Background(), created.ID)
	require.ErrorIs(t, err, task.ErrNotFound)
}

func TestLoopCancellationDuringWaitAfterFirstHTTP(t *testing.T) {
	db := testDB(t)
	created := addSingleTask(t, db)
	_, err := db.ExecContext(context.Background(), `
		UPDATE tasks SET kind = 'loop', interval_seconds = 15, max_count = 2 WHERE id = ?`, created.ID)
	require.NoError(t, err)
	sender, closeServer := testSender(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0}`))
	}, fakeCredentials{})
	defer closeServer()
	s := testExecutor(db, sender, nil)
	// The first request succeeds, but a cancelled wait must prevent a
	// second HTTP request rather than sleeping for the real account interval.
	ctx, cancel := context.WithCancel(context.Background())
	s.wait = func(ctx context.Context, d time.Duration) error {
		cancel()
		return ctx.Err()
	}
	result, err := s.RunManual(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, "stopped", result.Status)
	require.Equal(t, 1, result.SentCount)
	require.Equal(t, "cancelled", result.ErrorCode)
	var count int
	require.NoError(t, db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM executions").Scan(&count))
	require.Equal(t, 1, count)
	require.NoError(t, db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM send_attempts").Scan(&count))
	require.Equal(t, 1, count)
}

func TestRunManualPendingFailureRollsBackAndPreventsHTTP(t *testing.T) {
	db := testDB(t)
	created := addSingleTask(t, db)
	_, err := db.ExecContext(context.Background(), `
		CREATE TRIGGER fail_pending BEFORE INSERT ON send_attempts
		BEGIN SELECT RAISE(ABORT, 'injected failure'); END`)
	require.NoError(t, err)
	sender, closeServer := testSender(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no HTTP request if pending cannot commit")
	}, fakeCredentials{})
	defer closeServer()
	_, err = testExecutor(db, sender, nil).RunManual(context.Background(), created.ID)
	require.Error(t, err)
	var count int
	require.NoError(t, db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM executions").Scan(&count))
	require.Zero(t, count)
}

func TestRunManualCancelsButResolvesPending(t *testing.T) {
	db := testDB(t)
	created := addSingleTask(t, db)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sender := &fakeGate{send: func(ctx context.Context, room, message string) (danmaku.Result, error) {
		cancel()
		return danmaku.Result{Outcome: danmaku.Unknown}, errors.Join(danmaku.ErrUncertain, context.Canceled)
	}}
	s := &Service{db: db, sender: sender, now: func() time.Time { return fixedTime }}
	result, err := s.RunManual(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, "cancelled", result.ErrorCode)
	require.Equal(t, danmaku.Unknown, result.Outcome)
	checkStored(t, db, result, "unknown", created.NextDueAt)
}

func TestRunManualFailedFinalWriteLeavesRecoverablePending(t *testing.T) {
	db := testDB(t)
	created := addSingleTask(t, db)
	_, err := db.ExecContext(context.Background(), `
		CREATE TRIGGER fail_finalize BEFORE UPDATE ON executions
		WHEN NEW.status = 'completed'
		BEGIN SELECT RAISE(ABORT, 'injected failure'); END`)
	require.NoError(t, err)
	sender, closeServer := testSender(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0}`))
	}, fakeCredentials{})
	defer closeServer()
	result, err := testExecutor(db, sender, nil).RunManual(context.Background(), created.ID)
	require.Error(t, err)
	require.Positive(t, result.ExecutionID)
	var status, attempt string
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT e.status, a.result FROM executions e JOIN send_attempts a ON a.execution_id = e.id
		WHERE e.id = ?`, result.ExecutionID).Scan(&status, &attempt))
	require.Equal(t, "running", status)
	require.Equal(t, "pending", attempt, "finalization must be atomic; E3 will recover it as unknown")
}

type fakeGate struct {
	send func(context.Context, string, string) (danmaku.Result, error)
}

func (f *fakeGate) Send(ctx context.Context, room, msg string) (danmaku.Result, error) {
	return f.send(ctx, room, msg)
}

func (f *fakeGate) SendWithAdmission(ctx context.Context, room, msg string, admitted func(context.Context) error) (danmaku.Result, error) {
	if err := ctx.Err(); err != nil {
		return danmaku.Result{Outcome: danmaku.NotSent}, err
	}
	if err := admitted(ctx); err != nil {
		return danmaku.Result{Outcome: danmaku.NotSent}, err
	}
	return f.Send(ctx, room, msg)
}

func TestRunManualCredentialMissingIsSafe(t *testing.T) {
	db := testDB(t)
	created := addSingleTask(t, db)
	sender, closeServer := testSender(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("missing credentials must prevent HTTP")
	}, fakeCredentials{err: errors.New("private provider secret")})
	defer closeServer()
	r, err := testExecutor(db, sender, nil).RunManual(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, "credential_unavailable", r.ErrorCode)
	checkStored(t, db, r, "failed", created.NextDueAt)
	payload := r.ErrorCode + r.Status
	require.False(t, strings.Contains(payload, "private"))
}
