package diagnostic

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var ErrUnsafeExport = errors.New("diagnostic log contains unrecognized fields or unsafe content")

// Export writes an atomic ZIP of bounded, validated application events.
// Locking the rolling writer during the snapshot keeps a rotation from
// renaming or removing a file while it is being copied. The archive contains
// no SQLite data, Keyring entries, paths, message bodies, or raw exceptions.
func (l *Logger) Export(destination, version string) error {
	logs, err := l.exportSnapshot()
	if err != nil {
		return err
	}
	if len(version) == 0 || len(version) > 80 || !safeValue(version) {
		version = "development"
	}
	if info, err := os.Lstat(destination); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("invalid export destination")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".dan-tick-export-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	defer temporary.Close()
	if err := temporary.Chmod(0600); err != nil {
		return err
	}
	archive := zip.NewWriter(temporary)
	meta, _ := json.MarshalIndent(struct {
		Version    string `json:"version"`
		Platform   string `json:"platform"`
		Arch       string `json:"arch"`
		ExportedAt string `json:"exportedAt"`
	}{version, runtime.GOOS, runtime.GOARCH, time.Now().UTC().Format(time.RFC3339)}, "", "  ")
	manifest, err := archive.Create("manifest.json")
	if err == nil {
		_, err = manifest.Write(meta)
	}
	for _, item := range logs {
		if err != nil {
			break
		}
		var entry interface{ Write([]byte) (int, error) }
		entry, err = archive.Create("logs/" + item.name)
		if err == nil {
			_, err = entry.Write(item.data)
		}
	}
	if closeErr := archive.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), destination)
}

type exportedLog struct {
	name string
	data []byte
}

func (l *Logger) exportSnapshot() ([]exportedLog, error) {
	w := l.writer
	w.mu.Lock()
	defer w.mu.Unlock()
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return nil, err
	}
	var result []exportedLog
	for _, entry := range entries {
		name := entry.Name()
		if name != activeName && !backupName.MatchString(name) {
			continue
		}
		// Reject symlinks, devices and injected filenames rather than following
		// anything outside the logger's exclusively managed directory.
		info, err := os.Lstat(filepath.Join(w.dir, name))
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() > defaultPolicy().maxSize {
			return nil, ErrUnsafeExport
		}
		data, err := os.ReadFile(filepath.Join(w.dir, name))
		if err != nil {
			return nil, err
		}
		data, err = sanitizeLog(data)
		if err != nil {
			return nil, err
		}
		result = append(result, exportedLog{name: name, data: data})
		if len(result) > defaultPolicy().maxFiles {
			return nil, ErrUnsafeExport
		}
	}
	return result, nil
}

var exportFields = map[string]bool{
	"time": true, "level": true, "event": true, "component": true,
	"result": true, "error_category": true, "execution_id": true,
	"task_id": true, "scheduled_at": true, "attempt_sequence": true,
}

// Validate every field before exporting. This fails closed for old/unexpected
// log schemas so arbitrary text cannot masquerade as a reviewed event.
func sanitizeLog(data []byte) ([]byte, error) {
	if len(data) > 0 && data[len(data)-1] != '\n' {
		return nil, ErrUnsafeExport
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 1024), 4096)
	var sanitized bytes.Buffer
	for scanner.Scan() {
		line := scanner.Bytes()
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(line, &fields); err != nil {
			return nil, ErrUnsafeExport
		}
		if len(fields) < 5 || len(fields) > len(exportFields) {
			return nil, ErrUnsafeExport
		}
		var name, component, result, level, category string
		for key, raw := range fields {
			if !exportFields[key] {
				return nil, ErrUnsafeExport
			}
			switch key {
			case "time", "level", "event", "component", "result", "error_category":
				var value string
				if json.Unmarshal(raw, &value) != nil || !safeValue(value) {
					return nil, ErrUnsafeExport
				}
				if key == "time" {
					if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
						return nil, ErrUnsafeExport
					}
				} else if key == "level" && value != "info" && value != "warn" && value != "error" {
					return nil, ErrUnsafeExport
				}
				switch key {
				case "event":
					name = value
				case "component":
					component = value
				case "result":
					result = value
				case "level":
					level = value
				case "error_category":
					category = value
				}
			case "execution_id", "task_id", "scheduled_at", "attempt_sequence":
				var value int64
				if json.Unmarshal(raw, &value) != nil || value <= 0 {
					return nil, ErrUnsafeExport
				}
			}
		}
		for _, mandatory := range []string{"time", "level", "event", "component", "result"} {
			if _, ok := fields[mandatory]; !ok {
				return nil, ErrUnsafeExport
			}
		}
		if !allowedEvent(name, component, result, level, category) {
			return nil, ErrUnsafeExport
		}
		// Re-encode, never copy source bytes into the archive: duplicate JSON
		// keys cannot conceal a secret in an otherwise accepted log line.
		clean, err := json.Marshal(fields)
		if err != nil {
			return nil, ErrUnsafeExport
		}
		sanitized.Write(clean)
		sanitized.WriteByte('\n')
	}
	if scanner.Err() != nil {
		return nil, ErrUnsafeExport
	}
	return sanitized.Bytes(), nil
}

func allowedEvent(name, component, result, level, category string) bool {
	known := []Event{
		AppStarting, AppStopped, AppFailed, AppPanicked, DatabaseReady,
		DatabaseOpenFailed, DatabaseCloseFailed, DesktopInitFailed, DesktopRunFailed, DesktopError,
		ExecutionStarted, ExecutionQueued, ExecutionConflict, ExecutionCompleted,
		ExecutionStopped, ExecutionFailed, ExecutionPersistFailed, ExecutionInterrupted,
		SendAccepted, SendRejected, SendNotSent, SendUnknown,
		ScheduleClaimed, ScheduleConflict, ScheduleSyncFailed, ScheduleMissed,
		RecoveryFailed, HistoryCleanupFailed,
	}
	for _, event := range known {
		if event.name == name && event.component == component && event.result == result && event.level.String() == level {
			return category == "" || category == event.errorCategory || safeSendCategory(category) == category
		}
	}
	return false
}

func safeValue(value string) bool {
	if value == "" || len(value) > 80 {
		return false
	}
	for _, char := range value {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._:+-TZ", char) {
			return false
		}
	}
	return true
}
