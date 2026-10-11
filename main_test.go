package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adrg/xdg"
	"github.com/stretchr/testify/require"
)

func TestDatabaseStartupFailureIsPersisted(t *testing.T) {
	oldData, oldState := xdg.DataHome, xdg.StateHome
	t.Cleanup(func() { xdg.DataHome, xdg.StateHome = oldData, oldState })
	root := t.TempDir()
	// Deliberately block database directory creation before the Wails UI starts.
	badDataDir := filepath.Join(root, "blocked")
	require.NoError(t, os.WriteFile(badDataDir, []byte("not a directory"), 0600))
	xdg.DataHome = badDataDir
	xdg.StateHome = filepath.Join(root, "state")

	require.Equal(t, 1, start())
	log, err := os.ReadFile(filepath.Join(xdg.StateHome, "dan-tick", "logs", "dan-tick.jsonl"))
	require.NoError(t, err)
	require.Contains(t, string(log), `"event":"app.starting"`)
	require.Contains(t, string(log), `"event":"database.open_failed"`)
	require.Contains(t, string(log), `"event":"app.failed"`)
	require.NotContains(t, string(log), badDataDir)
	require.NotContains(t, strings.ToLower(string(log)), "not a directory")
}
