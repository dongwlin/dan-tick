package app

import (
	"fmt"
	"io/fs"
	"os"

	"github.com/dongwlin/dan-tick/internal/diagnostic"
)

// Run starts the desktop application with the embedded frontend assets and
// returns an exit code after recording safe startup and shutdown diagnostics.
func Run(assets fs.FS) int {
	return startWithRun(func(logger *diagnostic.Logger) error {
		return runDesktop(assets, logger)
	})
}

func startWithRun(runApplication func(*diagnostic.Logger) error) (code int) {
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
	if err := runApplication(logger); err != nil {
		logger.Record(diagnostic.AppFailed)
		// The underlying error may contain credentials or private paths.
		_, _ = fmt.Fprintln(os.Stderr, "dan-tick: application failed; see diagnostic logs for the failure stage")
		return 1
	}
	logger.Record(diagnostic.AppStopped)
	return 0
}
