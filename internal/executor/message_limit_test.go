package executor

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dongwlin/dan-tick/internal/credential"
	"github.com/dongwlin/dan-tick/internal/danmaku"
	"github.com/dongwlin/dan-tick/internal/task"
	"github.com/stretchr/testify/require"
)

func TestRunManualRejectsLegacyOverlongMessageWithoutChangingTaskOrSnapshot(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	created := addSingleTask(t, db)
	legacyMessage := strings.Repeat("弹", 41)
	// Simulate a task saved before submission validation existed.
	_, err := db.ExecContext(ctx, "UPDATE task_messages SET content = ? WHERE task_id = ?", legacyMessage, created.ID)
	require.NoError(t, err)
	tasks := task.NewService(db)
	before, err := tasks.Get(ctx, created.ID)
	require.NoError(t, err)

	var requests atomic.Int32
	sender, closeServer := testSender(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{"code":0}`))
	}, fakeCredentials{err: credential.ErrUnavailable})
	defer closeServer()

	result, err := testExecutor(db, sender, nil).RunManual(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, danmaku.NotSent, result.Outcome)
	require.Equal(t, "failed", result.Status)
	require.Equal(t, "too_long", result.ErrorCode,
		"validation must precede credential loading, which would report credential_unavailable")
	require.Zero(t, result.SentCount)
	require.Equal(t, 1, result.FailedCount)
	require.Zero(t, requests.Load())

	var status, executionCode, attemptResult, attemptCode, snapshot string
	require.NoError(t, db.QueryRowContext(ctx, `
SELECT e.status, e.error_code, a.result, a.error_code, a.content_snapshot
FROM executions e JOIN send_attempts a ON a.execution_id = e.id
WHERE e.id = ?`, result.ExecutionID).Scan(
		&status, &executionCode, &attemptResult, &attemptCode, &snapshot))
	require.Equal(t, "failed", status)
	require.Equal(t, "too_long", executionCode)
	require.Equal(t, "failed", attemptResult)
	require.Equal(t, "too_long", attemptCode)
	require.Equal(t, legacyMessage, snapshot, "history retains the complete attempted text")
	var executions, attempts int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM executions").Scan(&executions))
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM send_attempts").Scan(&attempts))
	require.Equal(t, 1, executions)
	require.Equal(t, 1, attempts)

	after, err := tasks.Get(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, before, after, "manual rejection preserves task content and its plan cursor")
	require.Equal(t, created.NextDueAt, after.NextDueAt)
}
