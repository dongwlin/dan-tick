package desktop

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type trayDialog struct {
	title   string
	message string
	failed  bool
}

func testTrayActions(t *testing.T, service *ApplicationService) (*trayActions, <-chan trayDialog) {
	t.Helper()
	dialogs := make(chan trayDialog, 8)
	actions := newTrayActions(service, func(fn func()) { fn() }, func() {}, func() {},
		func(title, message string, failed bool) {
			dialogs <- trayDialog{title, message, failed}
		})
	t.Cleanup(actions.close)
	return actions, dialogs
}

func receiveTrayDialog(t *testing.T, dialogs <-chan trayDialog) trayDialog {
	t.Helper()
	select {
	case dialog := <-dialogs:
		return dialog
	case <-time.After(2 * time.Second):
		t.Fatal("tray did not display an operation result")
		return trayDialog{}
	}
}

func TestTraySchedulingActionsUsePersistedStateInRequestOrder(t *testing.T) {
	ctx := context.Background()
	config, manager, db := configurationTest(t)
	room, err := config.AddRoom(ctx, AddRoomInput{Value: "12345"})
	require.NoError(t, err)
	task, err := config.CreateTask(ctx, taskRequest(room.ID))
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO executions
		(task_id, trigger_kind, status, created_at, sent_count, failed_count, task_name_snapshot, platform_room_id_snapshot)
		VALUES (?, 'manual', 'running', ?, 0, 0, 'private execution title', '98765')`, task.ID, time.Now().UnixMilli())
	require.NoError(t, err)

	states := make(chan bool, 4)
	errors := make(chan error, 4)
	var service *ApplicationService
	service = NewApplicationService(db, manager, func(string) {
		preferences, err := service.GetPreferences(ctx)
		if err != nil {
			errors <- err
			return
		}
		states <- preferences.SchedulingEnabled
	})
	actions, dialogs := testTrayActions(t, service)
	// Rapid alternating requests must leave the last accepted state persisted.
	actions.setSchedulingEnabled(false)
	actions.setSchedulingEnabled(true)
	actions.setSchedulingEnabled(false)
	actions.showStatus()
	dialog := receiveTrayDialog(t, dialogs)
	require.False(t, dialog.failed)
	require.Contains(t, dialog.message, "自动调度：已暂停")
	require.Contains(t, dialog.message, "已启用任务：1")
	require.Contains(t, dialog.message, "未结束执行：1")
	require.NotContains(t, dialog.message, "private")
	require.Empty(t, errors)
	require.Len(t, states, 3)
	require.Equal(t, []bool{false, true, false}, []bool{<-states, <-states, <-states})
	// Neither pausing nor resuming changes task enablement or active execution.
	preferences, err := NewApplicationService(db, manager, nil).GetPreferences(ctx)
	require.NoError(t, err)
	require.False(t, preferences.SchedulingEnabled)
	var taskEnabled bool
	require.NoError(t, db.QueryRowContext(ctx, `SELECT enabled FROM tasks WHERE id = ?`, task.ID).Scan(&taskEnabled))
	require.True(t, taskEnabled)
	var executionStatus string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT status FROM executions WHERE task_id = ?`, task.ID).Scan(&executionStatus))
	require.Equal(t, "running", executionStatus)

	// A frontend change is immediately reflected on the next status request,
	// without a tray cache or an event listener.
	require.NoError(t, service.SetSchedulingEnabled(ctx, true))
	actions.showStatus()
	dialog = receiveTrayDialog(t, dialogs)
	require.Contains(t, dialog.message, "自动调度：已开启")
}

func TestTraySchedulingFailureDisplaysSafeError(t *testing.T) {
	_, manager, db := configurationTest(t)
	service := NewApplicationService(db, manager, nil)
	require.NoError(t, service.SetSchedulingEnabled(context.Background(), true))
	_, err := db.ExecContext(context.Background(), `CREATE TRIGGER deny_tray_setting BEFORE UPDATE ON app_settings
		BEGIN SELECT RAISE(ABORT, 'private cookie and database detail'); END`)
	require.NoError(t, err)
	actions, dialogs := testTrayActions(t, service)
	actions.setSchedulingEnabled(false)
	dialog := receiveTrayDialog(t, dialogs)
	require.True(t, dialog.failed)
	require.Equal(t, "操作失败，请重试或查看诊断日志", dialog.message)
	preferences, err := NewApplicationService(db, manager, nil).GetPreferences(context.Background())
	require.NoError(t, err)
	require.True(t, preferences.SchedulingEnabled)
}

func TestTrayStatusQueryFailureIsNotPresentedAsZeroCounts(t *testing.T) {
	_, manager, db := configurationTest(t)
	_, err := db.ExecContext(context.Background(), `DROP TABLE tasks`)
	require.NoError(t, err)
	actions, dialogs := testTrayActions(t, NewApplicationService(db, manager, nil))
	actions.showStatus()
	dialog := receiveTrayDialog(t, dialogs)
	require.True(t, dialog.failed)
	require.Equal(t, "操作失败，请重试或查看诊断日志", dialog.message)
	require.NotContains(t, dialog.message, "已启用任务")
}

