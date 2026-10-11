package desktop

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/dongwlin/dan-tick/internal/settings"
	"github.com/dongwlin/dan-tick/internal/storage"
	"github.com/stretchr/testify/require"
)

func TestWindowServiceUpdatesCommittedBehaviorAndSanitizesErrors(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "settings.db"))
	require.NoError(t, err)
	defer db.Close()

	var changed []string
	quitCalled := false
	service := NewWindowService(settings.NewService(db), func(value string) {
		changed = append(changed, value)
	}, func() { quitCalled = true })

	value, err := service.GetCloseBehavior(ctx)
	require.NoError(t, err)
	require.Equal(t, settings.CloseHide, value)
	require.Error(t, service.SetCloseBehavior(ctx, "invalid"))
	require.Empty(t, changed)
	require.NoError(t, service.SetCloseBehavior(ctx, settings.CloseQuit))
	require.Equal(t, []string{settings.CloseQuit}, changed)
	value, err = service.GetCloseBehavior(ctx)
	require.NoError(t, err)
	require.Equal(t, settings.CloseQuit, value)
	service.Quit()
	require.True(t, quitCalled)

	_, err = db.ExecContext(ctx, `CREATE TRIGGER deny_window_setting BEFORE UPDATE ON app_settings
		BEGIN SELECT RAISE(ABORT, 'private database detail'); END`)
	require.NoError(t, err)
	err = service.SetCloseBehavior(ctx, settings.CloseHide)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "private database detail")
	require.Equal(t, []string{settings.CloseQuit}, changed)
}
