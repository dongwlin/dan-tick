// Package diagnostic provides persistent, privacy-conscious application events.
// Task execution history belongs in SQLite, not in this log.
package diagnostic

import (
	"path/filepath"
	"time"

	"github.com/adrg/xdg"
	"github.com/rs/zerolog"
)

// DefaultDir is independent of the database path, so database failures can be logged.
func DefaultDir() string {
	return filepath.Join(xdg.StateHome, "dan-tick", "logs")
}

// Event deliberately has no public fields. Callers can only use predefined,
// reviewed events; arbitrary strings, errors, URLs or request bodies cannot
// accidentally enter the diagnostic log through this API.
type Event struct {
	name, component, result, errorCategory string
	level                                  zerolog.Level
}

var (
	AppStarting            = Event{name: "app.starting", component: "app", result: "started", level: zerolog.InfoLevel}
	AppStopped             = Event{name: "app.stopped", component: "app", result: "completed", level: zerolog.InfoLevel}
	AppFailed              = Event{name: "app.failed", component: "app", result: "failed", errorCategory: "application", level: zerolog.ErrorLevel}
	AppPanicked            = Event{name: "app.panicked", component: "app", result: "failed", errorCategory: "unexpected_panic", level: zerolog.ErrorLevel}
	DatabaseReady          = Event{name: "database.ready", component: "storage", result: "ok", level: zerolog.InfoLevel}
	DatabaseOpenFailed     = Event{name: "database.open_failed", component: "storage", result: "failed", errorCategory: "database_open", level: zerolog.ErrorLevel}
	DatabaseCloseFailed    = Event{name: "database.close_failed", component: "storage", result: "failed", errorCategory: "database_close", level: zerolog.ErrorLevel}
	DesktopRunFailed       = Event{name: "desktop.run_failed", component: "desktop", result: "failed", errorCategory: "desktop_runtime", level: zerolog.ErrorLevel}
	ExecutionStarted       = Event{name: "execution.started", component: "executor", result: "running", level: zerolog.InfoLevel}
	ExecutionQueued        = Event{name: "execution.queued", component: "executor", result: "queued", level: zerolog.InfoLevel}
	ExecutionConflict      = Event{name: "execution.conflict", component: "executor", result: "rejected", errorCategory: "task_busy", level: zerolog.WarnLevel}
	ExecutionCompleted     = Event{name: "execution.completed", component: "executor", result: "completed", level: zerolog.InfoLevel}
	ExecutionStopped       = Event{name: "execution.stopped", component: "executor", result: "stopped", level: zerolog.InfoLevel}
	ExecutionFailed        = Event{name: "execution.failed", component: "executor", result: "failed", level: zerolog.WarnLevel}
	ExecutionPersistFailed = Event{name: "execution.persist_failed", component: "executor", result: "failed", errorCategory: "persistence", level: zerolog.ErrorLevel}
	SendAccepted           = Event{name: "send.resolved", component: "executor", result: "accepted", level: zerolog.InfoLevel}
	SendRejected           = Event{name: "send.resolved", component: "executor", result: "rejected", level: zerolog.WarnLevel}
	SendNotSent            = Event{name: "send.resolved", component: "executor", result: "not_sent", level: zerolog.WarnLevel}
	SendUnknown            = Event{name: "send.resolved", component: "executor", result: "unknown", level: zerolog.WarnLevel}
	ScheduleClaimed        = Event{name: "schedule.claimed", component: "scheduler", result: "waiting", level: zerolog.InfoLevel}
	ScheduleConflict       = Event{name: "schedule.conflict", component: "scheduler", result: "missed", errorCategory: "task_busy", level: zerolog.WarnLevel}
)

// Status can be read by a future settings/diagnostics UI without exposing
// backend errors or any sensitive event payload.
type Status struct {
	Directory string
	Degraded  bool
	Warning   string
}

type Logger struct {
	log    zerolog.Logger
	writer *rollingWriter
}

