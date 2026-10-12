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
	err = s.ClearAccount(ctx)
	assertPublicError(t, err, "keyring_unavailable", "test_session_secret")
	require.Empty(t, events, "failed clear must not emit a success event")
	store.err = nil
	require.NoError(t, s.ClearAccount(ctx))
	require.True(t, store.cleared)
	require.Equal(t, []string{EventAccountChanged}, events)
}

type fakeQRChecker struct {
	fakeChecker
	err error
}

func (c fakeQRChecker) BeginQRLogin(context.Context) (account.QRLoginStatus, error) {
	return account.QRLoginStatus{State: "waiting", QRImage: "data:image/png;base64,fake", ExpiresAt: 123}, c.err
}
func (c fakeQRChecker) PollQRLogin(context.Context) (account.QRLoginStatus, error) {
	return account.QRLoginStatus{State: "account_mismatch", CandidateUID: "42"}, c.err
}
func (c fakeQRChecker) ConfirmQRAccount(context.Context) error { return c.err }
func (c fakeQRChecker) CancelQRLogin(context.Context) error    { return c.err }

func TestQRBindingExposesOnlyWorkflowAndSafeErrors(t *testing.T) {
	ctx := context.Background()
	s := NewOperationsService(nil, nil, fakeQRChecker{}, nil, nil, nil)
	begin, err := s.BeginQRLogin(ctx)
	require.NoError(t, err)
	require.Equal(t, "waiting", begin.State)
	require.Contains(t, begin.QRImage, "data:image/png;base64,")
	poll, err := s.PollQRLogin(ctx)
	require.NoError(t, err)
	require.Equal(t, "42", poll.CandidateUID)
	require.NoError(t, s.ConfirmQRAccount(ctx))
	require.NoError(t, s.CancelQRLogin(ctx))
	for _, failure := range []struct {
		err  error
		code string
	}{
		{account.ErrPassportUnavailable, "login_unavailable"},
		{account.ErrPassportExpired, "qr_expired"},
		{credential.ErrReauthenticationRequired, "reauthentication_required"},
		{errors.New("SESSDATA=private; refresh_token=private"), "internal"},
	} {
		s.account = fakeQRChecker{err: failure.err}
		_, err := s.BeginQRLogin(ctx)
		assertPublicError(t, err, failure.code, "private")
		_, err = s.PollQRLogin(ctx)
		assertPublicError(t, err, failure.code, "private")
		assertPublicError(t, s.ConfirmQRAccount(ctx), failure.code, "private")
	}
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
