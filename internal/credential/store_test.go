package credential

import (
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

type fakeKeyring struct {
	secret  string
	present bool
	setErr  error
	getErr  error
	delErr  error
	writes  int
	service string
	account string
}

func (f *fakeKeyring) Set(service, account, secret string) error {
	f.service, f.account = service, account
	if f.setErr != nil {
		return f.setErr
	}
	f.secret, f.present = secret, true
	f.writes++
	return nil
}

func (f *fakeKeyring) Get(service, account string) (string, error) {
	f.service, f.account = service, account
	if f.getErr != nil {
		return "", f.getErr
	}
	if !f.present {
		return "", keyring.ErrNotFound
	}
	return f.secret, nil
}

func (f *fakeKeyring) Delete(service, account string) error {
	f.service, f.account = service, account
	if f.delErr != nil {
		return f.delErr
	}
	if !f.present {
		return keyring.ErrNotFound
	}
	f.secret, f.present = "", false
	return nil
}

func TestSaveLoadUpdateAndClear(t *testing.T) {
	backend := &fakeKeyring{}
	store := &Store{backend: backend}
	first := Credentials{SESSDATA: "original_session", BiliJCT: "original_csrf"}
	updated := Credentials{SESSDATA: "updated_session", BiliJCT: "updated_csrf"}

	_, err := store.Load()
	require.ErrorIs(t, err, ErrNotConfigured)
	require.NoError(t, store.Clear()) // An absent item is already cleared.

	require.NoError(t, store.Save(first))
	require.Equal(t, serviceName, backend.service)
	require.Equal(t, accountName, backend.account)
	// A new store instance can read the persisted pair, without an in-memory cache.
	reopened := &Store{backend: backend}
	got, err := reopened.Load()
	require.NoError(t, err)
	require.Equal(t, first, got)

	require.NoError(t, reopened.Save(updated))
	got, err = store.Load()
	require.NoError(t, err)
	require.Equal(t, updated, got)
	require.Equal(t, 2, backend.writes)
	require.NotContains(t, backend.secret, first.SESSDATA)
	require.NotContains(t, backend.secret, first.BiliJCT)

	require.NoError(t, store.Clear())
	require.NoError(t, store.Clear())
	_, err = reopened.Load()
	require.ErrorIs(t, err, ErrNotConfigured)
}

func TestSaveRejectsInvalidCredentialsWithoutChangingStoredPair(t *testing.T) {
	backend := &fakeKeyring{}
	store := &Store{backend: backend}
	good := Credentials{SESSDATA: "session", BiliJCT: "csrf"}
	require.NoError(t, store.Save(good))

	for name, invalid := range map[string]Credentials{
		"missing sessdata": {BiliJCT: "csrf"},
		"missing csrf":     {SESSDATA: "session"},
		"empty spaces":     {SESSDATA: "   ", BiliJCT: "csrf"},
		"cookie injection": {SESSDATA: "abc;admin=1", BiliJCT: "csrf"},
		"newline":          {SESSDATA: "abc\r\n", BiliJCT: "csrf\nx"},
		"control":          {SESSDATA: "abc", BiliJCT: "x\x00y"},
		"internal space":   {SESSDATA: "abc def", BiliJCT: "csrf"},
		"quoted session":   {SESSDATA: `abc"def`, BiliJCT: "csrf"},
		"session slash":    {SESSDATA: `abc\def`, BiliJCT: "csrf"},
		"unicode session":  {SESSDATA: "abc中文", BiliJCT: "csrf"},
		"quoted csrf":      {SESSDATA: "session", BiliJCT: `abc"def`},
		"csrf slash":       {SESSDATA: "session", BiliJCT: `abc\def`},
		"unicode csrf":     {SESSDATA: "session", BiliJCT: "abc中文"},
		"too large":        {SESSDATA: strings.Repeat("x", maxStoredBytes), BiliJCT: "csrf"},
	} {
		t.Run(name, func(t *testing.T) {
			require.ErrorIs(t, store.Save(invalid), ErrInvalid)
			got, err := store.Load()
			require.NoError(t, err)
			require.Equal(t, good, got)
			require.Equal(t, 1, backend.writes)
		})
	}
}

func TestSaveTrimsPastedWhitespace(t *testing.T) {
	store := &Store{backend: &fakeKeyring{}}
	require.NoError(t, store.Save(Credentials{SESSDATA: " session ", BiliJCT: "\tcsrf\n"}))
	got, err := store.Load()
	require.NoError(t, err)
	require.Equal(t, Credentials{SESSDATA: "session", BiliJCT: "csrf"}, got)
}

func TestSaveLoadPreservesURLEncodedSession(t *testing.T) {
	store := &Store{backend: &fakeKeyring{}}
	credentials := Credentials{SESSDATA: "fake%2Csession%2F%3B%22value", BiliJCT: "fake_csrf"}
	require.NoError(t, store.Save(credentials))
	got, err := store.Load()
	require.NoError(t, err)
	require.Equal(t, credentials, got)
}

func TestLoadRejectsCorruptedKeyringValue(t *testing.T) {
	for name, saved := range map[string]string{
		"empty":         "",
		"not json":      "a-secret",
		"missing field": `{"sessdata":"session"}`,
		"invalid value": `{"sessdata":"session;bad","bili_jct":"csrf"}`,
		"quoted value":  `{"sessdata":"session\"bad","bili_jct":"csrf"}`,
		"backslash":     `{"sessdata":"session\\bad","bili_jct":"csrf"}`,
		"unicode value": `{"sessdata":"session中文","bili_jct":"csrf"}`,
		"invalid csrf":  `{"sessdata":"session","bili_jct":"csrf\\bad"}`,
	} {
		t.Run(name, func(t *testing.T) {
			store := &Store{backend: &fakeKeyring{secret: saved, present: true}}
			got, err := store.Load()
			require.ErrorIs(t, err, ErrCorrupt)
			require.Equal(t, Credentials{}, got)
			if saved != "" {
				require.NotContains(t, err.Error(), saved)
			}
		})
	}
}

func TestKeyringFailuresAreSafeAndDistinguishable(t *testing.T) {
	secret := "secret_value_that_must_not_leak"
	failure := fmt.Errorf("backend diagnostic includes %s", secret)
	for name, err := range map[string]error{
		"locked or unavailable": failure,
		"unsupported platform":  keyring.ErrUnsupportedPlatform,
	} {
		t.Run(name, func(t *testing.T) {
			backend := &fakeKeyring{setErr: err, getErr: err, delErr: err}
			store := &Store{backend: backend}
			writeErr := store.Save(Credentials{SESSDATA: secret, BiliJCT: "csrf"})
			require.ErrorIs(t, writeErr, ErrUnavailable)
			_, readErr := store.Load()
			require.ErrorIs(t, readErr, ErrUnavailable)
			clearErr := store.Clear()
			require.ErrorIs(t, clearErr, ErrUnavailable)
			for _, operationErr := range []error{writeErr, readErr, clearErr} {
				require.NotContains(t, operationErr.Error(), secret)
				require.NotContains(t, operationErr.Error(), err.Error())
				require.NotErrorIs(t, operationErr, ErrNotConfigured)
			}
		})
	}

	backend := &fakeKeyring{setErr: keyring.ErrSetDataTooBig}
	require.ErrorIs(t, (&Store{backend: backend}).Save(Credentials{SESSDATA: "a", BiliJCT: "b"}), ErrInvalid)
	require.False(t, errors.Is(ErrInvalid, ErrUnavailable))
}

func qrCredentials() Credentials {
	return Credentials{
		SESSDATA: "qr_session", BiliJCT: "qr_csrf", Version: 1,
		Source: "qr", UID: "123456", RefreshToken: "qr_refresh_token", RefreshState: "ready",
	}
}

func TestLegacyCredentialsRemainStoredUntilExplicitQRMigration(t *testing.T) {
	legacySecret := `{"sessdata":"legacy_session","bili_jct":"legacy_csrf"}`
	backend := &fakeKeyring{secret: legacySecret, present: true}
	store := NewStoreWithBackend(backend)

	got, err := store.Load()
	require.NoError(t, err)
	require.Equal(t, Credentials{SESSDATA: "legacy_session", BiliJCT: "legacy_csrf"}, got)
	require.Equal(t, legacySecret, backend.secret)
	require.Zero(t, backend.writes)

	require.NoError(t, store.Save(qrCredentials()))
	got, err = NewStoreWithBackend(backend).Load()
	require.NoError(t, err)
	require.Equal(t, qrCredentials(), got)
	require.NotContains(t, backend.secret, "legacy_session")
	require.NotContains(t, backend.secret, "legacy_csrf")
}

func TestQRRefreshStoresWholeGroupsAndSurvivesReopening(t *testing.T) {
	backend := &fakeKeyring{}
	store := NewStoreWithBackend(backend)
	original := qrCredentials()
	require.NoError(t, store.Save(original))

	pending := original
	pending.RefreshState = "pending"
	require.NoError(t, store.Save(pending))
	got, err := NewStoreWithBackend(backend).Load()
	require.NoError(t, err)
	require.Equal(t, pending, got)

	rotated := original
	rotated.SESSDATA, rotated.BiliJCT, rotated.RefreshToken = "new_session", "new_csrf", "new_token"
	require.NoError(t, store.Save(rotated))
	got, err = NewStoreWithBackend(backend).Load()
	require.NoError(t, err)
	require.Equal(t, rotated, got)
	for _, oldSecret := range []string{original.SESSDATA, original.BiliJCT, original.RefreshToken} {
		require.NotContains(t, backend.secret, oldSecret)
	}

	uncertain := rotated
	uncertain.RefreshState = "uncertain"
	require.NoError(t, store.Save(uncertain))
	got, err = NewStoreWithBackend(backend).Load()
	require.NoError(t, err)
	require.Equal(t, uncertain, got)
}

func TestFailedQRRotationPreservesPersistedGroupAndHidesSecrets(t *testing.T) {
	backend := &fakeKeyring{}
	store := NewStoreWithBackend(backend)
	pending := qrCredentials()
	pending.RefreshState = "pending"
	require.NoError(t, store.Save(pending))
	previous := backend.secret

	rotated := qrCredentials()
	rotated.SESSDATA, rotated.BiliJCT, rotated.RefreshToken = "new_session", "new_csrf", "new_token"
	backend.setErr = fmt.Errorf("failed saving %s, %s and %s", rotated.SESSDATA, rotated.BiliJCT, rotated.RefreshToken)
	err := store.Save(rotated)
	require.ErrorIs(t, err, ErrUnavailable)
	for _, secret := range []string{rotated.SESSDATA, rotated.BiliJCT, rotated.RefreshToken} {
		require.NotContains(t, err.Error(), secret)
	}
	require.Equal(t, previous, backend.secret)
	got, err := NewStoreWithBackend(backend).Load()
	require.NoError(t, err)
	require.Equal(t, pending, got)
	require.Equal(t, 1, backend.writes)
}

func TestQRFormatRejectsPartialAndInvalidGroupsWithoutChangingSavedValue(t *testing.T) {
	for name, change := range map[string]func(*Credentials){
		"legacy with qr metadata": func(c *Credentials) { c.Version = 0 },
		"unsupported version":     func(c *Credentials) { c.Version = 2 },
		"missing source":          func(c *Credentials) { c.Source = "" },
		"unsupported source":      func(c *Credentials) { c.Source = "chrome" },
		"missing uid":             func(c *Credentials) { c.UID = "" },
		"zero uid":                func(c *Credentials) { c.UID = "0" },
		"ambiguous uid":           func(c *Credentials) { c.UID = "0123" },
		"negative uid":            func(c *Credentials) { c.UID = "-123" },
		"uid spaces":              func(c *Credentials) { c.UID = " 123" },
		"uid injection":           func(c *Credentials) { c.UID = "123;bad" },
		"uid overflow":            func(c *Credentials) { c.UID = "18446744073709551616" },
		"missing refresh token":   func(c *Credentials) { c.RefreshToken = "" },
		"refresh token injection": func(c *Credentials) { c.RefreshToken = "secret;other=1" },
		"refresh token spaces":    func(c *Credentials) { c.RefreshToken = " secret " },
		"refresh token control":   func(c *Credentials) { c.RefreshToken = "secret\x00value" },
		"missing refresh state":   func(c *Credentials) { c.RefreshState = "" },
		"invalid refresh state":   func(c *Credentials) { c.RefreshState = "unknown" },
		"oversized group":         func(c *Credentials) { c.RefreshToken = strings.Repeat("x", maxStoredBytes) },
	} {
		t.Run(name, func(t *testing.T) {
			backend := &fakeKeyring{}
			store := NewStoreWithBackend(backend)
			require.NoError(t, store.Save(qrCredentials()))
			previous := backend.secret
			invalid := qrCredentials()
			change(&invalid)
			require.ErrorIs(t, store.Save(invalid), ErrInvalid)
			require.Equal(t, previous, backend.secret)
			require.Equal(t, 1, backend.writes)

			saved, err := json.Marshal(invalid)
			require.NoError(t, err)
			backend.secret = string(saved)
			got, err := store.Load()
			require.ErrorIs(t, err, ErrCorrupt)
			require.Equal(t, Credentials{}, got)
			require.Equal(t, string(saved), backend.secret)
		})
	}
}

func TestLoadRejectsOversizedOrUnrecognizedJSONWithoutDeletingIt(t *testing.T) {
	for name, secret := range map[string]string{
		"oversized":      `{"sessdata":"` + strings.Repeat("x", maxStoredBytes) + `","bili_jct":"csrf"}`,
		"unknown fields": `{"sessdata":"session","bili_jct":"csrf","refreshToken":"token"}`,
		"extra object":   `{"sessdata":"session","bili_jct":"csrf"} {"version":1}`,
	} {
		t.Run(name, func(t *testing.T) {
			backend := &fakeKeyring{secret: secret, present: true}
			got, err := NewStoreWithBackend(backend).Load()
			require.ErrorIs(t, err, ErrCorrupt)
			require.Equal(t, Credentials{}, got)
			require.Equal(t, secret, backend.secret)
			require.True(t, backend.present)
			require.Zero(t, backend.writes)
		})
	}
}

type serializedKeyring struct {
	fakeKeyring
	active  atomic.Bool
	overlap atomic.Bool
}

func (f *serializedKeyring) enter() {
	if !f.active.CompareAndSwap(false, true) {
		f.overlap.Store(true)
	}
	runtime.Gosched()
}

func (f *serializedKeyring) Set(service, account, secret string) error {
	f.enter()
	defer f.active.Store(false)
	return f.fakeKeyring.Set(service, account, secret)
}

func (f *serializedKeyring) Get(service, account string) (string, error) {
	f.enter()
	defer f.active.Store(false)
	return f.fakeKeyring.Get(service, account)
}

func (f *serializedKeyring) Delete(service, account string) error {
	f.enter()
	defer f.active.Store(false)
	return f.fakeKeyring.Delete(service, account)
}

func TestStoreSerializesConcurrentSaveLoadAndClear(t *testing.T) {
	backend := &serializedKeyring{}
	store := NewStoreWithBackend(backend)
	var workers sync.WaitGroup
	results := make(chan error, 20)
	for range 20 {
		workers.Go(func() {
			if err := store.Save(qrCredentials()); err != nil {
				results <- err
				return
			}
			got, err := store.Load()
			if err == nil {
				if got != qrCredentials() {
					results <- errors.New("read returned a mixed credential group")
					return
				}
			} else if !errors.Is(err, ErrNotConfigured) {
				results <- err
				return
			}
			results <- store.Clear()
		})
	}
	workers.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	require.False(t, backend.overlap.Load())
}
