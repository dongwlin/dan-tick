// Package account checks the configured Bilibili account's login state.
// All credential access and authenticated HTTP requests stay in Go.
package account

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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
	Unconfigured             State = "unconfigured"
	LoggedIn                 State = "logged_in"
	Expired                  State = "expired"
	VerificationFailed       State = "verification_failed"
	ReauthenticationRequired State = "reauthentication_required"
)

// Status exposes only the login state and safe public profile fields.
// Credentials and untrusted upstream messages never leave the Go backend.
type Status struct {
	State     State  `json:"state"`
	UID       string `json:"uid,omitempty"`
	Name      string `json:"name,omitempty"`
	AvatarURL string `json:"avatarUrl,omitempty"`
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

	return s.CheckCredentials(ctx, credentials)
}

// CheckCredentials verifies a candidate without first replacing the keyring.
// It is used by the QR session owner for UID binding and refresh validation.
func (s *Service) CheckCredentials(ctx context.Context, credentials credential.Credentials) (Status, error) {
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
			IsLogin *bool  `json:"isLogin"`
			MID     int64  `json:"mid"`
			Uname   string `json:"uname"`
			Face    string `json:"face"`
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
	status := Status{State: LoggedIn}
	if response.Data.MID > 0 {
		status.UID = strconv.FormatInt(response.Data.MID, 10)
	}
	status.Name = strings.TrimSpace(response.Data.Uname)
	status.AvatarURL = safeAvatarURL(response.Data.Face)
	for _, secret := range []string{credentials.SESSDATA, credentials.BiliJCT, credentials.RefreshToken} {
		if secret == "" {
			continue
		}
		if strings.Contains(status.Name, secret) {
			status.Name = ""
		}
		if strings.Contains(status.AvatarURL, secret) {
			status.AvatarURL = ""
		}
	}
	return status, nil
}

// Only pass through Bilibili CDN profile images. Never expose arbitrary URLs
// from an upstream response to the WebView or retain their query parameters.
func safeAvatarURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Port() != "" ||
		!strings.HasSuffix(strings.ToLower(u.Hostname()), ".hdslb.com") ||
		!strings.HasPrefix(u.EscapedPath(), "/bfs/face/") {
		return ""
	}
	u.Scheme = "https"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
