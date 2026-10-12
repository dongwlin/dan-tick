package account

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dongwlin/dan-tick/internal/credential"
	"github.com/stretchr/testify/require"
)

type sessionTestStore struct {
	mu          sync.Mutex
	credentials credential.Credentials
	loadErr     error
	failSave    int
	saved       []credential.Credentials
	clears      int
}

func (s *sessionTestStore) Load() (credential.Credentials, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.credentials, s.loadErr
}

func (s *sessionTestStore) Save(c credential.Credentials) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saved = append(s.saved, c)
	if len(s.saved) == s.failSave {
		return credential.ErrUnavailable
	}
	s.credentials, s.loadErr = c, nil
	return nil
}

func (s *sessionTestStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clears++
	s.credentials, s.loadErr = credential.Credentials{}, credential.ErrNotConfigured
	return nil
}

func (s *sessionTestStore) snapshot() (credential.Credentials, []credential.Credentials, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.credentials, append([]credential.Credentials(nil), s.saved...), s.clears
}

type sessionTestPassport struct {
	generate      func(context.Context) (QRChallenge, error)
	poll          func(context.Context, string) (QRResult, error)
	refreshNeeded func(context.Context, credential.Credentials) (bool, int64, error)
	refresh       func(context.Context, credential.Credentials, int64) (credential.Credentials, error)
	confirm       func(context.Context, credential.Credentials, string) error
	infoCalls     atomic.Int32
	refreshCalls  atomic.Int32
	confirmCalls  atomic.Int32
	pollCalls     atomic.Int32
}

func (p *sessionTestPassport) Generate(ctx context.Context) (QRChallenge, error) {
	if p.generate != nil {
		return p.generate(ctx)
	}
	return QRChallenge{URL: "https://passport.bilibili.com/qrcode/h5/login?qrcode_key=test_key", Key: "test_key"}, nil
}

func (p *sessionTestPassport) Poll(ctx context.Context, key string) (QRResult, error) {
	p.pollCalls.Add(1)
	if p.poll != nil {
		return p.poll(ctx, key)
	}
	return QRResult{State: "waiting"}, nil
}

func (p *sessionTestPassport) RefreshNeeded(ctx context.Context, c credential.Credentials) (bool, int64, error) {
	p.infoCalls.Add(1)
	if p.refreshNeeded != nil {
		return p.refreshNeeded(ctx, c)
	}
	return false, 1700000000000, nil
}

func (p *sessionTestPassport) Refresh(ctx context.Context, c credential.Credentials, timestamp int64) (credential.Credentials, error) {
	p.refreshCalls.Add(1)
	if p.refresh != nil {
		return p.refresh(ctx, c, timestamp)
	}
	return sessionTestCredentials("new", c.UID), nil
}

func (p *sessionTestPassport) Confirm(ctx context.Context, c credential.Credentials, oldToken string) error {
	p.confirmCalls.Add(1)
	if p.confirm != nil {
		return p.confirm(ctx, c, oldToken)
	}
	return nil
}

type sessionTestChecker func(context.Context, credential.Credentials) (Status, error)

func (f sessionTestChecker) CheckCredentials(ctx context.Context, c credential.Credentials) (Status, error) {
	return f(ctx, c)
}

func sessionTestCredentials(prefix, uid string) credential.Credentials {
	return credential.Credentials{
		SESSDATA: prefix + "_session", BiliJCT: prefix + "_csrf", Version: 1,
		Source: "qr", UID: uid, RefreshToken: prefix + "_token", RefreshState: "ready",
	}
}

func sessionTestFixture(t *testing.T, initial credential.Credentials) (*Session, *sessionTestStore, *sessionTestPassport) {
	t.Helper()
	store := &sessionTestStore{credentials: initial}
	protocol := &sessionTestPassport{}
	checker := sessionTestChecker(func(_ context.Context, c credential.Credentials) (Status, error) {
		return Status{State: LoggedIn, UID: c.UID}, nil
	})
	session := NewSession(store, protocol, checker, nil)
	t.Cleanup(session.Close)
	return session, store, protocol
}

