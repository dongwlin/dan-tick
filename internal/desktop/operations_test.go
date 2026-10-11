package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/dongwlin/dan-tick/internal/account"
	"github.com/dongwlin/dan-tick/internal/credential"
	"github.com/dongwlin/dan-tick/internal/danmaku"
	"github.com/dongwlin/dan-tick/internal/executor"
	"github.com/stretchr/testify/require"
)

type fakeOperationsStore struct {
	credentials credential.Credentials
	err         error
	cleared     bool
}

func (s *fakeOperationsStore) Save(c credential.Credentials) error {
	if s.err != nil {
		return s.err
	}
	s.credentials = c
	return nil
}
func (s *fakeOperationsStore) Clear() error {
	if s.err != nil {
		return s.err
	}
	s.cleared = true
	return nil
}

type fakeChecker struct {
	status account.Status
	err    error
}

func (c fakeChecker) Check(context.Context) (account.Status, error) { return c.status, c.err }

type fakeTestSender struct {
	room, message string
	result        danmaku.Result
	err           error
}

func (s *fakeTestSender) TestSend(_ context.Context, room, message string) (danmaku.Result, error) {
	s.room, s.message = room, message
	return s.result, s.err
}

type fakeOperationsRunner struct {
	started, stopped int64
	err              error
}

func (s *fakeOperationsRunner) StartManual(id int64) (int64, error) {
	s.started = id
	return 23, s.err
}
func (s *fakeOperationsRunner) StopExecution(id int64) error {
	s.stopped = id
	return s.err
}

func TestOperationsServiceCredentialAndSendBoundary(t *testing.T) {
	ctx := context.Background()
	_, _, db := configurationTest(t)
	var roomID int64
	require.NoError(t, db.QueryRowContext(ctx, `INSERT INTO rooms (platform_room_id, created_at, updated_at)
		VALUES ('98765', 1, 1) RETURNING id`).Scan(&roomID))
	store := &fakeOperationsStore{}
	sender := &fakeTestSender{result: danmaku.Result{Outcome: danmaku.Accepted}}
	runner := &fakeOperationsRunner{}
	var events []string
	s := NewOperationsService(db, store, fakeChecker{status: account.Status{State: account.LoggedIn}}, sender, runner, func(event string) { events = append(events, event) })
	require.Equal(t, account.LoggedIn, s.CheckAccount(ctx).State)
	require.NoError(t, s.SaveAccount(ctx, SaveAccountInput{SESSDATA: "test_session_secret", BiliJCT: "test_csrf_secret"}))
	require.Equal(t, "test_session_secret", store.credentials.SESSDATA)
	require.Equal(t, []string{EventAccountChanged}, events)

	result, err := s.TestSend(ctx, TestSendInput{RoomID: roomID, Message: "private test text"})
	require.NoError(t, err)
	require.Equal(t, danmaku.Accepted, result.Outcome)
	require.Equal(t, "98765", sender.room)
	require.Equal(t, "private test text", sender.message)
	require.Empty(t, result.ErrorCode)
	_, err = s.TestSend(ctx, TestSendInput{RoomID: roomID + 10, Message: "private test text"})
	assertPublicError(t, err, "not_found", "private test text")

	sender.err = errors.New("upstream includes test_session_secret")
	sender.result = danmaku.Result{Outcome: danmaku.Unknown}
	result, err = s.TestSend(ctx, TestSendInput{RoomID: roomID, Message: "private test text"})
	require.NoError(t, err)
	require.Equal(t, "send_failed", result.ErrorCode)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "secret")

	store.err = credential.ErrUnavailable
	err = s.SaveAccount(ctx, SaveAccountInput{SESSDATA: "test_session_secret", BiliJCT: "test_csrf_secret"})
	assertPublicError(t, err, "keyring_unavailable", "test_session_secret")
	require.Len(t, events, 1, "failed save must not emit a success event")
	store.err = nil
	require.NoError(t, s.ClearAccount(ctx))
	require.True(t, store.cleared)
	require.Equal(t, []string{EventAccountChanged, EventAccountChanged}, events)
}

func TestOperationsServiceExecutionAndAccountStatus(t *testing.T) {
	ctx := context.Background()
	_, _, db := configurationTest(t)
	runner := &fakeOperationsRunner{}
	s := NewOperationsService(db, &fakeOperationsStore{}, fakeChecker{err: errors.New("private network error")}, &fakeTestSender{}, runner, nil)
	require.Equal(t, account.VerificationFailed, s.CheckAccount(ctx).State)
	result, err := s.StartManual(ctx, 12)
	require.NoError(t, err)
	require.EqualValues(t, 23, result.ExecutionID)
	require.EqualValues(t, 12, runner.started)
	require.NoError(t, s.StopExecution(ctx, result.ExecutionID))
	require.EqualValues(t, 23, runner.stopped)
	runner.err = executor.ErrNotRunning
	assertPublicError(t, s.StopExecution(ctx, 23), "not_running", "private")
	runner.err = executor.ErrExecutorClosed
	_, err = s.StartManual(ctx, 12)
	assertPublicError(t, err, "unavailable", "private")
	latest, err := s.LatestExecutions(ctx)
	require.NoError(t, err)
	require.Empty(t, latest)
}
