package account

import (
	"context"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"github.com/dongwlin/dan-tick/internal/credential"
	qrcode "github.com/skip2/go-qrcode"
)

var ErrSessionClosed = errors.New("account session closed")

type sessionStore interface {
	Load() (credential.Credentials, error)
	Save(credential.Credentials) error
	Clear() error
}

type passport interface {
	Generate(context.Context) (QRChallenge, error)
	Poll(context.Context, string) (QRResult, error)
	RefreshNeeded(context.Context, credential.Credentials) (bool, int64, error)
	Refresh(context.Context, credential.Credentials, int64) (credential.Credentials, error)
	Confirm(context.Context, credential.Credentials, string) error
}

type candidateChecker interface {
	CheckCredentials(context.Context, credential.Credentials) (Status, error)
}

// QRLoginStatus exposes only a temporary image and safe workflow state. The
// passport key, ticket, Cookie and refresh token never cross a Wails binding.
type QRLoginStatus struct {
	State        string `json:"state"`
	QRImage      string `json:"qrImage,omitempty"`
	ExpiresAt    int64  `json:"expiresAt,omitempty"`
	CandidateUID string `json:"candidateUID,omitempty"`
}

type qrLogin struct {
	key       string
	expires   time.Time
	nextPoll  time.Time
	state     string
	ctx       context.Context
	candidate *credential.Credentials
}

// Session owns the single QR session and serializes rotation, replacement,
// clearing and authenticated sends. It persists a pending marker BEFORE the
// refresh side effect, so a crash cannot silently reuse an ambiguous token.
type Session struct {
	store    sessionStore
	passport passport
	checker  candidateChecker
	changed  func()
	gate     chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
	start    sync.Once
	done     chan struct{}
	now      func() time.Time
	qr       *qrLogin
	qrMu     sync.Mutex // cancellation must interrupt an HTTP call holding gate
	qrCancel context.CancelFunc
	checked  time.Time
	checkErr error
	blocked  bool
}

func NewSession(store sessionStore, protocol passport, checker candidateChecker, changed func()) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	return &Session{store: store, passport: protocol, checker: checker, changed: changed,
		gate: make(chan struct{}, 1), ctx: ctx, cancel: cancel, done: make(chan struct{}), now: time.Now}
}

func (s *Session) lock(ctx context.Context) (context.Context, func(), error) {
	select {
	case s.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-s.ctx.Done():
		return nil, nil, ErrSessionClosed
	}
	work, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.ctx, cancel)
	if s.ctx.Err() != nil || ctx.Err() != nil {
		stop()
		cancel()
		<-s.gate
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		return nil, nil, ErrSessionClosed
	}
	var once sync.Once
	return work, func() { once.Do(func() { stop(); cancel(); <-s.gate }) }, nil
}

func (s *Session) notify() {
	if s.changed != nil {
		s.changed()
	}
}

// SetOnChange is wired by the composition root before starting any workers.
func (s *Session) SetOnChange(changed func()) { s.changed = changed }