func TestSessionBlocksLegacyAndInterruptedCredentials(t *testing.T) {
	for _, state := range []string{"legacy", "pending", "uncertain"} {
		t.Run(state, func(t *testing.T) {
			initial := sessionTestCredentials("old", "123")
			if state == "legacy" {
				initial = credential.Credentials{SESSDATA: "legacy_session", BiliJCT: "legacy_csrf"}
			} else {
				initial.RefreshState = state
			}
			session, store, protocol := sessionTestFixture(t, initial)
			_, err := session.Load()
			require.ErrorIs(t, err, credential.ErrReauthenticationRequired)
			got, release, err := session.Acquire(context.Background())
			require.ErrorIs(t, err, credential.ErrReauthenticationRequired)
			require.Empty(t, got)
			require.Nil(t, release)
			status, err := session.Check(context.Background())
			require.NoError(t, err)
			require.Equal(t, Status{State: ReauthenticationRequired}, status)
			persisted, writes, clears := store.snapshot()
			require.Equal(t, initial, persisted)
			require.Empty(t, writes)
			require.Zero(t, clears)
			require.Zero(t, protocol.infoCalls.Load())
			require.Zero(t, protocol.refreshCalls.Load())
		})
	}
}

func TestSessionRefreshPersistsWholeGroupsAndConfirmsOldToken(t *testing.T) {
	old := sessionTestCredentials("old", "123")
	next := sessionTestCredentials("new", "123")
	session, store, protocol := sessionTestFixture(t, old)
	const timestamp = int64(1700000000123)
	protocol.refreshNeeded = func(_ context.Context, c credential.Credentials) (bool, int64, error) {
		require.Equal(t, old, c)
		return true, timestamp, nil
	}
	oldPending := old
	oldPending.RefreshState = "pending"
	newPending := next
	newPending.RefreshState = "pending"
	protocol.refresh = func(_ context.Context, c credential.Credentials, gotTimestamp int64) (credential.Credentials, error) {
		require.Equal(t, oldPending, c)
		require.Equal(t, timestamp, gotTimestamp)
		persisted, writes, _ := store.snapshot()
		require.Equal(t, oldPending, persisted)
		require.Equal(t, []credential.Credentials{oldPending}, writes)
		return next, nil
	}
	protocol.confirm = func(_ context.Context, c credential.Credentials, oldToken string) error {
		require.Equal(t, newPending, c)
		require.Equal(t, old.RefreshToken, oldToken)
		persisted, writes, _ := store.snapshot()
		require.Equal(t, newPending, persisted)
		require.Equal(t, []credential.Credentials{oldPending, newPending}, writes)
		return nil
	}
	got, release, err := session.Acquire(context.Background())
	require.NoError(t, err)
	require.Equal(t, next, got)
	release()
	persisted, writes, _ := store.snapshot()
	require.Equal(t, next, persisted)
	require.Equal(t, []credential.Credentials{oldPending, newPending, next}, writes)
	require.EqualValues(t, 1, protocol.refreshCalls.Load())
	require.EqualValues(t, 1, protocol.confirmCalls.Load())

	// A subsequent send loads the completed group without repeating renewal.
	got, release, err = session.Acquire(context.Background())
	require.NoError(t, err)
	require.Equal(t, next, got)
	release()
	require.EqualValues(t, 1, protocol.infoCalls.Load())
}

