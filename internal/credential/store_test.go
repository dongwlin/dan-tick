package credential

import (
	"errors"
	"fmt"
	"strings"
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

func TestLoadRejectsCorruptedKeyringValue(t *testing.T) {
	for name, saved := range map[string]string{
		"empty":         "",
		"not json":      "a-secret",
		"missing field": `{"sessdata":"session"}`,
		"invalid value": `{"sessdata":"session;bad","bili_jct":"csrf"}`,
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
