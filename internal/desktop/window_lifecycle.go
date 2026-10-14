package desktop

import (
	"context"
	"sync/atomic"

	"github.com/dongwlin/dan-tick/internal/settings"
	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// WindowLifecycle coordinates the native close button, frontend exit action,
// and single-instance activation without tying any of them to the scheduler.
type WindowLifecycle struct {
	app         *application.App
	db          *bun.DB
	window      application.Window
	behavior    atomic.Value // settings.CloseHide or settings.CloseQuit
	startHidden atomic.Bool
	shown       atomic.Bool
	exiting     atomic.Bool
	confirming  atomic.Bool
}

func NewWindowLifecycle(app *application.App, db *bun.DB, behavior string, hiddenOnStart bool) *WindowLifecycle {
	lifecycle := &WindowLifecycle{app: app, db: db}
	lifecycle.SetCloseBehavior(behavior)
	lifecycle.startHidden.Store(hiddenOnStart)
	return lifecycle
}

func (l *WindowLifecycle) SetCloseBehavior(behavior string) { l.behavior.Store(behavior) }

// ShouldStartHidden reports whether the main window is created without being
// shown. Hiding the window at launch is only safe while the user can still
// reach it: the tray entry and relaunching the executable both restore it.
func (l *WindowLifecycle) ShouldStartHidden() bool { return l.startHidden.Load() }

// MarkShown records that the main window has become visible to the user. Every
// path that brings a hidden window back goes through Show, which calls this.
func (l *WindowLifecycle) MarkShown() { l.shown.Store(true) }

// Show restores the main window and records it as visible.
func (l *WindowLifecycle) Show() {
	if l.window == nil {
		return
	}
	RestoreWindow(l.window)
	l.MarkShown()
}

func (l *WindowLifecycle) Attach(window application.Window) {
	l.window = window
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		// The native close button and the explicit quit action share one rule.
		if l.closeWindow() {
			event.Cancel()
		}
	})
}

// closeWindow answers a close request. It reports whether the window stays
// open, and hides it for the background close behavior.
func (l *WindowLifecycle) closeWindow() bool {
	// A window that was never shown must not answer the close request by
	// hiding itself again; that would strand the application invisibly.
	if l.exiting.Load() || !l.shown.Load() {
		return false
	}
	if l.behavior.Load() == settings.CloseHide {
		if l.window != nil {
			l.window.Hide()
		}
		return true
	}
	l.RequestQuit()
	return true
}

// RequestQuit is also the explicit Quit binding. Both native close and the
// frontend button have the same confirmation rule; cancelling leaves Cron live.
func (l *WindowLifecycle) RequestQuit() {
	if !l.confirming.CompareAndSwap(false, true) {
		return
	}
	go func() {
		needsWarning, err := needsExitWarning(context.Background(), l.db)
		if err == nil && !needsWarning {
			l.quit()
			return
		}
		dialog := l.app.Dialog.Question().
			SetTitle("退出 dan-tick？").
			SetMessage("退出后定时任务将停止执行，直到应用再次启动。正在执行的任务也会停止。").
			AttachToWindow(l.window)
		dialog.AddButton("继续运行").SetAsCancel().OnClick(func() {
			l.confirming.Store(false)
		})
		dialog.AddButton("退出应用").OnClick(l.quit)
		dialog.Show()
	}()
}

func (l *WindowLifecycle) quit() {
	l.exiting.Store(true)
	go l.app.Quit()
}

// Fail closed: if querying the task state fails, show the exit warning.
func needsExitWarning(ctx context.Context, db *bun.DB) (bool, error) {
	var active bool
	err := db.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM tasks WHERE enabled = 1 AND deleted_at IS NULL
		UNION ALL
		SELECT 1 FROM executions WHERE status IN ('waiting', 'queued', 'running')
	)`).Scan(&active)
	return active, err
}