func TestSessionRefreshFailureAndRestartNeverReplayRotation(t *testing.T) {
	for _, failure := range []string{"refresh result uncertain", "write new group", "confirm", "write ready group", "changed uid"} {
		t.Run(failure, func(t *testing.T) {
			old := sessionTestCredentials("old", "123")
			session, store, protocol := sessionTestFixture(t, old)
			protocol.refreshNeeded = func(context.Context, credential.Credentials) (bool, int64, error) {
				return true, 1700000000123, nil
			}
			switch failure {
			case "refresh result uncertain":
				protocol.refresh = func(context.Context, credential.Credentials, int64) (credential.Credentials, error) {
					return credential.Credentials{}, ErrRefreshUncertain
				}
			case "write new group":
				store.failSave = 2
			case "confirm":
				protocol.confirm = func(context.Context, credential.Credentials, string) error { return ErrPassportUnavailable }
			case "write ready group":
				store.failSave = 3
			case "changed uid":
				protocol.refresh = func(context.Context, credential.Credentials, int64) (credential.Credentials, error) {
					return sessionTestCredentials("wrong_account", "456"), nil
				}
			}
			_, release, err := session.Acquire(context.Background())
			require.ErrorIs(t, err, credential.ErrReauthenticationRequired)
			require.Nil(t, release)
			persisted, _, _ := store.snapshot()
			require.Equal(t, "pending", persisted.RefreshState)
			require.Equal(t, "123", persisted.UID)
			if failure == "confirm" || failure == "write ready group" {
				require.Equal(t, "new_token", persisted.RefreshToken)
			} else {
				require.Equal(t, old.RefreshToken, persisted.RefreshToken)
			}
			for range 2 {
				_, release, err = session.Acquire(context.Background())
				require.ErrorIs(t, err, credential.ErrReauthenticationRequired)
				require.Nil(t, release)
			}
			require.EqualValues(t, 1, protocol.refreshCalls.Load())
			confirmCalls := protocol.confirmCalls.Load()
			session.Close()
			reopened := NewSession(store, protocol, session.checker, nil)
			defer reopened.Close()
			_, release, err = reopened.Acquire(context.Background())
			require.ErrorIs(t, err, credential.ErrReauthenticationRequired)
			require.Nil(t, release)
			require.EqualValues(t, 1, protocol.refreshCalls.Load())
			require.Equal(t, confirmCalls, protocol.confirmCalls.Load())
		})
	}
}

func TestSessionDoesNotStartRefreshWhenPendingMarkerCannotBeSaved(t *testing.T) {
	old := sessionTestCredentials("old", "123")
	session, store, protocol := sessionTestFixture(t, old)
	store.failSave = 1
	protocol.refreshNeeded = func(context.Context, credential.Credentials) (bool, int64, error) {
		return true, 1700000000123, nil
	}
	_, release, err := session.Acquire(context.Background())
	require.ErrorIs(t, err, credential.ErrUnavailable)
	require.Nil(t, release)
	persisted, _, _ := store.snapshot()
	require.Equal(t, old, persisted)
	got, release, err := session.Acquire(context.Background())
	require.ErrorIs(t, err, credential.ErrUnavailable)
	require.Empty(t, got)
	require.Nil(t, release)
	require.EqualValues(t, 1, protocol.infoCalls.Load())
	require.Zero(t, protocol.refreshCalls.Load())
	require.Zero(t, protocol.confirmCalls.Load())
}

func TestSessionRefreshRequiresVerifiedNewSessionBeforeConfirm(t *testing.T) {
	for _, failure := range []string{"different uid", "expired", "verification unavailable"} {
		t.Run(failure, func(t *testing.T) {
			session, store, protocol := sessionTestFixture(t, sessionTestCredentials("old", "123"))
			protocol.refreshNeeded = func(context.Context, credential.Credentials) (bool, int64, error) {
				return true, 1700000000123, nil
			}
			checks := 0
			session.checker = sessionTestChecker(func(_ context.Context, c credential.Credentials) (Status, error) {
				checks++
				require.Equal(t, "new_session", c.SESSDATA)
				require.Equal(t, "pending", c.RefreshState)
				switch failure {
				case "different uid":
					return Status{State: LoggedIn, UID: "456"}, nil
				case "expired":
					return Status{State: Expired}, nil
				default:
					return Status{State: VerificationFailed}, ErrUnavailable
				}
			})
			for range 2 {
				_, release, err := session.Acquire(context.Background())
				require.ErrorIs(t, err, credential.ErrReauthenticationRequired)
				require.Nil(t, release)
			}
			persisted, writes, _ := store.snapshot()
			expected := sessionTestCredentials("new", "123")
			expected.RefreshState = "pending"
			require.Equal(t, expected, persisted)
			require.Len(t, writes, 2)
			require.Equal(t, 1, checks)
			require.EqualValues(t, 1, protocol.refreshCalls.Load())
			require.Zero(t, protocol.confirmCalls.Load())
		})
	}
}

