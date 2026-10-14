//go:build linux

package autostart

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func newLinuxTestManager(t *testing.T, target string) (*Manager, string) {
	t.Helper()
	config := t.TempDir()
	r := &linuxRegistration{configDir: func() (string, error) { return config, nil }}
	m := &Manager{platform: r, executable: func() (string, error) { return target, nil }}
	return m, filepath.Join(config, "autostart", Identifier+".desktop")
}

func TestLinuxRegistrationRoundTripAndMovedExecutable(t *testing.T) {
	ctx := context.Background()
	m, path := newLinuxTestManager(t, "/Applications/Dan Tick.AppImage")
	m.platform.(*linuxRegistration).isAppImage = true
	status, err := m.Status(ctx)
	require.NoError(t, err)
	require.Equal(t, Status{Supported: true}, status)
	status, err = m.SetEnabled(ctx, true)
	require.NoError(t, err)
	require.True(t, status.Enabled)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(data), "Exec=\"/Applications/Dan Tick.AppImage\" --appimage-extract-and-run\n")
	require.Contains(t, string(data), "Hidden=false")
	_, err = m.SetEnabled(ctx, true)
	require.NoError(t, err)
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	m.executable = func() (string, error) { return "/Applications/New Version.AppImage", nil }
	status, err = m.Status(ctx)
	require.NoError(t, err)
	require.False(t, status.Enabled)
	require.True(t, status.NeedsRepair)
	status, err = m.SetEnabled(ctx, true)
	require.NoError(t, err)
	require.True(t, status.Enabled)
	require.False(t, status.NeedsRepair)
	// Disable removes the fixed ID after another path change and leaves other
	// programs' startup entries untouched.
	unrelated := filepath.Join(filepath.Dir(path), "other.desktop")
	require.NoError(t, os.WriteFile(unrelated, []byte("other application"), 0644))
	m.executable = func() (string, error) { return "/Applications/Moved Again.AppImage", nil }
	_, err = m.SetEnabled(ctx, false)
	require.NoError(t, err)
	_, err = os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(unrelated)
	require.NoError(t, err)
	_, err = m.SetEnabled(ctx, false)
	require.NoError(t, err)
}

func TestLinuxRegistrationReflectsExternalDisableAndInvalidEntry(t *testing.T) {
	tests := []struct {
		name   string
		edit   func(string) string
		repair bool
	}{
		{"hidden by desktop settings", func(data string) string { return strings.ReplaceAll(data, "Hidden=false", "Hidden=true") }, false},
		{"disabled by GNOME", func(data string) string {
			return strings.ReplaceAll(data, "X-GNOME-Autostart-enabled=true", "X-GNOME-Autostart-enabled=false")
		}, false},
		{"stale command", func(data string) string { return strings.ReplaceAll(data, "/current/dan-tick", "/old/dan-tick") }, true},
		{"duplicate exec", func(data string) string { return data + "Exec=/different/program\n" }, true},
		{"restricted desktop", func(data string) string { return data + "OnlyShowIn=KDE;\n" }, true},
		{"missing group", func(string) string { return "Exec=/current/dan-tick\n" }, true},
		{"missing name", func(data string) string { return strings.ReplaceAll(data, "Name=dan-tick\n", "") }, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m, path := newLinuxTestManager(t, "/current/dan-tick")
			_, err := m.SetEnabled(context.Background(), true)
			require.NoError(t, err)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, []byte(test.edit(string(data))), 0644))
			status, err := m.Status(context.Background())
			require.NoError(t, err)
			require.False(t, status.Enabled)
			require.Equal(t, test.repair, status.NeedsRepair)
			_, err = m.SetEnabled(context.Background(), false)
			require.NoError(t, err)
			_, err = os.Stat(path)
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

func TestLinuxDesktopExecEscapesSpecialCharacters(t *testing.T) {
	quoted, err := desktopExec("/Apps/a b\\c\"$`&;'().AppImage")
	require.NoError(t, err)
	require.Equal(t, `"/Apps/a b\\\\c\\"\\$\\`+"`"+`&;'().AppImage"`, quoted)
	for _, target := range []string{"relative/AppImage", "/Apps/a=b", "/Apps/a%b", "/Apps/a\nInjected=true", "/Apps/a\x00b", "/Apps/a\tb"} {
		_, err := desktopExec(target)
		require.Error(t, err)
	}
}

func TestLinuxUnlaunchableExecutablePathCannotClaimEnabled(t *testing.T) {
	for _, target := range []string{"/Applications/Dan=Tick.AppImage", "/Applications/Dan%Tick.AppImage"} {
		m, path := newLinuxTestManager(t, target)
		status, err := m.SetEnabled(context.Background(), true)
		require.ErrorIs(t, err, ErrUnavailable)
		require.False(t, status.Enabled)
		_, err = os.Stat(path)
		require.ErrorIs(t, err, os.ErrNotExist)
	}
}

func TestLinuxAppImageUsesPersistentTarget(t *testing.T) {
	image := filepath.Join(t.TempDir(), "Dan Tick.AppImage")
	require.NoError(t, os.WriteFile(image, []byte("test image"), 0700))
	t.Setenv("APPIMAGE", image)
	target, err := executablePath()
	require.NoError(t, err)
	require.Equal(t, image, target)
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	m := NewManager()
	status, err := m.SetEnabled(context.Background(), true)
	require.NoError(t, err)
	require.True(t, status.Enabled)
	data, err := os.ReadFile(filepath.Join(config, "autostart", Identifier+".desktop"))
	require.NoError(t, err)
	exec, err := desktopExec(image)
	require.NoError(t, err)
	require.Contains(t, string(data), "Exec="+exec+" --appimage-extract-and-run\n")
	t.Setenv("APPIMAGE", "relative/image.AppImage")
	_, err = executablePath()
	require.Error(t, err)
	t.Setenv("APPIMAGE", filepath.Join(t.TempDir(), "missing.AppImage"))
	_, err = executablePath()
	require.Error(t, err)
}

func TestLinuxOrdinaryBinaryDoesNotReceiveAppImageArguments(t *testing.T) {
	m, path := newLinuxTestManager(t, "/Applications/dan-tick")
	_, err := m.SetEnabled(context.Background(), true)
	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(data), "Exec=\"/Applications/dan-tick\"\n")
	require.NotContains(t, string(data), "--appimage-extract-and-run")
}

func TestLinuxConfigUsesAbsoluteXDGDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	actual, err := userConfigDir()
	require.NoError(t, err)
	require.Equal(t, dir, actual)
	t.Setenv("XDG_CONFIG_HOME", "relative")
	actual, err = userConfigDir()
	require.NoError(t, err)
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, ".config"), actual)
}

func TestLinuxRegistrationFailureDoesNotClaimEnabled(t *testing.T) {
	m, path := newLinuxTestManager(t, "/current/dan-tick")
	require.NoError(t, os.WriteFile(filepath.Dir(path), []byte("blocks directory creation"), 0600))
	_, err := m.SetEnabled(context.Background(), true)
	require.ErrorIs(t, err, ErrUnavailable)
	_, err = m.Status(context.Background())
	require.ErrorIs(t, err, ErrUnavailable)
}
