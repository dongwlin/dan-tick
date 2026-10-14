// Package autostart manages this user's login startup registration. The
// operating system registration is authoritative; no preference is cached.
package autostart

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

const Identifier = "io.github.dongwlin.dan-tick"

var (
	ErrUnsupported = errors.New("login startup is not supported")
	ErrUnavailable = errors.New("login startup registration is unavailable")
)

type Status struct {
	Enabled     bool
	Supported   bool
	NeedsRepair bool
}

// registration is the external platform boundary. Implementations only touch
// the stable application identifier, including after the executable moves.
type registration interface {
	status(target string) (Status, error)
	enable(target string) error
	disable() error
}

type Manager struct {
	mu         sync.Mutex
	platform   registration
	executable func() (string, error)
}

func NewManager() *Manager {
	return &Manager{platform: newRegistration(), executable: executablePath}
}

func (m *Manager) Status(ctx context.Context) (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Status{}, err
	}
	return m.status()
}

func (m *Manager) SetEnabled(ctx context.Context, enabled bool) (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Status{}, err
	}
	if m.platform == nil {
		return Status{}, ErrUnsupported
	}
	if enabled {
		target, err := m.executable()
		if err != nil {
			return Status{}, unavailable(err)
		}
		if err := ctx.Err(); err != nil {
			return Status{}, err
		}
		if err := m.platform.enable(target); err != nil {
			return Status{}, unavailable(err)
		}
	} else if err := m.platform.disable(); err != nil {
		return Status{}, unavailable(err)
	}
	// Once the platform change commits, read its result even if the caller was
	// cancelled meanwhile. A completed change cannot be rolled back by context.
	return m.status()
}

func (m *Manager) status() (Status, error) {
	if m.platform == nil {
		return Status{}, nil
	}
	target, err := m.executable()
	if err != nil {
		return Status{}, unavailable(err)
	}
	value, err := m.platform.status(target)
	if err != nil {
		return Status{}, unavailable(err)
	}
	value.Supported = true
	return value, nil
}

func unavailable(err error) error {
	return fmt.Errorf("%w: %w", ErrUnavailable, err)
}

func executablePath() (string, error) {
	var target string
	var err error
	if runtime.GOOS == "linux" {
		// APPDIR/os.Executable refer to the temporary AppImage mount. APPIMAGE
		// points at the persistent image that can be run on the next login.
		target = os.Getenv("APPIMAGE")
	}
	if target == "" {
		target, err = os.Executable()
		if err != nil {
			return "", err
		}
	}
	if !filepath.IsAbs(target) {
		return "", errors.New("startup executable path is not absolute")
	}
	if resolved, err := filepath.EvalSymlinks(target); err == nil {
		target = resolved
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || (runtime.GOOS == "linux" && info.Mode().Perm()&0111 == 0) {
		return "", errors.New("startup executable is not executable")
	}
	return target, nil
}