func TestSessionNetworkCheckFailureDoesNotExpireCredentials(t *testing.T) {
	for _, boundary := range []string{"cookie info", "nav"} {
		t.Run(boundary, func(t *testing.T) {
			old := sessionTestCredentials("old", "123")
			session, store, protocol := sessionTestFixture(t, old)
			if boundary == "cookie info" {
				protocol.refreshNeeded = func(context.Context, credential.Credentials) (bool, int64, error) {
					return false, 0, ErrPassportUnavailable
				}
			} else {
				session.checker = sessionTestChecker(func(context.Context, credential.Credentials) (Status, error) {
					return Status{State: VerificationFailed}, ErrUnavailable
				})
			}
			status, err := session.Check(context.Background())
			require.Error(t, err)
			require.Equal(t, Status{State: VerificationFailed}, status)
			persisted, writes, _ := store.snapshot()
			require.Equal(t, old, persisted)
			require.Empty(t, writes)
			require.Zero(t, protocol.refreshCalls.Load())
		})
	}
}

func TestSessionConfirmedExpiryBlocksSending(t *testing.T) {
	for _, boundary := range []string{"cookie info", "nav", "nav changed uid"} {
		t.Run(boundary, func(t *testing.T) {
			old := sessionTestCredentials("old", "123")
			session, store, protocol := sessionTestFixture(t, old)
			if boundary == "cookie info" {
				protocol.refreshNeeded = func(context.Context, credential.Credentials) (bool, int64, error) {
					return false, 0, ErrPassportExpired
				}
			} else {
				session.checker = sessionTestChecker(func(context.Context, credential.Credentials) (Status, error) {
					if boundary == "nav changed uid" {
						return Status{State: LoggedIn, UID: "456"}, nil
					}
					return Status{State: Expired}, nil
				})
			}
			status, err := session.Check(context.Background())
			require.NoError(t, err)
			require.Equal(t, Status{State: ReauthenticationRequired}, status)
			persisted, _, _ := store.snapshot()
			require.Equal(t, "uncertain", persisted.RefreshState)
			_, release, err := session.Acquire(context.Background())
			require.ErrorIs(t, err, credential.ErrReauthenticationRequired)
			require.Nil(t, release)
		})
	}
}

func TestSessionCannotForgetExpiryWhenUncertainMarkerSaveFails(t *testing.T) {
	old := sessionTestCredentials("old", "123")
	session, store, protocol := sessionTestFixture(t, old)
	store.failSave = 1
	session.checker = sessionTestChecker(func(context.Context, credential.Credentials) (Status, error) {
		return Status{State: Expired}, nil
	})
	status, err := session.Check(context.Background())
	require.NoError(t, err)
	require.Equal(t, Status{State: ReauthenticationRequired}, status)
	persisted, _, _ := store.snapshot()
	require.Equal(t, old, persisted)

	// Neither a subsequent forced check nor a cached loader can recover the
	// known-expired group just because its durable marker could not be written.
	status, err = session.Check(context.Background())
	require.NoError(t, err)
	require.Equal(t, Status{State: ReauthenticationRequired}, status)
	_, release, err := session.Acquire(context.Background())
	require.ErrorIs(t, err, credential.ErrReauthenticationRequired)
	require.Nil(t, release)
	_, err = session.Load()
	require.ErrorIs(t, err, credential.ErrReauthenticationRequired)
	require.EqualValues(t, 1, protocol.infoCalls.Load())
}

func TestSessionBackgroundMaintenanceStartsWithoutFrontend(t *testing.T) {
	session, _, protocol := sessionTestFixture(t, sessionTestCredentials("old", "123"))
	checked := make(chan struct{})
	protocol.refreshNeeded = func(context.Context, credential.Credentials) (bool, int64, error) {
		close(checked)
		return false, 1700000000123, nil
	}
	session.Start()
	session.Start()
	select {
	case <-checked:
	case <-time.After(time.Second):
		t.Fatal("background session maintenance did not check the account")
	}
	session.Close()
	require.EqualValues(t, 1, protocol.infoCalls.Load())
}

