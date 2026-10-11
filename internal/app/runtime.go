// Package app owns the single shared desktop business runtime and its lifecycle.
package app

import (
	"context"
	"net/http"
	"sync"

	"github.com/dongwlin/dan-tick/internal/credential"
	"github.com/dongwlin/dan-tick/internal/danmaku"
	"github.com/dongwlin/dan-tick/internal/diagnostic"
	"github.com/dongwlin/dan-tick/internal/executor"
	"github.com/dongwlin/dan-tick/internal/scheduler"
	"github.com/uptrace/bun"
)

// Runtime exposes the long-lived business services to later Wails bindings.
// All send paths must use Sender: a second instance would bypass account limits.
type Runtime struct {
	Sender    *danmaku.Sender
	Executor  *executor.Service
	Scheduler *scheduler.Manager

	stopOnce sync.Once
}

func New(db *bun.DB, log *diagnostic.Logger) *Runtime {
	return newRuntime(db, log, danmaku.NewClient(credential.NewStore(), &http.Client{}))
}

func newRuntime(db *bun.DB, log *diagnostic.Logger, client *danmaku.Client) *Runtime {
	sender := danmaku.NewSender(client)
	runner := executor.NewService(db, sender, log)
	manager := scheduler.NewManager(db, log, func(ctx context.Context, claimed scheduler.ClaimResult) error {
		return runner.StartScheduled(ctx, claimed.ExecutionID, nil)
	}, func(error) {
		// Scheduler errors may contain private database data; do not log their text.
		if log != nil {
			log.Record(diagnostic.ScheduleSyncFailed)
		}
	})
	return &Runtime{Sender: sender, Executor: runner, Scheduler: manager}
}

// Start performs transactional startup recovery before registering any Cron
// wakeup. Failure leaves the caller responsible for Close, not a half-running
// scheduler. The scheduler is independent of Wails views and window lifetime.
func (r *Runtime) Start(ctx context.Context) error {
	return r.Scheduler.Start(ctx)
}

// Close is safe after startup failure, normal application shutdown, or both.
// Stop Cron callbacks first; then cancel and join every accepted send before
// the caller closes SQLite. No new send is admitted after executor shutdown.
func (r *Runtime) Close() {
	r.stopOnce.Do(func() {
		<-r.Scheduler.Stop().Done()
		_ = r.Executor.Shutdown(context.Background())
	})
}
