package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/adrg/xdg"
	"github.com/dongwlin/dan-tick/internal/diagnostic"
	"github.com/stretchr/testify/require"
	"github.com/wailsapp/wails/v3/pkg/application"
)

var startupAssets = fstest.MapFS{
	"index.html": {Data: []byte("<!doctype html><title>dan-tick test</title>")},
}

func TestStartupSuccessIsPersisted(t *testing.T) {
	oldState := xdg.StateHome
	t.Cleanup(func() { xdg.StateHome = oldState })
	xdg.StateHome = t.TempDir()

	stderr := captureStartupStderr(t, func() {
		require.Equal(t, 0, startWithRun(func(*diagnostic.Logger) error { return nil }))
	})
	events, _ := readStartupLog(t, xdg.StateHome)
	require.Equal(t, []string{"app.starting", "app.stopped"}, events)
	require.Empty(t, stderr)
}

func TestDatabaseStartupFailureIsPersisted(t *testing.T) {
	oldData, oldState := xdg.DataHome, xdg.StateHome
	t.Cleanup(func() { xdg.DataHome, xdg.StateHome = oldData, oldState })
	root := t.TempDir()
	// Deliberately block database directory creation before the Wails UI starts.
	badDataDir := filepath.Join(root, "blocked")
	require.NoError(t, os.WriteFile(badDataDir, []byte("not a directory"), 0600))
	xdg.DataHome = badDataDir
	xdg.StateHome = filepath.Join(root, "state")

	require.Equal(t, 1, Run(startupAssets))
	require.Nil(t, application.Get(), "database directory failure must not initialize Wails")
	events, log := readStartupLog(t, xdg.StateHome)
	require.Equal(t, []string{"app.starting", "database.open_failed", "app.failed"}, events)
	require.NotContains(t, string(log), badDataDir)
	require.NotContains(t, strings.ToLower(string(log)), "not a directory")
}

func TestStartupErrorAndPanicRemainDistinct(t *testing.T) {
	const privateDetail = "private-startup-value=/home/private/session-secret"
	for _, test := range []struct {
		name   string
		run    func(*diagnostic.Logger) error
		events []string
	}{
		{
			name:   "returned error",
			run:    func(*diagnostic.Logger) error { return errors.New(privateDetail) },
			events: []string{"app.starting", "app.failed"},
		},
		{
			name:   "unexpected panic",
			run:    func(*diagnostic.Logger) error { panic(privateDetail) },
			events: []string{"app.starting", "app.panicked"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			oldState := xdg.StateHome
			t.Cleanup(func() { xdg.StateHome = oldState })
			xdg.StateHome = t.TempDir()
			stderr := captureStartupStderr(t, func() {
				require.Equal(t, 1, startWithRun(test.run))
			})
			events, log := readStartupLog(t, xdg.StateHome)
			require.Equal(t, test.events, events)
			require.NotContains(t, string(log), privateDetail)
			require.NotContains(t, stderr, privateDetail)
			if test.name == "unexpected panic" {
				require.Contains(t, string(log), `"error_category":"unexpected_panic"`)
			} else {
				require.NotContains(t, string(log), "unexpected_panic")
			}
		})
	}
}

func TestDesktopInitializationFailureIsPersisted(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the missing session bus case exercises Wails' Linux initialization")
	}
	if os.Getenv("DAN_TICK_TEST_DESKTOP_FAILURE") == "1" {
		xdg.DataHome = os.Getenv("DAN_TICK_TEST_DATA_HOME")
		xdg.StateHome = os.Getenv("DAN_TICK_TEST_STATE_HOME")
		os.Exit(Run(startupAssets))
	}

	root := t.TempDir()
	dataHome, stateHome := filepath.Join(root, "data"), filepath.Join(root, "state")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDesktopInitializationFailureIsPersisted$")
	cmd.Env = append(os.Environ(),
		"DAN_TICK_TEST_DESKTOP_FAILURE=1",
		"DAN_TICK_TEST_DATA_HOME="+dataHome,
		"DAN_TICK_TEST_STATE_HOME="+stateHome,
		"DBUS_SESSION_BUS_ADDRESS=unix:path="+filepath.Join(root, "missing-session-bus"),
	)
	output, err := cmd.CombinedOutput()
	require.NoError(t, ctx.Err(), "isolated startup did not return: %s", output)
	var exitError *exec.ExitError
	require.ErrorAs(t, err, &exitError)
	require.Equal(t, 1, exitError.ExitCode())
	events, log := readStartupLog(t, stateHome)
	require.Equal(t, []string{"app.starting", "desktop.init_failed", "app.failed"}, events)
	_, err = os.Stat(filepath.Join(dataHome, "dan-tick", "dan-tick.db"))
	require.ErrorIs(t, err, os.ErrNotExist, "desktop initialization failure must not open SQLite")
	for _, privateDetail := range []string{root, "missing-session-bus", "no such file or directory", "dial unix"} {
		require.NotContains(t, string(log), privateDetail)
		require.NotContains(t, string(output), privateDetail)
	}
}