func TestSessionQRAccountReplacementRequiresExplicitConfirmation(t *testing.T) {
	for _, outcome := range []string{"confirm", "cancel"} {
		t.Run(outcome, func(t *testing.T) {
			old := sessionTestCredentials("old", "123")
			candidate := sessionTestCredentials("candidate", "456")
			session, store, protocol := sessionTestFixture(t, old)
			protocol.poll = func(context.Context, string) (QRResult, error) {
				return QRResult{State: "confirmed", Credentials: candidate}, nil
			}
			started, err := session.BeginQRLogin(context.Background())
			require.NoError(t, err)
			require.Equal(t, "waiting", started.State)
			require.Contains(t, started.QRImage, "data:image/png;base64,")
			status, err := session.PollQRLogin(context.Background())
			require.NoError(t, err)
			require.Equal(t, QRLoginStatus{State: "account_mismatch", CandidateUID: "456"}, status)
			persisted, writes, _ := store.snapshot()
			require.Equal(t, old, persisted)
			require.Empty(t, writes)
			status, err = session.PollQRLogin(context.Background())
			require.NoError(t, err)
			require.Equal(t, "account_mismatch", status.State)
			require.EqualValues(t, 1, protocol.pollCalls.Load())
			if outcome == "confirm" {
				require.NoError(t, session.ConfirmQRAccount(context.Background()))
				persisted, writes, _ = store.snapshot()
				require.Equal(t, candidate, persisted)
				require.Equal(t, []credential.Credentials{candidate}, writes)
			} else {
				require.NoError(t, session.CancelQRLogin(context.Background()))
				persisted, writes, _ = store.snapshot()
				require.Equal(t, old, persisted)
				require.Empty(t, writes)
				status, err = session.PollQRLogin(context.Background())
				require.NoError(t, err)
				require.Equal(t, "cancelled", status.State)
			}
		})
	}
}

func TestSessionQRFailureOrExpiryPreservesExistingAccount(t *testing.T) {
	for _, failure := range []string{"generate", "poll", "verify", "remote expiry", "local expiry", "save"} {
		t.Run(failure, func(t *testing.T) {
			old := sessionTestCredentials("old", "123")
			session, store, protocol := sessionTestFixture(t, old)
			now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
			session.now = func() time.Time { return now }
			protocol.poll = func(context.Context, string) (QRResult, error) {
				return QRResult{State: "confirmed", Credentials: sessionTestCredentials("candidate", "123")}, nil
			}
			switch failure {
			case "generate":
				protocol.generate = func(context.Context) (QRChallenge, error) { return QRChallenge{}, ErrPassportUnavailable }
			case "poll":
				protocol.poll = func(context.Context, string) (QRResult, error) { return QRResult{}, ErrPassportUnavailable }
			case "verify":
				session.checker = sessionTestChecker(func(context.Context, credential.Credentials) (Status, error) {
					return Status{State: VerificationFailed}, ErrUnavailable
				})
			case "remote expiry":
				protocol.poll = func(context.Context, string) (QRResult, error) { return QRResult{State: "expired"}, nil }
			case "save":
				store.failSave = 1
			}
			_, err := session.BeginQRLogin(context.Background())
			if failure == "generate" {
				require.ErrorIs(t, err, ErrPassportUnavailable)
			} else {
				require.NoError(t, err)
				if failure == "local expiry" {
					now = now.Add(180 * time.Second)
				}
				status, err := session.PollQRLogin(context.Background())
				if failure == "remote expiry" || failure == "local expiry" {
					require.NoError(t, err)
					require.Equal(t, "expired", status.State)
				} else {
					require.Error(t, err)
				}
				if failure == "save" {
					status, err = session.PollQRLogin(context.Background())
					require.NoError(t, err)
					require.Equal(t, "cancelled", status.State)
				}
			}
			persisted, _, clears := store.snapshot()
			require.Equal(t, old, persisted)
			require.Zero(t, clears)
		})
	}
}

func TestSessionQRMigrationVerifiesLegacyUIDBeforeReplacement(t *testing.T) {
	legacy := credential.Credentials{SESSDATA: "legacy_session", BiliJCT: "legacy_csrf"}
	session, store, protocol := sessionTestFixture(t, legacy)
	protocol.poll = func(context.Context, string) (QRResult, error) {
		return QRResult{State: "confirmed", Credentials: sessionTestCredentials("candidate", "456")}, nil
	}
	session.checker = sessionTestChecker(func(_ context.Context, c credential.Credentials) (Status, error) {
		if c.Version == 0 {
			return Status{State: LoggedIn, UID: "123"}, nil
		}
		return Status{State: LoggedIn, UID: c.UID}, nil
	})
	_, err := session.BeginQRLogin(context.Background())
	require.NoError(t, err)
	status, err := session.PollQRLogin(context.Background())
	require.NoError(t, err)
	require.Equal(t, QRLoginStatus{State: "account_mismatch", CandidateUID: "456"}, status)
	persisted, writes, _ := store.snapshot()
	require.Equal(t, legacy, persisted)
	require.Empty(t, writes)
}

