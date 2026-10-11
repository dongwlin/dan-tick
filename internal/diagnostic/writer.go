package diagnostic

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"
)

const (
	activeName  = "dan-tick.jsonl"
	warningText = "Diagnostic logs cannot be written or maintained; disk logging is disabled. Check the log directory permissions and free space."
	// A broken logger cannot write indefinitely to an absent terminal either.
	maxFallbackLines = 8
)

var backupName = regexp.MustCompile(`^dan-tick-(\d{8}T\d{6}\.\d{9})\.jsonl$`)

type policy struct {
	maxSize  int64
	maxFiles int // includes the active file
	maxAge   time.Duration
}

func defaultPolicy() policy {
	return policy{maxSize: 10 * 1024 * 1024, maxFiles: 5, maxAge: 14 * 24 * time.Hour}
}

// rollingWriter serializes both writes and rotation. Only files matching our
// timestamped backup pattern are eligible for deletion.
type rollingWriter struct {
	mu           sync.Mutex
	dir          string
	file         *os.File
	size         int64
	lastWrite    time.Time
	policy       policy
	now          func() time.Time
	lastCleanup  time.Time
	degraded     bool
	closed       bool
	fallbackLeft int
}

func newRollingWriter(dir string, p policy, now func() time.Time) *rollingWriter {
	w := &rollingWriter{dir: dir, policy: p, now: now, fallbackLeft: maxFallbackLines}
	if err := w.open(); err != nil {
		w.failLocked()
	} else if err := w.cleanupLocked(); err != nil {
		w.failLocked()
	}
	return w
}

func (w *rollingWriter) open() error {
	if err := os.MkdirAll(w.dir, 0700); err != nil {
		return err
	}
	// Existing directories/files may have been created with permissive modes.
	if err := os.Chmod(w.dir, 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(w.dir, activeName), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if err := f.Chmod(0600); err != nil {
		_ = f.Close()
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	w.file, w.size, w.lastWrite = f, info.Size(), info.ModTime().UTC()
	return nil
}

func (w *rollingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return len(p), nil
	}
	if w.degraded {
		w.fallbackLocked(p)
		return len(p), nil
	}
	if len(p) > int(w.policy.maxSize) {
		w.failLocked()
		w.fallbackLocked(p)
		return len(p), nil
	}
	if w.size > 0 && (w.size+int64(len(p)) > w.policy.maxSize || !sameDay(w.lastWrite, w.now())) {
		if err := w.rotateLocked(); err != nil {
			w.failLocked()
			w.fallbackLocked(p)
			return len(p), nil
		}
	}
	// Cleanup must also occur on days without a size rotation.
	if w.now().Sub(w.lastCleanup) >= 24*time.Hour {
		if err := w.cleanupLocked(); err != nil {
			w.failLocked()
			w.fallbackLocked(p)
			return len(p), nil
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	if n > 0 {
		w.lastWrite = w.now().UTC()
	}
	if err != nil || n != len(p) {
		w.failLocked()
		w.fallbackLocked(p)
	}
	return len(p), nil
}

func (w *rollingWriter) rotateLocked() error {
	if err := w.file.Close(); err != nil {
		w.file = nil
		return err
	}
	w.file = nil
	// Reserve a unique name without overwriting an older backup. Timestamp
	// precision prevents normal collisions, including rapid test rotations.
	stamp := w.lastWrite.UTC()
	var dest string
	for {
		dest = filepath.Join(w.dir, "dan-tick-"+stamp.Format("20060102T150405.000000000")+".jsonl")
		_, err := os.Lstat(dest)
		if os.IsNotExist(err) {
			break
		}
		if err != nil {
			return err
		}
		stamp = stamp.Add(time.Nanosecond)
	}
	if err := os.Rename(filepath.Join(w.dir, activeName), dest); err != nil {
		return err
	}
	if err := w.open(); err != nil {
		return err
	}
	return w.cleanupLocked()
}

func sameDay(a, b time.Time) bool {
	a, b = a.UTC(), b.UTC()
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

func (w *rollingWriter) cleanupLocked() error {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return err
	}
	type backup struct {
		name string
		date time.Time
	}
	var backups []backup
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		match := backupName.FindStringSubmatch(e.Name())
		if match == nil {
			continue
		}
		date, err := time.Parse("20060102T150405.000000000", match[1])
		if err == nil {
			backups = append(backups, backup{name: e.Name(), date: date})
		}
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].date.After(backups[j].date) })
	cutoff := w.now().UTC().Add(-w.policy.maxAge)
	for i, b := range backups {
		if i < w.policy.maxFiles-1 && !b.date.Before(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(w.dir, b.name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	w.lastCleanup = w.now()
	return nil
}

func (w *rollingWriter) failLocked() {
	if w.degraded {
		return
	}
	w.degraded = true
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	// Never print an underlying filesystem error: paths and error messages
	// could include private data. Only one generic warning is emitted.
	_, _ = fmt.Fprintln(os.Stderr, "dan-tick:", warningText)
}

func (w *rollingWriter) fallbackLocked(p []byte) {
	if w.fallbackLeft > 0 {
		w.fallbackLeft--
		_, _ = os.Stderr.Write(p)
	}
}

func (w *rollingWriter) status() Status {
	w.mu.Lock()
	defer w.mu.Unlock()
	s := Status{Directory: w.dir, Degraded: w.degraded}
	if w.degraded {
		s.Warning = warningText
	}
	return s
}

func (w *rollingWriter) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			w.failLocked()
		}
		w.file = nil
	}
}
