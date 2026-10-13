package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dongwlin/dan-tick/internal/desktop"
	"github.com/dongwlin/dan-tick/internal/diagnostic"
	"github.com/dongwlin/dan-tick/internal/history"
	"github.com/dongwlin/dan-tick/internal/settings"
	"github.com/dongwlin/dan-tick/internal/storage"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

func newDesktopApplication(logger *diagnostic.Logger, options application.Options) *application.App {
	var initialized atomic.Bool
	options.ErrorHandler = func(err error) {
		var fatal *application.FatalError
		if !errors.As(err, &fatal) {
			logger.Record(diagnostic.DesktopError)
			return
		}
		if initialized.Load() {
			logger.Record(diagnostic.DesktopRunFailed)
		} else {
			logger.Record(diagnostic.DesktopInitFailed)
		}
		// Wails exits after this handler returns, without running our defers.
		// Record the final category synchronously and never print the raw error.
		logger.Record(diagnostic.AppFailed)
		_, _ = fmt.Fprintln(os.Stderr, "dan-tick: desktop failed; see diagnostic logs for the failure stage")
	}
	options.PanicHandler = func(*application.PanicDetails) {
		// Wails recovers some callback goroutines itself. Preserve its default
		// fatal termination, but omit the panic value and stack from diagnostics.
		logger.Record(diagnostic.AppPanicked)
		_, _ = fmt.Fprintln(os.Stderr, "dan-tick: unexpected application failure; see diagnostic logs")
		os.Exit(1)
	}
	desktopApp := application.New(options)
	initialized.Store(true)
	return desktopApp
}

func runDesktop(assets fs.FS, logger *diagnostic.Logger) error {
	// Detect an unusable database directory before Wails initialization can
	// fail for unrelated desktop-session reasons. This does not open SQLite;
	// single-instance ownership is still acquired before database access.
	if err := os.MkdirAll(filepath.Dir(storage.DefaultPath()), 0700); err != nil {
		logger.Record(diagnostic.DatabaseOpenFailed)
		return fmt.Errorf("prepare application database directory: %w", err)
	}

	// Acquire Wails' OS-session single-instance ownership BEFORE opening SQLite
	// or recovering Cron. A second launch signals the existing window and exits.
	var windowMu sync.RWMutex
	var mainWindow application.Window
	var windowReady bool
	desktopApp := newDesktopApplication(logger, application.Options{
		Name:        "dan-tick",
		Description: "B 站直播弹幕定时发送桌面工具",
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "io.github.dongwlin.dan-tick",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				windowMu.RLock()
				window := mainWindow
				ready := windowReady
				windowMu.RUnlock()
				if ready && window != nil {
					desktop.RestoreWindow(window)
				}
			},
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	db, err := storage.Open(context.Background(), storage.DefaultPath())
	if err != nil {
		logger.Record(diagnostic.DatabaseOpenFailed)
		return fmt.Errorf("open application database: %w", err)
	}
	logger.Record(diagnostic.DatabaseReady)
	defer func() {
		if err := db.Close(); err != nil {
			logger.Record(diagnostic.DatabaseCloseFailed)
		}
	}()

	// Run retention once at startup and every 24 hours while the desktop app
	// remains open. Maintenance failure must not prevent tasks from running.
	cleanup := history.NewService(db)
	cleanupHistory := func(ctx context.Context) {
		if _, err := cleanup.CleanupExpired(ctx, time.Now()); err != nil && ctx.Err() == nil {
			logger.Record(diagnostic.HistoryCleanupFailed)
		}
	}
	cleanupHistory(context.Background())
	cleanupCtx, stopCleanup := context.WithCancel(context.Background())
	cleanupDone := make(chan struct{})
	go func() {
		defer close(cleanupDone)
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-cleanupCtx.Done():
				return
			case <-ticker.C:
				cleanupHistory(cleanupCtx)
			}
		}
	}()
	defer func() {
		stopCleanup()
		<-cleanupDone // stop all queries before closing SQLite
	}()

	runtime := New(db, logger)
	defer runtime.Close() // Also handles startup or Wails.Run failures.
	preferences := settings.NewService(db)
	closeBehavior, err := preferences.CloseBehavior(context.Background())
	if err != nil {
		return fmt.Errorf("load desktop preferences: %w", err)
	}
	desktopApp.RegisterService(application.NewServiceWithOptions(runtime.Config, application.ServiceOptions{
		MarshalError: desktop.MarshalError,
	}))
	applicationService := desktop.NewApplicationService(db, runtime.Scheduler, func(event string) { desktopApp.Event.Emit(event) })
	desktopApp.RegisterService(application.NewServiceWithOptions(
		applicationService,
		application.ServiceOptions{MarshalError: desktop.MarshalError},
	))
	version := "development"
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		version = info.Main.Version
	}
	desktopApp.RegisterService(application.NewServiceWithOptions(
		desktop.NewDiagnosticsService(logger, func(path string) error {
			return desktopApp.Env.OpenFileManager(path, false)
		}, func() (string, error) {
			return desktopApp.Dialog.SaveFile().SetFilename("dan-tick-diagnostics.zip").
				AddFilter("ZIP archive", "*.zip").PromptForSingleSelection()
		}, version),
		application.ServiceOptions{MarshalError: desktop.MarshalError},
	))
	desktopApp.RegisterService(application.NewServiceWithOptions(runtime.Operations, application.ServiceOptions{
		MarshalError: desktop.MarshalError,
	}))
	desktopApp.RegisterService(application.NewServiceWithOptions(
		desktop.NewHistoryService(history.NewService(db)),
		application.ServiceOptions{MarshalError: desktop.MarshalError},
	))
	windowLifecycle := desktop.NewWindowLifecycle(desktopApp, db, closeBehavior)
	desktopApp.RegisterService(application.NewServiceWithOptions(
		desktop.NewWindowService(preferences, windowLifecycle.SetCloseBehavior, windowLifecycle.RequestQuit),
		application.ServiceOptions{MarshalError: desktop.MarshalError},
	))
	runtime.BindEvents(func(event string) { desktopApp.Event.Emit(event) })
	if err := runtime.Start(context.Background()); err != nil {
		return fmt.Errorf("start scheduled work: %w", err)
	}
	// The F3 window remains independent of the long-lived Go scheduler.
	window := desktopApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "dan-tick",
		Width:     1280,
		Height:    760,
		MinWidth:  1080,
		MinHeight: 680,
		Frameless: true,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(6, 7, 15),
		URL:              "/",
	})
	windowLifecycle.Attach(window)
	tray := desktop.NewTray(desktopApp, window, applicationService, windowLifecycle.RequestQuit)
	defer tray.Close()
	// Wails may terminate without returning from Run on some platforms. Stop
	// tray queries before business shutdown and database close in both paths.
	desktopApp.OnShutdown(tray.Close)
	desktopApp.OnShutdown(runtime.Close)
	windowMu.Lock()
	mainWindow = window
	windowMu.Unlock()
	desktopApp.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		windowMu.Lock()
		windowReady = true
		windowMu.Unlock()
	})

	// Run the application. This blocks until the application has been exited.
	err = desktopApp.Run()
	if err != nil {
		logger.Record(diagnostic.DesktopRunFailed)
		return fmt.Errorf("run application: %w", err)
	}
	return nil
}
