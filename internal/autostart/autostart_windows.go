//go:build windows

package autostart

import (
	"errors"
	"strings"
	"unicode/utf16"

	"golang.org/x/sys/windows/registry"
)

const runRegistryKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// runStore isolates registry access so tests never modify login startup items.
type runStore interface {
	read() (string, error)
	write(command string) error
	remove() error
}

type windowsRegistration struct{ store runStore }

func newRegistration() registration {
	return &windowsRegistration{store: currentUserRunStore{}}
}

func (r *windowsRegistration) status(target string) (Status, error) {
	command, err := r.store.read()
	if errors.Is(err, registry.ErrNotExist) {
		return Status{}, nil
	}
	if err != nil {
		return Status{}, err
	}
	expected, err := windowsCommand(target)
	if err != nil {
		return Status{}, err
	}
	valid := strings.EqualFold(strings.TrimSpace(command), expected)
	// This reports the Run registration. Windows may separately block it via
	// system startup settings; StartupApproved has no documented public format.
	return Status{Enabled: valid, NeedsRepair: !valid}, nil
}

func (r *windowsRegistration) enable(target string) error {
	command, err := windowsCommand(target)
	if err != nil {
		return err
	}
	return r.store.write(command)
}

func (r *windowsRegistration) disable() error {
	if err := r.store.remove(); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

func windowsCommand(target string) (string, error) {
	if strings.ContainsAny(target, "\"\x00\r\n") {
		return "", errors.New("unsupported Windows executable path")
	}
	command := `"` + target + `"`
	// Run values have a documented 260-character command-line limit.
	if len(utf16.Encode([]rune(command))) > 260 {
		return "", errors.New("startup executable path is too long")
	}
	return command, nil
}

type currentUserRunStore struct{}

func (currentUserRunStore) read() (string, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, runRegistryKey, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer key.Close()
	value, _, err := key.GetStringValue(Identifier)
	return value, err
}

func (currentUserRunStore) write(command string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, runRegistryKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue(Identifier, command)
}

func (currentUserRunStore) remove() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, runRegistryKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.DeleteValue(Identifier)
}