// Start checks renewal independently of any mounted frontend, including while
// hidden. Send admission also checks renewal, covering sleeps and delayed ticks.
func (s *Session) Start() {
	s.start.Do(func() {
		go func() {
			defer close(s.done)
			ticker := time.NewTicker(time.Hour)
			defer ticker.Stop()
			for {
				_, release, err := s.Acquire(s.ctx)
				if err == nil {
					release()
				}
				select {
				case <-s.ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	})
}

// Stop cancels authentication HTTP immediately; Close subsequently joins it.
func (s *Session) Stop() {
	s.cancel()
	s.cancelQRRequest()
}

func (s *Session) Close() {
	s.Stop()
	s.start.Do(func() { close(s.done) })
	<-s.done
	// Join an in-flight UI/refresh operation before discarding its candidate.
	s.gate <- struct{}{}
	s.qr = nil
	<-s.gate
}

// Load supports existing loader boundaries but never admits a legacy or
// unfinished rotation. Send clients use Acquire to hold the gate through HTTP.
func (s *Session) Load() (credential.Credentials, error) {
	_, release, err := s.lock(context.Background())
	if err != nil {
		return credential.Credentials{}, err
	}
	defer release()
	if s.blocked {
		return credential.Credentials{}, credential.ErrReauthenticationRequired
	}
	c, err := s.store.Load()
	if err == nil {
		err = ready(c)
	}
	return c, err
}

func ready(c credential.Credentials) error {
	if c.Version != 1 || c.Source != "qr" || c.UID == "" || c.RefreshToken == "" || c.RefreshState != "ready" {
		return credential.ErrReauthenticationRequired
	}
	return nil
}

func (s *Session) Acquire(ctx context.Context) (credential.Credentials, func(), error) {
	_, c, release, err := s.AcquireRequest(ctx)
	return c, release, err
}

// AcquireRequest ties the admitted HTTP request to both caller cancellation
// and application shutdown, including test sends outside the task executor.
func (s *Session) AcquireRequest(ctx context.Context) (context.Context, credential.Credentials, func(), error) {
	work, release, err := s.lock(ctx)
	if err != nil {
		return nil, credential.Credentials{}, nil, err
	}
	c, err := s.maintain(work, false)
	if err != nil {
		release()
		return nil, credential.Credentials{}, nil, err
	}
	return work, c, release, nil
}

func (s *Session) maintain(ctx context.Context, force bool) (credential.Credentials, error) {
	c, err := s.store.Load()
	if err != nil {
		return credential.Credentials{}, err
	}
	if s.blocked {
		return credential.Credentials{}, credential.ErrReauthenticationRequired
	}
	if err = ready(c); err != nil {
		return credential.Credentials{}, err
	}
	interval := 6 * time.Hour
	if s.checkErr != nil {
		interval = 5 * time.Minute
	}
	if !force && !s.checked.IsZero() && s.now().Sub(s.checked) < interval {
		return c, s.checkErr
	}
	needed, timestamp, err := s.passport.RefreshNeeded(ctx, c)
	s.checked, s.checkErr = s.now(), err
	if err != nil {
		if errors.Is(err, ErrPassportExpired) {
			return credential.Credentials{}, s.requireLogin(c)
		}
		return credential.Credentials{}, err
	}
	if !needed {
		return c, nil
	}
	c.RefreshState = "pending"
	if err := s.store.Save(c); err != nil {
		s.checkErr = err
		return credential.Credentials{}, err
	}
	s.blocked = true
	// From this point onward, failures (including cancellation) are ambiguous.
	// Keep the durable marker, never automatically repeat refresh or confirm.
	s.notify()
	next, err := s.passport.Refresh(ctx, c, timestamp)
	if err != nil || next.UID != c.UID || ready(next) != nil {
		return credential.Credentials{}, credential.ErrReauthenticationRequired
	}
	next.RefreshState = "pending"
	if err := s.store.Save(next); err != nil {
		return credential.Credentials{}, credential.ErrReauthenticationRequired
	}
	status, err := s.checker.CheckCredentials(ctx, next)
	if err != nil || status.State != LoggedIn || status.UID != c.UID {
		return credential.Credentials{}, credential.ErrReauthenticationRequired
	}
	if err := s.passport.Confirm(ctx, next, c.RefreshToken); err != nil {
		return credential.Credentials{}, credential.ErrReauthenticationRequired
	}
	next.RefreshState = "ready"
	if err := s.store.Save(next); err != nil {
		return credential.Credentials{}, credential.ErrReauthenticationRequired
	}
	s.checkErr = nil
	s.blocked = false
	s.notify()
	return next, nil
}

func (s *Session) requireLogin(c credential.Credentials) error {
	c.RefreshState = "uncertain"
	// Even if the vault is inaccessible now, the cached failure blocks sends.
	s.checkErr = credential.ErrReauthenticationRequired
	s.blocked = true
	_ = s.store.Save(c)
	s.notify()
	return credential.ErrReauthenticationRequired
}

func (s *Session) Check(ctx context.Context) (Status, error) {
	work, release, err := s.lock(ctx)
	if err != nil {
		return Status{State: VerificationFailed}, err
	}
	defer release()
	c, err := s.maintain(work, true)
	switch {
	case errors.Is(err, credential.ErrNotConfigured):
		return Status{State: Unconfigured}, nil
	case errors.Is(err, credential.ErrReauthenticationRequired):
		return Status{State: ReauthenticationRequired}, nil
	case err != nil:
		return Status{State: VerificationFailed}, err
	}
	status, err := s.checker.CheckCredentials(work, c)
	if err != nil {
		return Status{State: VerificationFailed}, err
	}
	if status.State == Expired || (status.State == LoggedIn && status.UID != c.UID) {
		_ = s.requireLogin(c)
		return Status{State: ReauthenticationRequired}, nil
	}
	return status, nil
}

func (s *Session) cancelQRRequest() {
	s.qrMu.Lock()
	defer s.qrMu.Unlock()
	if s.qrCancel != nil {
		s.qrCancel()
		s.qrCancel = nil
	}
}

func (s *Session) BeginQRLogin(ctx context.Context) (QRLoginStatus, error) {
	s.cancelQRRequest()
	work, release, err := s.lock(ctx)
	if err != nil {
		return QRLoginStatus{}, err
	}
	defer release()
	s.qr = nil
	qrCtx, cancel := context.WithCancel(s.ctx)
	s.qrMu.Lock()
	s.qrCancel = cancel
	s.qrMu.Unlock()
	stop := context.AfterFunc(work, cancel)
	challenge, err := s.passport.Generate(qrCtx)
	stop()
	if err != nil {
		cancel()
		return QRLoginStatus{}, err
	}
	if qrCtx.Err() != nil {
		return QRLoginStatus{}, context.Canceled
	}
	png, err := qrcode.Encode(challenge.URL, qrcode.Medium, 256)
	if err != nil {
		cancel()
		return QRLoginStatus{}, ErrPassportInvalidResponse
	}
	if qrCtx.Err() != nil {
		return QRLoginStatus{}, context.Canceled
	}
	expires := s.now().Add(180 * time.Second)
	s.qr = &qrLogin{key: challenge.Key, expires: expires, state: "waiting", ctx: qrCtx}
	return QRLoginStatus{State: "waiting", QRImage: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), ExpiresAt: expires.UnixMilli()}, nil
}

func (s *Session) PollQRLogin(ctx context.Context) (QRLoginStatus, error) {
	work, release, err := s.lock(ctx)
	if err != nil {
		return QRLoginStatus{}, err
	}
	defer release()
	q := s.qr
	if q == nil || q.ctx.Err() != nil {
		return QRLoginStatus{State: "cancelled"}, nil
	}
	if !s.now().Before(q.expires) {
		s.qr = nil
		s.cancelQRRequest()
		return QRLoginStatus{State: "expired"}, nil
	}
	if q.candidate != nil {
		return QRLoginStatus{State: "account_mismatch", CandidateUID: q.candidate.UID}, nil
	}
	if s.now().Before(q.nextPoll) {
		return QRLoginStatus{State: q.state}, nil
	}
	q.nextPoll = s.now().Add(2 * time.Second)
	pollCtx, cancel := context.WithCancel(work)
	stop := context.AfterFunc(q.ctx, cancel)
	defer func() { stop(); cancel() }()
	result, err := s.passport.Poll(pollCtx, q.key)
	if err != nil {
		return QRLoginStatus{}, err
	}
	if pollCtx.Err() != nil {
		return QRLoginStatus{State: "cancelled"}, nil
	}
	if result.State != "confirmed" {
		q.state = result.State
		if result.State == "expired" {
			s.qr = nil
			s.cancelQRRequest()
		}
		return QRLoginStatus{State: result.State}, nil
	}
	if ready(result.Credentials) != nil {
		s.qr = nil
		s.cancelQRRequest()
		return QRLoginStatus{}, ErrPassportInvalidResponse
	}
	status, err := s.checker.CheckCredentials(pollCtx, result.Credentials)
	if err != nil || status.State != LoggedIn || status.UID != result.Credentials.UID {
		s.qr = nil
		s.cancelQRRequest()
		return QRLoginStatus{}, ErrPassportInvalidResponse
	}
	old, err := s.store.Load()
	if err != nil && !errors.Is(err, credential.ErrNotConfigured) {
		s.qr = nil
		s.cancelQRRequest()
		return QRLoginStatus{}, err
	}
	uid := old.UID
	if err == nil && old.Version == 0 {
		legacy, checkErr := s.checker.CheckCredentials(pollCtx, old)
		if checkErr != nil {
			s.qr = nil
			s.cancelQRRequest()
			return QRLoginStatus{}, checkErr
		}
		if legacy.State == LoggedIn {
			uid = legacy.UID
		}
	}
	if uid != "" && uid != result.Credentials.UID {
		q.candidate = &result.Credentials
		return QRLoginStatus{State: "account_mismatch", CandidateUID: result.Credentials.UID}, nil
	}
	if err := s.saveQR(pollCtx, result.Credentials); err != nil {
		s.qr = nil
		s.cancelQRRequest()
		return QRLoginStatus{}, err
	}
	return QRLoginStatus{State: "confirmed"}, nil
}

func (s *Session) saveQR(ctx context.Context, c credential.Credentials) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.store.Save(c); err != nil {
		return err
	}
	s.checked, s.checkErr = time.Time{}, nil
	s.blocked = false
	s.qr = nil
	s.cancelQRRequest()
	s.notify()
	return nil
}

func (s *Session) ConfirmQRAccount(ctx context.Context) error {
	work, release, err := s.lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	if s.qr == nil || s.qr.candidate == nil || s.qr.ctx.Err() != nil || !s.now().Before(s.qr.expires) {
		return ErrPassportExpired
	}
	confirmCtx, cancel := context.WithCancel(work)
	stop := context.AfterFunc(s.qr.ctx, cancel)
	defer func() { stop(); cancel() }()
	return s.saveQR(confirmCtx, *s.qr.candidate)
}

func (s *Session) CancelQRLogin(ctx context.Context) error {
	s.cancelQRRequest()
	_, release, err := s.lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	s.qr = nil
	return nil
}

func (s *Session) Clear() error {
	s.cancelQRRequest()
	_, release, err := s.lock(context.Background())
	if err != nil {
		return err
	}
	defer release()
	s.qr = nil
	if err := s.store.Clear(); err != nil {
		return err
	}
	s.checked, s.checkErr = time.Time{}, nil
	s.blocked = false
	s.notify()
	return nil
}
