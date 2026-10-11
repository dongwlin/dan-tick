package main

import (
	"context"
	"embed"
	"fmt"
	"os"
	"time"

	"github.com/dongwlin/dan-tick/internal/app"
	"github.com/dongwlin/dan-tick/internal/diagnostic"
	"github.com/dongwlin/dan-tick/internal/history"
	"github.com/dongwlin/dan-tick/internal/storage"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Wails uses Go's `embed` package to embed the frontend files into the binary.
// Any files in the frontend/dist folder will be embedded into the binary and
// made available to the frontend.
// See https://pkg.go.dev/embed for more information.

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	os.Exit(start())
}

func start() (code int) {
	logger := diagnostic.New(diagnostic.DefaultDir())
	defer logger.Close()
	defer func() {
		if recover() != nil {
			// Panics can contain private data; record the category, not the value.
			logger.Record(diagnostic.AppPanicked)
			_, _ = fmt.Fprintln(os.Stderr, "dan-tick: unexpected application failure; see diagnostic logs")
			code = 1
		}
	}()
	logger.Record(diagnostic.AppStarting)
	if err := run(logger); err != nil {
		logger.Record(diagnostic.AppFailed)
		// The underlying error may contain credentials or private paths.
		_, _ = fmt.Fprintln(os.Stderr, "dan-tick: application failed; see diagnostic logs for the failure stage")
		return 1
	}
	logger.Record(diagnostic.AppStopped)
	return 0
}

func run(logger *diagnostic.Logger) error {
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

	runtime := app.New(db, logger)
	defer runtime.Close() // Also handles startup or Wails.Run failures.
	if err := runtime.Start(context.Background()); err != nil {
		return fmt.Errorf("start scheduled work: %w", err)
	}

	// Start the desktop UI only after the persisted scheduler has recovered.
	// Business bindings will be registered here when their real use cases land.
	app := application.New(application.Options{
		Name:        "dan-tick",
		Description: "B 站直播弹幕定时发送桌面工具",
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	// Wails may terminate without returning from Run on some platforms.
	// Keep the business shutdown before database close in both exit paths.
	app.OnShutdown(runtime.Close)

	// The F3 window remains independent of the long-lived Go scheduler.
	app.Window.NewWithOptions(application.WebviewWindowOptions{
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

	// Run the application. This blocks until the application has been exited.
	err = app.Run()
	if err != nil {
		logger.Record(diagnostic.DesktopRunFailed)
		return fmt.Errorf("run application: %w", err)
	}
	return nil
}
