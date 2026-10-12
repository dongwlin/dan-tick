package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/dongwlin/dan-tick/internal/diagnostic"
	"github.com/stretchr/testify/require"
)

func TestDiagnosticsServiceOpensDirectoriesAndHandlesExportCancel(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	logger := diagnostic.New(dir)
	defer logger.Close()
	var opened []string
	service := NewDiagnosticsService(logger, func(path string) error {
		opened = append(opened, path)
		return nil
	}, func() (string, error) { return "", nil }, "test")
	require.NoError(t, service.OpenLogDirectory())
	require.Equal(t, []string{dir}, opened)
	require.NoError(t, service.OpenDataDirectory())
	require.Len(t, opened, 2)
	require.DirExists(t, dir)
	status := service.GetDiagnosticsStatus()
	require.Equal(t, dir, status.Directory)
	require.False(t, status.Degraded)
	result, err := service.ExportDiagnostics()
	require.NoError(t, err)
	require.True(t, result.Cancelled)
	require.Empty(t, result.Filename)
}

func TestDiagnosticsServiceSanitizesErrorsAndExports(t *testing.T) {
	logger := diagnostic.New(t.TempDir())
	defer logger.Close()
	logger.Record(diagnostic.AppStarting)
	output := filepath.Join(t.TempDir(), "diagnostics.zip")
	service := NewDiagnosticsService(logger, func(string) error { return errors.New("private desktop error") },
		func() (string, error) { return output, nil }, "test-version")
	assertPublicError(t, service.OpenLogDirectory(), "internal", "private desktop error")
	result, err := service.ExportDiagnostics()
	require.NoError(t, err)
	require.False(t, result.Cancelled)
	require.Equal(t, "diagnostics.zip", result.Filename)
	require.FileExists(t, output)
	service.savePath = func() (string, error) { return "", errors.New("private dialog error") }
	_, err = service.ExportDiagnostics()
	assertPublicError(t, err, "internal", "private dialog error")
	service.savePath = func() (string, error) { return filepath.Join(t.TempDir(), "absent", "failure.zip"), nil }
	_, err = service.ExportDiagnostics()
	assertPublicError(t, err, "internal", "failure.zip")
	_, err = os.Stat(output)
	require.NoError(t, err)
}
