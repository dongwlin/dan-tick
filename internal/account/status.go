// Package account checks the configured Bilibili account's login state.
// All credential access and authenticated HTTP requests stay in Go.
package account

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/dongwlin/dan-tick/internal/credential"
)

const (
	navURL          = "https://api.bilibili.com/x/web-interface/nav"
	requestTimeout  = 10 * time.Second
	maxResponseSize = 1 << 20
)

var (
	ErrUnavailable     = errors.New("login status service unavailable")
	ErrInvalidResponse = errors.New("invalid login status response")
)

// State is safe to expose in a future Wails binding. Checking is a transient
// UI state; Check returns only a completed result.
type State string

const (
	Unconfigured       State = "unconfigured"
	LoggedIn           State = "logged_in"
	Expired            State = "expired"
	VerificationFailed State = "verification_failed"
)

// Status intentionally excludes credentials, upstream messages and raw data.
type Status struct {
	State State `json:"state"`
}

// credentialLoader isolates the real OS keyring in tests.
type credentialLoader interface {
	Load() (credential.Credentials, error)
}

type Service struct {
	store      credentialLoader
	httpClient *http.Client
	endpoint   string
}

func NewService(store credentialLoader, httpClient *http.Client) *Service {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: requestTimeout}
	}
	// The request carries a session cookie. Never forward it to a redirect
	// target, even if the caller supplied a redirect-following HTTP client.
	safeClient := *httpClient
	// The login check has exactly one credential source: the OS keyring.
	// Ignore any unrelated cookies from an injected client's jar.
	safeClient.Jar = nil
	safeClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &Service{store: store, httpClient: &safeClient, endpoint: navURL}
}

// Check loads the stored credentials and checks Bilibili's nav endpoint.
// A missing credential is distinct from an expired login or a failed check.
// No result or error contains Cookie values or untrusted upstream text.
func (s *Service) Check(ctx context.Context) (Status, error) {
	credentials, err := s.store.Load()
	if errors.Is(err, credential.ErrNotConfigured) {
		return Status{State: Unconfigured}, nil
	}
	if errors.Is(err, credential.ErrCorrupt) {
		return Status{State: VerificationFailed}, credential.ErrCorrupt
	}
	if err != nil {
		return Status{State: VerificationFailed}, credential.ErrUnavailable
	}

	failed := Status{State: VerificationFailed}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.endpoint, nil)
	if err != nil {
		return failed, ErrInvalidResponse
	}
	req.Header.Set("Accept", "application/json")
	req.AddCookie(&http.Cookie{Name: "SESSDATA", Value: credentials.SESSDATA})

	resp, err := s.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return failed, errors.Join(ErrUnavailable, ctx.Err())
		}
		return failed, ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return failed, ErrUnavailable
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil || len(body) > maxResponseSize {
		return failed, ErrInvalidResponse
	}
	var response struct {
		Code *int `json:"code"`
		Data *struct {
			IsLogin *bool `json:"isLogin"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &response) != nil || response.Code == nil {
		return failed, ErrInvalidResponse
	}
	if *response.Code == -101 {
		return Status{State: Expired}, nil
	}
	if *response.Code != 0 {
		return failed, ErrUnavailable
	}
	if response.Data == nil || response.Data.IsLogin == nil {
		return failed, ErrInvalidResponse
	}
	if !*response.Data.IsLogin {
		return Status{State: Expired}, nil
	}
	return Status{State: LoggedIn}, nil
}
