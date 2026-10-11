package settings

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/dongwlin/dan-tick/internal/storage"
	"github.com/stretchr/testify/require"
)

func TestPreferencesPersistAcrossDatabaseReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "preferences.db")
	db, err := storage.Open(ctx, path)
	require.NoError(t, err)
	s := NewService(db)
	values, err := s.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, Values{DefaultTimezone: "Asia/Shanghai", SchedulingEnabled: true}, values)
	var rows int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM app_settings").Scan(&rows))
	require.Zero(t, rows, "reading defaults does not insert rows")

	require.NoError(t, s.SetDefaultTimezone(ctx, " America/New_York "))
	require.NoError(t, s.SetSchedulingEnabled(ctx, false))
	require.NoError(t, s.SetSchedulingEnabled(ctx, false))
	require.NoError(t, db.Close())

	db, err = storage.Open(ctx, path)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	s = NewService(db)
	values, err = s.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, Values{DefaultTimezone: "America/New_York", SchedulingEnabled: false}, values)
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM app_settings").Scan(&rows))
	require.Equal(t, 2, rows)
	require.NoError(t, s.SetSchedulingEnabled(ctx, true))
	values, err = s.Get(ctx)
	require.NoError(t, err)
	require.True(t, values.SchedulingEnabled)
}

func TestInvalidAndFailedPreferenceUpdatesDoNotOverwrite(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "preferences.db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	s := NewService(db)
	for _, zone := range []string{"", "Local", "Mars/Phobos"} {
		require.ErrorIs(t, s.SetDefaultTimezone(ctx, zone), ErrInvalidTimezone)
	}
	require.NoError(t, s.SetDefaultTimezone(ctx, "UTC"))
	require.NoError(t, s.SetSchedulingEnabled(ctx, false))
	_, err = db.ExecContext(ctx, `CREATE TRIGGER reject_settings BEFORE UPDATE ON app_settings
		BEGIN SELECT RAISE(ABORT, 'test preference write rejected'); END`)
	require.NoError(t, err)
	require.Error(t, s.SetDefaultTimezone(ctx, "Europe/London"))
	require.Error(t, s.SetSchedulingEnabled(ctx, true))
	values, err := s.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, Values{DefaultTimezone: "UTC", SchedulingEnabled: false}, values)

	_, err = db.ExecContext(ctx, "DROP TRIGGER reject_settings")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "UPDATE app_settings SET value = 'invalid' WHERE key = 'scheduling_enabled'")
	require.NoError(t, err)
	_, err = s.Get(ctx)
	require.Error(t, err, "invalid stored switch must not silently enable scheduling")
	_, err = db.ExecContext(ctx, "UPDATE app_settings SET value = 'true' WHERE key = 'scheduling_enabled'")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "UPDATE app_settings SET value = 'Local' WHERE key = 'default_timezone'")
	require.NoError(t, err)
	_, err = s.Get(ctx)
	require.ErrorIs(t, err, ErrInvalidTimezone)
}

func TestCloseBehaviorDefaultsValidatesAndPersists(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "preferences.db")
	db, err := storage.Open(ctx, path)
	require.NoError(t, err)
	preferences := NewService(db)
	value, err := preferences.CloseBehavior(ctx)
	require.NoError(t, err)
	require.Equal(t, CloseHide, value)
	require.ErrorIs(t, preferences.SetCloseBehavior(ctx, "invalid"), ErrInvalidCloseBehavior)
	require.NoError(t, preferences.SetCloseBehavior(ctx, CloseQuit))
	require.NoError(t, db.Close())

	db, err = storage.Open(ctx, path)
	require.NoError(t, err)
	defer db.Close()
	preferences = NewService(db)
	value, err = preferences.CloseBehavior(ctx)
	require.NoError(t, err)
	require.Equal(t, CloseQuit, value)
	require.NoError(t, preferences.SetCloseBehavior(ctx, CloseHide))
	_, err = db.ExecContext(ctx, "UPDATE app_settings SET value = 'invalid' WHERE key = 'close_behavior'")
	require.NoError(t, err)
	_, err = preferences.CloseBehavior(ctx)
	require.ErrorIs(t, err, ErrInvalidCloseBehavior)
}