func TestSessionSendLeaseBlocksRotationAndClear(t *testing.T) {
	for _, operation := range []string{"rotation", "clear"} {
		t.Run(operation, func(t *testing.T) {
			session, store, protocol := sessionTestFixture(t, sessionTestCredentials("old", "123"))
			_, release, err := session.Acquire(context.Background())
			require.NoError(t, err)
			defer release()
			if operation == "rotation" {
				protocol.refreshNeeded = func(context.Context, credential.Credentials) (bool, int64, error) {
					return true, 1700000000123, nil
				}
			}
			started := make(chan struct{})
			finished := make(chan error, 1)
			go func() {
				close(started)
				if operation == "clear" {
					finished <- session.Clear()
					return
				}
				_, err := session.Check(context.Background())
				finished <- err
			}()
			<-started
			select {
			case err := <-finished:
				t.Fatalf("%s completed while the send still held its lease: %v", operation, err)
			case <-time.After(20 * time.Millisecond):
			}
			_, writes, clears := store.snapshot()
			require.Empty(t, writes)
			require.Zero(t, clears)
			require.Zero(t, protocol.refreshCalls.Load())
			release()
			select {
			case err := <-finished:
				require.NoError(t, err)
			case <-time.After(time.Second):
				t.Fatal("operation did not resume after the send lease was released")
			}
		})
	}
}

func TestSessionCancelledSendDoesNotWaitForAnotherSendLease(t *testing.T) {
	session, _, protocol := sessionTestFixture(t, sessionTestCredentials("old", "123"))
	_, release, err := session.Acquire(context.Background())
	require.NoError(t, err)
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		close(started)
		_, secondRelease, err := session.Acquire(ctx)
		if secondRelease != nil {
			secondRelease()
		}
		finished <- err
	}()
	<-started
	cancel()
	select {
	case err := <-finished:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("cancelled send remained blocked on the account gate")
	}
	require.EqualValues(t, 1, protocol.infoCalls.Load())
}

func TestSessionCloseCancelsInFlightRefreshWithoutReplay(t *testing.T) {
	session, store, protocol := sessionTestFixture(t, sessionTestCredentials("old", "123"))
	protocol.refreshNeeded = func(context.Context, credential.Credentials) (bool, int64, error) {
		return true, 1700000000123, nil
	}
	refreshing := make(chan struct{})
	protocol.refresh = func(ctx context.Context, _ credential.Credentials, _ int64) (credential.Credentials, error) {
		close(refreshing)
		<-ctx.Done()
		return credential.Credentials{}, errors.Join(ErrRefreshUncertain, ctx.Err())
	}
	finished := make(chan error, 1)
	go func() {
		_, release, err := session.Acquire(context.Background())
		if release != nil {
			release()
		}
		finished <- err
	}()
	select {
	case <-refreshing:
	case <-time.After(time.Second):
		t.Fatal("refresh did not begin")
	}
	closed := make(chan struct{})
	go func() { session.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("closing the session did not cancel the in-flight refresh")
	}
	require.ErrorIs(t, <-finished, credential.ErrReauthenticationRequired)
	persisted, _, _ := store.snapshot()
	require.Equal(t, "pending", persisted.RefreshState)
	require.EqualValues(t, 1, protocol.refreshCalls.Load())
	require.Zero(t, protocol.confirmCalls.Load())
	_, release, err := session.Acquire(context.Background())
	require.ErrorIs(t, err, ErrSessionClosed)
	require.Nil(t, release)

	reopened := NewSession(store, protocol, session.checker, nil)
	defer reopened.Close()
	_, release, err = reopened.Acquire(context.Background())
	require.ErrorIs(t, err, credential.ErrReauthenticationRequired)
	require.Nil(t, release)
	require.EqualValues(t, 1, protocol.refreshCalls.Load())
}
