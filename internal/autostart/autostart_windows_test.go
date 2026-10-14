//go:build windows

package autostart

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows/registry"
)

type fakeRunStore struct {
	command string
	present bool
	err     error
}

func (s *fakeRunStore) read() (string, error) {
	if s.err != nil {
		return "", s.err
	}
	if !s.present {
		return "", registry.ErrNotExist
	}
	return s.command, nil
}

func (s *fakeRunStore) write(command string) error {
	if s.err == nil {
		s.command, s.present = command, true
	}
	return s.err
}

func (s *fakeRunStore) remove() error {
	if s.err == nil {
		s.command, s.present = "", false
	}
	return s.err
}

func TestWindowsRegistrationUsesStableEntryAndQuotedCommand(t *testing.T) {
	store := &fakeRunStore{}
	r := &windowsRegistration{store: store}
	path := `C:\Users\User\Dan Tick\dan-tick.exe`
	status, err := r.status(path)
	require.NoError(t, err)
	require.False(t, status.Enabled)
	require.NoError(t, r.enable(path))
	require.Equal(t, `"C:\Users\User\Dan Tick\dan-tick.exe"`, store.command)
	status, err = r.status(strings.ToUpper(path))
	require.NoError(t, err)
	require.True(t, status.Enabled)
	status, err = r.status(`C:\new\dan-tick.exe`)
	require.NoError(t, err)
	require.False(t, status.Enabled)
	require.True(t, status.NeedsRepair)
	require.NoError(t, r.disable())
	require.False(t, store.present)
	require.NoError(t, r.disable())
}

func TestWindowsRegistrationReflectsExternalChangesAndErrors(t *testing.T) {
	store := &fakeRunStore{command: `"C:\old\dan-tick.exe" --extra`, present: true}
	r := &windowsRegistration{store: store}
	status, err := r.status(`C:\current\dan-tick.exe`)
	require.NoError(t, err)
	require.True(t, status.NeedsRepair)
	require.NoError(t, r.enable(`C:\current\dan-tick.exe`))
	store.present = false
	status, err = r.status(`C:\current\dan-tick.exe`)
	require.NoError(t, err)
	require.False(t, status.Enabled)
	store.err = errors.New("registry denied")
	_, err = r.status(`C:\current\dan-tick.exe`)
	require.Error(t, err)
	require.Error(t, r.enable(`C:\current\dan-tick.exe`))
	require.Error(t, r.disable())
}

func TestWindowsCommandRejectsUnusablePaths(t *testing.T) {
	for _, target := range []string{"C:\\bad\"name.exe", "C:\\bad\x00name.exe", "C:\\bad\nname.exe", `C:\` + strings.Repeat("a", 257)} {
		_, err := windowsCommand(target)
		require.Error(t, err)
	}
	_, err := windowsCommand(`C:\` + strings.Repeat("a", 255))
	require.NoError(t, err) // 258 path characters + two quotes
}
