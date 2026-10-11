// Package settings stores non-sensitive application preferences in SQLite.
// Account credentials are never represented here; they belong in Keyring.
package settings

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"
)

const DefaultTimezone = "Asia/Shanghai"

var ErrInvalidTimezone = errors.New("invalid default timezone")
var ErrInvalidCloseBehavior = errors.New("invalid window close behavior")

const (
	CloseHide = "hide"
	CloseQuit = "quit"
)

type Values struct {
	DefaultTimezone   string
	SchedulingEnabled bool
}

type Service struct{ db *bun.DB }

func NewService(db *bun.DB) *Service { return &Service{db: db} }

// Get returns defaults for preferences that have not yet been written.
func (s *Service) Get(ctx context.Context) (Values, error) {
	zone, err := DefaultZone(ctx, s.db)
	if err != nil {
		return Values{}, err
	}
	enabled, err := SchedulingEnabled(ctx, s.db)
	if err != nil {
		return Values{}, err
	}
	return Values{DefaultTimezone: zone, SchedulingEnabled: enabled}, nil
}

// DefaultZone reads the configured fallback for newly created tasks.
func DefaultZone(ctx context.Context, db bun.IDB) (string, error) {
	value, err := read(ctx, db, "default_timezone", DefaultTimezone)
	if err != nil {
		return "", err
	}
	if !validTimezone(value) {
		return "", fmt.Errorf("stored default timezone: %w", ErrInvalidTimezone)
	}
	return value, nil
}

// SchedulingEnabled may be read inside the same transaction as a scheduled
// claim, preventing a pause committed earlier from permitting a new claim.
func SchedulingEnabled(ctx context.Context, db bun.IDB) (bool, error) {
	value, err := read(ctx, db, "scheduling_enabled", "true")
	if err != nil {
		return false, err
	}
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("invalid persisted scheduling switch")
	}
}

func (s *Service) SetDefaultTimezone(ctx context.Context, timezone string) error {
	timezone = strings.TrimSpace(timezone)
	if !validTimezone(timezone) {
		return ErrInvalidTimezone
	}
	return write(ctx, s.db, "default_timezone", timezone)
}

// SetSchedulingEnabled only persists the preference. When the scheduler is
// running, use scheduler.Manager.SetSchedulingEnabled to synchronize Cron.
func (s *Service) SetSchedulingEnabled(ctx context.Context, enabled bool) error {
	value := "false"
	if enabled {
		value = "true"
	}
	return write(ctx, s.db, "scheduling_enabled", value)
}

// CloseBehavior defaults to background operation. Relaunching the executable
// activates the existing instance, so hiding never strands a window.
func (s *Service) CloseBehavior(ctx context.Context) (string, error) {
	value, err := read(ctx, s.db, "close_behavior", CloseHide)
	if err != nil {
		return "", err
	}
	if value != CloseHide && value != CloseQuit {
		return "", ErrInvalidCloseBehavior
	}
	return value, nil
}

func (s *Service) SetCloseBehavior(ctx context.Context, value string) error {
	if value != CloseHide && value != CloseQuit {
		return ErrInvalidCloseBehavior
	}
	return write(ctx, s.db, "close_behavior", value)
}

func validTimezone(value string) bool {
	if value == "" || value == "Local" {
		return false
	}
	_, err := time.LoadLocation(value)
	return err == nil
}

func read(ctx context.Context, db bun.IDB, key, fallback string) (string, error) {
	var value string
	err := db.QueryRowContext(ctx, "SELECT value FROM app_settings WHERE key = ?", key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return fallback, nil
	}
	if err != nil {
		return "", fmt.Errorf("read application setting %s: %w", key, err)
	}
	return value, nil
}

func write(ctx context.Context, db *bun.DB, key, value string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO app_settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("save application setting %s: %w", key, err)
	}
	return nil
}
