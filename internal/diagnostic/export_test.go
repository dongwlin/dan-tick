package diagnostic

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestExportIncludesOnlyReviewedLogsAndPlatformMetadata(t *testing.T) {
	logger := New(t.TempDir())
	defer logger.Close()
	logger.Record(AppStarting)
	logger.RecordExecution(ExecutionFailed, 20, 10, "timeout")
	path := filepath.Join(t.TempDir(), "diagnostics.zip")
	require.NoError(t, logger.Export(path, "v1.2.3"))
	archive, err := zip.OpenReader(path)
	require.NoError(t, err)
	defer archive.Close()
	require.Len(t, archive.File, 2)
	require.Equal(t, "manifest.json", archive.File[0].Name)
	reader, err := archive.File[0].Open()
	require.NoError(t, err)
	var info struct {
		Version    string `json:"version"`
		Platform   string `json:"platform"`
		ExportedAt string `json:"exportedAt"`
	}
	require.NoError(t, json.NewDecoder(reader).Decode(&info))
	require.NoError(t, reader.Close())
	require.Equal(t, "v1.2.3", info.Version)
	require.NotEmpty(t, info.Platform)
	require.NotEmpty(t, info.ExportedAt)
	require.Equal(t, "logs/"+activeName, archive.File[1].Name)
	reader, err = archive.File[1].Open()
	require.NoError(t, err)
	buffer, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	require.Contains(t, string(buffer), `"event":"execution.failed"`)
	require.NotContains(t, string(buffer), "Cookie")
}

func TestExportRejectsUnknownFieldsAndDoesNotWritePartialArchive(t *testing.T) {
	logger := New(t.TempDir())
	defer logger.Close()
	logger.Record(AppStarting)
	file := filepath.Join(logger.Status().Directory, activeName)
	f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0600)
	require.NoError(t, err)
	_, err = f.WriteString(`{"time":"2026-01-01T00:00:00Z","level":"info","event":"app.starting","component":"app","result":"started","cookie":"SESSDATA=secret"}` + "\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	output := filepath.Join(t.TempDir(), "blocked.zip")
	require.ErrorIs(t, logger.Export(output, "development"), ErrUnsafeExport)
	_, err = os.Stat(output)
	require.True(t, os.IsNotExist(err))
}

func TestExportSurvivesConcurrentRotation(t *testing.T) {
	dir := t.TempDir()
	w := newRollingWriter(dir, policy{maxSize: 400, maxFiles: 4, maxAge: 14 * 24 * time.Hour}, time.Now)
	logger := &Logger{writer: w, log: zerolog.New(w).With().Timestamp().Logger()}
	defer logger.Close()
	var writes sync.WaitGroup
	writes.Add(1)
	go func() {
		defer writes.Done()
		for range 100 {
			logger.Record(DatabaseReady)
		}
	}()
	for range 5 {
		path := filepath.Join(t.TempDir(), "snapshot.zip")
		require.NoError(t, logger.Export(path, "development"))
		archive, err := zip.OpenReader(path)
		require.NoError(t, err)
		for _, file := range archive.File {
			require.True(t, strings.HasPrefix(file.Name, "logs/") || file.Name == "manifest.json")
		}
		require.NoError(t, archive.Close())
	}
	writes.Wait()
}

func TestSanitizationEliminatesDuplicateJSONKeys(t *testing.T) {
	line := `{"time":"2026-01-01T00:00:00Z","level":"info","event":"app.starting","component":"secret=hidden","component":"app","result":"started"}` + "\n"
	clean, err := sanitizeLog([]byte(line))
	require.NoError(t, err)
	require.NotContains(t, string(clean), "secret=hidden")
	require.Equal(t, 1, strings.Count(string(clean), `"component"`))
}
