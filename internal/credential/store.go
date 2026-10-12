// Package credential stores the single Bilibili account's secrets in the OS
// keyring. It never persists them to the application's SQLite database.
package credential

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/zalando/go-keyring"
)

const (
	serviceName    = "dan-tick"
	accountName    = "bilibili"
	maxStoredBytes = 2560 // Windows Credential Manager's credential blob limit.
)

var (
	ErrInvalid       = errors.New("invalid account credentials")
	ErrNotConfigured = errors.New("account credentials not configured")
	ErrUnavailable   = errors.New("system credential storage unavailable or inaccessible")
	ErrCorrupt       = errors.New("stored account credentials are invalid")
	// ErrReauthenticationRequired means the saved session must be replaced by
	// explicit login before it can be used for authenticated operations.
	ErrReauthenticationRequired = errors.New("account requires reauthentication")
)

// Credentials contains secrets for authenticated Bilibili requests. Keep this
// value inside Go; never return it in a Wails response or diagnostic log.
type Credentials struct {
	SESSDATA     string `json:"sessdata"`
	BiliJCT      string `json:"bili_jct"`
	Version      int    `json:"version,omitempty"`
	Source       string `json:"source,omitempty"`
	UID          string `json:"uid,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	RefreshState string `json:"refresh_state,omitempty"`
}

// KeyringBackend is the credential vault boundary. Set must atomically replace
// the whole item, or leave the existing item unchanged when it returns an error.
// The interface allows authentication integration tests to avoid the OS vault.
type KeyringBackend interface {
	Set(service, account, secret string) error
	Get(service, account string) (string, error)
	Delete(service, account string) error
}

type systemKeyring struct{}

func (systemKeyring) Set(service, account, secret string) error {
	return keyring.Set(service, account, secret)
}

func (systemKeyring) Get(service, account string) (string, error) {
	return keyring.Get(service, account)
}

func (systemKeyring) Delete(service, account string) error {
	return keyring.Delete(service, account)
}

type Store struct {
	mu      sync.Mutex
	backend KeyringBackend
}

// NewStore uses the platform's credential vault (Windows Credential Manager
// or Linux Secret Service). No plaintext fallback is available.
func NewStore() *Store {
	return NewStoreWithBackend(systemKeyring{})
}

// NewStoreWithBackend injects a credential vault. Production callers should
// use NewStore rather than introducing a plaintext credential backend.
func NewStoreWithBackend(backend KeyringBackend) *Store {
	return &Store{backend: backend}
}

// Save atomically replaces the whole credential group in a single keyring
// item. Legacy two-Cookie values remain recognizable until explicit migration;
// a QR session includes its source, UID, rotating token and refresh state.
func (s *Store) Save(credentials Credentials) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	credentials.SESSDATA = strings.TrimSpace(credentials.SESSDATA)
	credentials.BiliJCT = strings.TrimSpace(credentials.BiliJCT)
	if !valid(credentials) {
		return ErrInvalid
	}
	secret, err := json.Marshal(credentials)
	if err != nil {
		return ErrInvalid
	}
	if len(secret) > maxStoredBytes {
		return ErrInvalid
	}
	if err := s.backend.Set(serviceName, accountName, string(secret)); err != nil {
		if errors.Is(err, keyring.ErrSetDataTooBig) {
			return ErrInvalid
		}
		return unavailable("save")
	}
	return nil
}

// Load is for backend authentication clients only. It preserves legacy and
// interrupted-refresh groups for recovery; the session coordinator decides
// whether those values may be used. Missing credentials are distinguished from
// an inaccessible system keyring.
func (s *Store) Load() (Credentials, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	secret, err := s.backend.Get(serviceName, accountName)
	if errors.Is(err, keyring.ErrNotFound) {
		return Credentials{}, ErrNotConfigured
	}
	if err != nil {
		return Credentials{}, unavailable("read")
	}
	if len(secret) > maxStoredBytes {
		return Credentials{}, ErrCorrupt
	}
	var credentials Credentials
	decoder := json.NewDecoder(strings.NewReader(secret))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&credentials) != nil || !valid(credentials) {
		return Credentials{}, ErrCorrupt
	}
	if decoder.Decode(new(any)) != io.EOF {
		return Credentials{}, ErrCorrupt
	}
	return credentials, nil
}

// Clear removes the account's keyring item. Clearing an already-empty store
// is successful, but an inaccessible keyring must not be mistaken for empty.
func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := s.backend.Delete(serviceName, accountName)
	if err == nil || errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return unavailable("clear")
}

func valid(c Credentials) bool {
	if !validCookieValue(c.SESSDATA) || !validCookieValue(c.BiliJCT) {
		return false
	}
	if c.Version == 0 {
		// The pre-QR format has exactly two Cookies. Do not accidentally treat a
		// partial or unsupported new format as a usable legacy credential pair.
		return c.Source == "" && c.UID == "" && c.RefreshToken == "" && c.RefreshState == ""
	}
	if c.Version != 1 || c.Source != "qr" || !validUID(c.UID) || !validCookieValue(c.RefreshToken) {
		return false
	}
	switch c.RefreshState {
	case "ready", "pending", "uncertain":
		return true
	default:
		return false
	}
}

func validUID(uid string) bool {
	if uid == "" || uid[0] == '0' {
		return false
	}
	for _, r := range uid {
		if r < '0' || r > '9' {
			return false
		}
	}
	_, err := strconv.ParseUint(uid, 10, 64)
	return err == nil
}

func validCookieValue(value string) bool {
	if value == "" || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if r == ';' || r == 0x7f || r <= 0x20 {
			return false
		}
	}
	return (&http.Cookie{Name: "credential", Value: value}).Valid() == nil
}

// Never wrap the underlying OS/DBus error: some providers can include user
// supplied values in error text, which must not reach logs or a Wails binding.
func unavailable(operation string) error {
	return fmt.Errorf("%s account credentials: %w", operation, ErrUnavailable)
}
