package executor

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dongwlin/dan-tick/internal/danmaku"
	"github.com/stretchr/testify/require"
)

func TestTestSendPersistsExecutionAndAttemptBeforeSharingSender(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	message := strings.Repeat("弹", 40)
	var roomID int64
	require.NoError(t, db.QueryRowContext(ctx, `
INSERT INTO rooms (platform_room_id, created_at, updated_at)
VALUES ('12345', ?, ?) RETURNING id`,
		fixedTime.UnixMilli(), fixedTime.UnixMilli()).Scan(&roomID))

	var pendingSeen, nullTaskSeen, testKindSeen bool
	sender, closeServer := testSender(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		require.Equal(t, message, r.PostForm.Get("msg"))
		var taskID sql.NullInt64
		var trigger, status, attempt string
		if err := db.QueryRowContext(r.Context(), `
SELECT e.task_id, e.trigger_kind, e.status, a.result
FROM executions e JOIN send_attempts a ON a.execution_id = e.id
WHERE e.id = (SELECT MAX(id) FROM executions)`).Scan(&taskID, &trigger, &status, &attempt); err != nil {
			t.Errorf("read pending test execution: %v", err)
			return
		}
		nullTaskSeen = !taskID.Valid
		testKindSeen = trigger == "test"
		pendingSeen = status == "running" && attempt == "pending"
		_, _ = w.Write([]byte(`{"code":0}`))
	}, fakeCredentials{})
	defer closeServer()

	result, err := testExecutor(db, sender, nil).TestSend(ctx, "12345", message)
	require.NoError(t, err)
	require.True(t, pendingSeen, "test execution and pending attempt must be committed before HTTP")
	require.True(t, nullTaskSeen)
	require.True(t, testKindSeen)
	require.Equal(t, danmaku.Accepted, result.Outcome)
	require.Equal(t, "completed", result.Status)
	require.Equal(t, 1, result.SentCount)
	require.Zero(t, result.FailedCount)
	require.Empty(t, result.ErrorCode)
	require.Positive(t, result.ExecutionID)

	var taskID sql.NullInt64
	var trigger, status, name, platformID string
	var scheduled, started, finished sql.NullInt64
	var code sql.NullString
	var sent, failed int
	require.NoError(t, db.QueryRowContext(ctx, `
SELECT task_id, trigger_kind, status, task_name_snapshot, platform_room_id_snapshot,
scheduled_at, started_at, finished_at, sent_count, failed_count, error_code
FROM executions WHERE id = ?`, result.ExecutionID).Scan(
		&taskID, &trigger, &status, &name, &platformID,
		&scheduled, &started, &finished, &sent, &failed, &code))
	require.False(t, taskID.Valid)
	require.Equal(t, "test", trigger)
	require.Equal(t, "completed", status)
	require.Equal(t, TestSendTaskName, name)
	require.Equal(t, "12345", platformID)
	require.False(t, scheduled.Valid)
	require.Equal(t, fixedTime.UnixMilli(), started.Int64)
	require.Equal(t, fixedTime.UnixMilli(), finished.Int64)
	require.Equal(t, 1, sent)
	require.Zero(t, failed)
	require.False(t, code.Valid)

	var attemptResult, content string
	var messageNo, sequence int
	require.NoError(t, db.QueryRowContext(ctx, `
SELECT sequence, message_no, content_snapshot, result
FROM send_attempts WHERE execution_id = ?`, result.ExecutionID).Scan(
		&sequence, &messageNo, &content, &attemptResult))
	require.Equal(t, 1, sequence)
	require.Equal(t, 1, messageNo)
	require.Equal(t, message, content)
	require.Equal(t, "accepted", attemptResult)

	var tasks int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks`).Scan(&tasks))
	require.Zero(t, tasks, "a test send must not create or reuse a task")
}

func TestTestSendPersistsKnownFailedAndUnknownOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		reply      string
		httpStatus int
		wantStatus string
		wantCode   string
		wantResult string
	}{
		{"authentication", `{"code":-101,"message":"private upstream response"}`, http.StatusOK, "failed", "authentication", "failed"},
		{"rate limited", `{"code":10031}`, http.StatusOK, "failed", "rate_limited", "failed"},
		{"rejected", `{"code":-403}`, http.StatusOK, "failed", "rejected", "failed"},
		{"unknown HTTP status", "private upstream response", http.StatusServiceUnavailable, "failed", "http_status", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := testDB(t)
			ctx := context.Background()
			var roomID int64
			require.NoError(t, db.QueryRowContext(ctx, `
INSERT INTO rooms (platform_room_id, created_at, updated_at)
VALUES ('12345', 1, 1) RETURNING id`).Scan(&roomID))
			sender, closeServer := testSender(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.httpStatus)
				_, _ = w.Write([]byte(tc.reply))
			}, fakeCredentials{})
			defer closeServer()

			result, err := testExecutor(db, sender, nil).TestSend(ctx, "12345", testMessage)
			require.NoError(t, err)
			require.Equal(t, tc.wantStatus, result.Status)
			require.Equal(t, tc.wantCode, result.ErrorCode)
			require.Zero(t, result.SentCount)
			require.Equal(t, 1, result.FailedCount)

			var status, code, attempt string
			require.NoError(t, db.QueryRowContext(ctx, `
SELECT e.status, e.error_code, a.result
FROM executions e JOIN send_attempts a ON a.execution_id = e.id
WHERE e.id = ?`, result.ExecutionID).Scan(&status, &code, &attempt))
			require.Equal(t, tc.wantStatus, status)
			require.Equal(t, tc.wantCode, code)
			require.Equal(t, tc.wantResult, attempt)
		})
	}
}

func TestTestSendRejectsInvalidInputAndClosedExecutorWithoutHistory(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	calls := 0
	svc := &Service{
		db: db, now: func() time.Time { return fixedTime },
		sender: &fakeGate{send: func(context.Context, string, string) (danmaku.Result, error) {
			calls++
			return danmaku.Result{Outcome: danmaku.Accepted}, nil
		}},
	}

	for _, tc := range []struct {
		message string
		want    error
	}{
		{"   ", danmaku.ErrInvalidInput},
		{"text\nline", danmaku.ErrInvalidInput},
		{string([]byte{0xff}), danmaku.ErrInvalidInput},
		{strings.Repeat("弹", 41), danmaku.ErrTooLong},
	} {
		_, err := svc.TestSend(ctx, "12345", tc.message)
		require.ErrorIs(t, err, tc.want)
	}
	_, err := svc.TestSend(ctx, "", testMessage)
	require.ErrorIs(t, err, danmaku.ErrInvalidInput)

	svc.mu.Lock()
	svc.closed = true
	svc.mu.Unlock()
	_, err = svc.TestSend(ctx, "12345", testMessage)
	require.ErrorIs(t, err, ErrExecutorClosed)

	var count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM executions`).Scan(&count))
	require.Zero(t, count)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM send_attempts`).Scan(&count))
	require.Zero(t, count)
	require.Zero(t, calls)
}