// New always returns a working logger. If persistence is unavailable, a bounded
// stderr fallback is used and Status reports the problem without blocking startup.
func New(dir string) *Logger {
	w := newRollingWriter(dir, defaultPolicy(), time.Now)
	return &Logger{
		log:    zerolog.New(w).Level(zerolog.InfoLevel).With().Timestamp().Logger(),
		writer: w,
	}
}

func (l *Logger) Record(event Event) {
	if event.name == "" {
		return
	}
	e := l.log.WithLevel(event.level).
		Str("event", event.name).
		Str("component", event.component).
		Str("result", event.result)
	if event.errorCategory != "" {
		e = e.Str("error_category", event.errorCategory)
	}
	e.Send()
}

// RecordExecution accepts only predefined events, integer identifiers and a
// whitelisted error category. Message contents and arbitrary provider errors
// cannot become log fields, even if passed by mistake.
func (l *Logger) RecordExecution(event Event, executionID, taskID int64, category string) {
	l.recordExecution(event, executionID, taskID, 0, category)
}

// RecordSendAttempt adds the positive attempt sequence to a predefined send
// event. It never accepts message text or raw upstream diagnostics.
func (l *Logger) RecordSendAttempt(event Event, executionID, taskID int64, sequence int, category string) {
	switch event {
	case SendAccepted, SendRejected, SendNotSent, SendUnknown:
		if sequence > 0 {
			l.recordExecution(event, executionID, taskID, sequence, category)
		}
	}
}

// RecordSchedule only accepts predeclared scheduled-claim events and numeric
// occurrence identifiers. No task content, request, or raw error can be logged.
func (l *Logger) RecordSchedule(event Event, executionID, taskID, scheduledAt int64) {
	if executionID <= 0 || taskID <= 0 || scheduledAt <= 0 {
		return
	}
	if event != ScheduleClaimed && event != ScheduleConflict {
		return
	}
	e := l.log.WithLevel(event.level).
		Str("event", event.name).
		Str("component", event.component).
		Str("result", event.result).
		Int64("execution_id", executionID).
		Int64("task_id", taskID).
		Int64("scheduled_at", scheduledAt)
	if event.errorCategory != "" {
		e = e.Str("error_category", event.errorCategory)
	}
	e.Send()
}

func (l *Logger) recordExecution(event Event, executionID, taskID int64, sequence int, category string) {
	if executionID <= 0 || taskID <= 0 {
		return
	}
	switch event {
	case ExecutionQueued, ExecutionConflict, ExecutionStarted, ExecutionCompleted, ExecutionStopped, ExecutionFailed,
		ExecutionPersistFailed, SendAccepted, SendRejected, SendNotSent, SendUnknown:
	default:
		return
	}
	e := l.log.WithLevel(event.level).
		Str("event", event.name).
		Str("component", event.component).
		Str("result", event.result).
		Int64("execution_id", executionID).
		Int64("task_id", taskID)
	if sequence > 0 {
		e = e.Int("attempt_sequence", sequence)
	}
	if event.errorCategory != "" {
		e = e.Str("error_category", event.errorCategory)
	} else if event == SendRejected || event == SendNotSent ||
		event == SendUnknown || event == ExecutionFailed || event == ExecutionStopped {
		e = e.Str("error_category", safeSendCategory(category))
	}
	e.Send()
}

func safeSendCategory(category string) string {
	switch category {
	case "credential_missing", "credential_unavailable", "credential_corrupt",
		"authentication", "rate_limited", "bad_request", "too_long",
		"rejected", "invalid_input", "timeout", "cancelled", "transport",
		"http_status", "invalid_response", "uncertain", "send_failure",
		"user_stopped", "wait_failure", "persistence":
		return category
	default:
		return "send_failure"
	}
}

func (l *Logger) Status() Status { return l.writer.status() }

// Close is called after recording the final event. Write failures never abort
// application shutdown.
func (l *Logger) Close() { l.writer.close() }
