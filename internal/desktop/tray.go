package desktop

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"runtime"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed icons/tray-light.ico
var trayLightIcon []byte

//go:embed icons/tray-dark.ico
var trayDarkIcon []byte

//go:embed icons/tray-linux.png
var trayLinuxIcon []byte

// Tray adds desktop entry points to the existing application and window
// services. Wails owns the native tray and destroys it during application exit.
type Tray struct {
	actions *trayActions
}

func NewTray(app *application.App, window application.Window, service *ApplicationService, quit func()) *Tray {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		return nil
	}
	show := func() { RestoreWindow(window) }
	actions := newTrayActions(service, application.InvokeAsync, show, quit, func(title, message string, failed bool) {
		show()
		dialog := app.Dialog.Info()
		if failed {
			dialog = app.Dialog.Error()
		}
		dialog.SetTitle(title).SetMessage(message).AttachToWindow(window)
		dialog.AddButton("确定").SetAsDefault().SetAsCancel()
		dialog.Show()
	})
	tray := app.SystemTray.New().SetMenu(newTrayMenu(actions))
	tray.SetLabel("dan-tick")
	if runtime.GOOS == "windows" {
		tray.SetIcon(trayLightIcon).SetDarkModeIcon(trayDarkIcon)
		tray.SetTooltip("dan-tick")
		tray.OnClick(actions.openWindow)
	} else {
		// Linux's StatusNotifier implementation does not select icons by theme.
		// Menu opening also fires OnClick, so restore only through the menu.
		tray.SetIcon(trayLinuxIcon)
	}
	return &Tray{actions: actions}
}

func newTrayMenu(actions *trayActions) *application.Menu {
	menu := application.NewMenu()
	menu.Add("打开主窗口").OnClick(func(*application.Context) { actions.openWindow() })
	menu.Add("查看运行状态").OnClick(func(*application.Context) { actions.showStatus() })
	menu.AddSeparator()
	// Both operations remain available and idempotent. Status is read on
	// demand rather than cached in native menu state or refreshed by a timer.
	menu.Add("暂停自动调度").OnClick(func(*application.Context) { actions.setSchedulingEnabled(false) })
	menu.Add("恢复自动调度").OnClick(func(*application.Context) { actions.setSchedulingEnabled(true) })
	menu.AddSeparator()
	menu.Add("退出应用").OnClick(func(*application.Context) { actions.requestQuit() })
	return menu
}

// RestoreWindow is shared by single-instance activation and tray actions. A
// hidden or minimised main window must be visible before focusing it.
func RestoreWindow(window application.Window) {
	window.Show()
	window.UnMinimise()
	window.Focus()
}

// Close stops accepting tray work and waits only for background operations.
// Native dialogs are dispatched asynchronously so the UI shutdown hook never
// waits for a worker that is itself waiting for the UI thread.
func (t *Tray) Close() {
	if t != nil {
		t.actions.close()
	}
}

type trayActions struct {
	service  *ApplicationService
	dispatch func(func())
	show     func()
	quit     func()
	dialog   func(title, message string, failed bool)
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	closed   bool
	last     <-chan struct{}
	workers  sync.WaitGroup
}

func newTrayActions(service *ApplicationService, dispatch func(func()), show, quit func(), dialog func(string, string, bool)) *trayActions {
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	close(ready)
	return &trayActions{service: service, dispatch: dispatch, show: show, quit: quit, dialog: dialog,
		ctx: ctx, cancel: cancel, last: ready}
}

func (t *trayActions) openWindow() {
	if t.ctx.Err() == nil {
		t.dispatch(func() {
			if t.ctx.Err() == nil {
				t.show()
			}
		})
	}
}

func (t *trayActions) requestQuit() {
	if t.ctx.Err() == nil {
		t.dispatch(func() {
			if t.ctx.Err() == nil {
				t.show()
				t.quit()
			}
		})
	}
}

func (t *trayActions) showStatus() {
	t.run(func(ctx context.Context) func() {
		preferences, err := t.service.GetPreferences(ctx)
		if err != nil {
			return t.failure(err)
		}
		overview, err := t.service.GetOverview(ctx)
		if err != nil {
			return t.failure(err)
		}
		state := "已暂停"
		if preferences.SchedulingEnabled {
			state = "已开启"
		}
		message := fmt.Sprintf("自动调度：%s\n已启用任务：%d\n未结束执行：%d\n\n暂停自动调度不影响手动执行，也不会停止已经开始的执行。",
			state, overview.EnabledTasks, overview.ActiveExecutions)
		return func() { t.dialog("dan-tick 运行状态", message, false) }
	})
}

func (t *trayActions) setSchedulingEnabled(enabled bool) {
	t.run(func(ctx context.Context) func() {
		if err := t.service.SetSchedulingEnabled(ctx, enabled); err != nil {
			return t.failure(err)
		}
		return nil
	})
}

func (t *trayActions) failure(err error) func() {
	message := "操作失败，请重试或查看诊断日志"
	var exposed publicError
	if errors.As(err, &exposed) {
		message = exposed.details.Message
	}
	return func() { t.dialog("dan-tick 操作失败", message, true) }
}

func (t *trayActions) run(work func(context.Context) func()) {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	previous := t.last
	done := make(chan struct{})
	t.last = done
	t.workers.Add(1)
	t.mu.Unlock()
	go func() {
		defer t.workers.Done()
		defer close(done)
		// Execute requests in their accepted order, including rapid pause /
		// resume requests, without blocking the native menu callback.
		select {
		case <-t.ctx.Done():
			return
		case <-previous:
		}
		if t.ctx.Err() != nil {
			return
		}
		if display := work(t.ctx); display != nil && t.ctx.Err() == nil {
			t.dispatch(func() {
				if t.ctx.Err() == nil {
					display()
				}
			})
		}
	}()
}

func (t *trayActions) close() {
	t.mu.Lock()
	t.closed = true
	t.cancel()
	t.mu.Unlock()
	t.workers.Wait()
}