func TestTrayCloseDropsAlreadyDispatchedDialog(t *testing.T) {
	_, manager, db := configurationTest(t)
	queued := make(chan func(), 1)
	dialogs := make(chan trayDialog, 1)
	actions := newTrayActions(NewApplicationService(db, manager, nil), func(fn func()) { queued <- fn },
		func() {}, func() {}, func(title, message string, failed bool) {
			dialogs <- trayDialog{title, message, failed}
		})
	t.Cleanup(actions.close)
	actions.showStatus()
	var display func()
	select {
	case display = <-queued:
	case <-time.After(2 * time.Second):
		t.Fatal("tray did not dispatch the status dialog")
	}
	// Native shutdown can run before a queued UI callback. Closing must finish
	// without that callback being invoked, and the callback must then be inert.
	actions.close()
	display()
	require.Empty(t, dialogs)
}

func TestTrayWindowActionsRunOnUIAndDropAfterClose(t *testing.T) {
	var pending []func()
	var windowActions []string
	actions := newTrayActions(nil, func(fn func()) { pending = append(pending, fn) },
		func() { windowActions = append(windowActions, "show") },
		func() { windowActions = append(windowActions, "quit") },
		func(string, string, bool) { t.Error("window actions displayed a dialog") })
	t.Cleanup(actions.close)
	actions.openWindow()
	actions.requestQuit()
	require.Empty(t, windowActions)
	require.Len(t, pending, 2)
	for _, callback := range pending {
		callback()
	}
	require.Equal(t, []string{"show", "show", "quit"}, windowActions)

	pending = nil
	actions.openWindow()
	actions.requestQuit()
	require.Len(t, pending, 2)
	actions.close()
	for _, callback := range pending {
		callback()
	}
	require.Equal(t, []string{"show", "show", "quit"}, windowActions)
}

func TestTrayClickAttributionClassifiesWailsTrayEntryPoints(t *testing.T) {
	wails := "github.com/wailsapp/wails/v3/pkg/application."
	tests := []struct {
		name   string
		frames []runtime.Frame
		want   trayClick
	}{
		{"item activation", []runtime.Frame{{Function: wails + "(*linuxSystemTray).Activate"}}, trayClickActivate},
		{"menu notification", []runtime.Frame{{Function: wails + "(*linuxSystemTray).Event"}}, trayClickMenuOpen},
		{"menu event group", []runtime.Frame{{Function: wails + "(*linuxSystemTray).EventGroup"}}, trayClickOther},
		{"renamed receiver", []runtime.Frame{{Function: wails + "(*linuxTray).Activate"}}, trayClickOther},
		{"caller outside wails", []runtime.Frame{{Function: "github.com/dongwlin/dan-tick/internal/desktop.(*trayActions).openWindowOnActivate"}}, trayClickOther},
		{"empty stack", nil, trayClickOther},
		{"innermost caller wins", []runtime.Frame{
			{Function: "github.com/dongwlin/dan-tick/internal/desktop.(*linuxSystemTray).Event"},
			{Function: wails + "(*linuxSystemTray).Activate"},
		}, trayClickMenuOpen},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, classifyTrayClick(test.frames))
		})
	}
}

// linuxSystemTray mirrors the receiver name in the Wails Linux tray type that
// the click attribution matches, so these methods produce the same stack
// frames as the dependency does.
type linuxSystemTray struct {
	actions *trayActions
}

func (s *linuxSystemTray) Activate() { s.actions.openWindowOnActivate() }

func (s *linuxSystemTray) Event() { s.actions.openWindowOnActivate() }

func TestTrayWindowRestoresOnActivationAndNotOnMenuOpening(t *testing.T) {
	var pending []func()
	var windowActions []string
	actions := newTrayActions(nil, func(fn func()) { pending = append(pending, fn) },
		func() { windowActions = append(windowActions, "show") },
		func() { t.Error("tray activation quit the application") },
		func(string, string, bool) { t.Error("tray activation displayed a dialog") })
	t.Cleanup(actions.close)
	tray := &linuxSystemTray{actions: actions}

	// Hosts report the menu opening through the same Wails callback: a single
	// or right click must leave the window alone.
	tray.Event()
	require.Empty(t, pending)
	// Activation, the double click in the GNOME AppIndicator extension,
	// restores the window.
	tray.Activate()
	require.Len(t, pending, 1)
	// A caller this build cannot attribute fails closed instead of restoring
	// the window on menu clicks.
	actions.openWindowOnActivate()
	require.Len(t, pending, 1)

	for _, callback := range pending {
		callback()
	}
	require.Equal(t, []string{"show"}, windowActions)
}

func TestTrayCloseCancelsBackgroundWorkAndRejectsNewActions(t *testing.T) {
	var windowActions []string
	actions := newTrayActions(nil, func(fn func()) { fn() },
		func() { windowActions = append(windowActions, "show") },
		func() { windowActions = append(windowActions, "quit") },
		func(string, string, bool) { t.Error("cancelled work displayed a dialog") })
	t.Cleanup(actions.close)
	actions.requestQuit()
	require.Equal(t, []string{"show", "quit"}, windowActions)

	started := make(chan struct{})
	actions.run(func(ctx context.Context) func() {
		close(started)
		<-ctx.Done()
		return func() { t.Error("cancelled work reached the UI") }
	})
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("tray did not start its background work")
	}
	closed := make(chan struct{})
	go func() {
		actions.close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("tray close did not cancel its background work")
	}
	actions.openWindow()
	actions.requestQuit()
	actions.run(func(context.Context) func() {
		t.Error("closed tray accepted more background work")
		return nil
	})
	actions.close()
	require.Equal(t, []string{"show", "quit"}, windowActions)
}
