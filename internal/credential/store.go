// Package credential stores the single Bilibili account's secrets in the OS
// keyring. It never persists them to the application's SQLite database.
package credential

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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
)

// Credentials contains secrets for authenticated Bilibili requests. Keep this
// value inside Go; never return it in a Wails response or diagnostic log.
type Credentials struct {
	SESSDATA string `json:"sessdata"`
	BiliJCT  string `json:"bili_jct"`
}

// keyringBackend is the external side-effect boundary, replaceable in tests.
type keyringBackend interface {
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
	backend keyringBackend
}

// NewStore uses the platform's credential vault (Windows Credential Manager
// or Linux Secret Service). No plaintext fallback is available.
func NewStore() *Store {
	return &Store{backend: systemKeyring{}}
}

// Save creates or replaces the account credentials as a single keyring item,
// avoiding a mixed old/new pair when updating both Cookie values.
func (s *Store) Save(credentials Credentials) error {
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

// Load is for backend authentication clients only. Missing credentials are
// distinguished from an inaccessible system keyring.
func (s *Store) Load() (Credentials, error) {
	secret, err := s.backend.Get(serviceName, accountName)
	if errors.Is(err, keyring.ErrNotFound) {
		return Credentials{}, ErrNotConfigured
	}
	if err != nil {
		return Credentials{}, unavailable("read")
	}
	var credentials Credentials
	if json.Unmarshal([]byte(secret), &credentials) != nil || !valid(credentials) {
		return Credentials{}, ErrCorrupt
	}
	return credentials, nil
}

// Clear removes the account's keyring item. Clearing an already-empty store
// is successful, but an inaccessible keyring must not be mistaken for empty.
func (s *Store) Clear() error {
	err := s.backend.Delete(serviceName, accountName)
	if err == nil || errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return unavailable("clear")
}

func valid(c Credentials) bool {
	return validCookieValue(c.SESSDATA) && validCookieValue(c.BiliJCT)
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
	return true
}

// Never wrap the underlying OS/DBus error: some providers can include user
// supplied values in error text, which must not reach logs or a Wails binding.
func unavailable(operation string) error {
	return fmt.Errorf("%s account credentials: %w", operation, ErrUnavailable)
}