func TestWailsFatalAndManagedPanicArePersisted(t *testing.T) {
	const privateDetail = "private-wails-value=/home/private/session-secret"
	if mode := os.Getenv("DAN_TICK_TEST_WAILS_FAILURE"); mode != "" {
		xdg.StateHome = os.Getenv("DAN_TICK_TEST_STATE_HOME")
		os.Exit(startWithRun(func(logger *diagnostic.Logger) error {
			desktopApp := newDesktopApplication(logger, application.Options{
				Assets: application.AssetOptions{Handler: application.AssetFileServerFS(startupAssets)},
			})
			switch mode {
			case "runtime_fatal":
				application.Fatal("%s", privateDetail)
			case "managed_panic":
				desktopApp.Event.On("test:panic", func(*application.CustomEvent) { panic(privateDetail) })
				desktopApp.Event.Emit("test:panic")
				// Wails invokes the listener in a goroutine and its PanicHandler exits.
				select {}
			}
			return nil
		}))
	}

	for _, test := range []struct {
		mode   string
		events []string
	}{
		{mode: "runtime_fatal", events: []string{"app.starting", "desktop.run_failed", "app.failed"}},
		{mode: "managed_panic", events: []string{"app.starting", "app.panicked"}},
	} {
		t.Run(test.mode, func(t *testing.T) {
			stateHome := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWailsFatalAndManagedPanicArePersisted$")
			cmd.Env = append(os.Environ(),
				"DAN_TICK_TEST_WAILS_FAILURE="+test.mode,
				"DAN_TICK_TEST_STATE_HOME="+stateHome,
			)
			output, err := cmd.CombinedOutput()
			require.NoError(t, ctx.Err(), "isolated startup did not return: %s", output)
			var exitError *exec.ExitError
			require.ErrorAs(t, err, &exitError)
			require.Equal(t, 1, exitError.ExitCode())
			events, log := readStartupLog(t, stateHome)
			require.Equal(t, test.events, events)
			for _, privateValue := range []string{privateDetail, stateHome, "goroutine", "startup_test.go", "desktop.go", "Error Details"} {
				require.NotContains(t, string(log), privateValue)
				require.NotContains(t, string(output), privateValue)
			}
		})
	}
}

func readStartupLog(t *testing.T, stateHome string) ([]string, []byte) {
	t.Helper()
	log, err := os.ReadFile(filepath.Join(stateHome, "dan-tick", "logs", "dan-tick.jsonl"))
	require.NoError(t, err)
	var events []string
	for _, line := range bytes.Split(bytes.TrimSpace(log), []byte("\n")) {
		var entry struct {
			Event string `json:"event"`
		}
		require.NoError(t, json.Unmarshal(line, &entry))
		require.NotEmpty(t, entry.Event)
		events = append(events, entry.Event)
	}
	return events, log
}

func captureStartupStderr(t *testing.T, action func()) string {
	t.Helper()
	file, err := os.Create(filepath.Join(t.TempDir(), "stderr"))
	require.NoError(t, err)
	oldStderr := os.Stderr
	os.Stderr = file
	defer func() {
		os.Stderr = oldStderr
		_ = file.Close()
	}()
	action()
	contents, err := os.ReadFile(file.Name())
	require.NoError(t, err)
	return string(contents)
}
