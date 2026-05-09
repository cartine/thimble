//go:build !windows

package cli

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/cartine/thimble/internal/web"
)

// watchManualRotate listens for SIGUSR1 and rotates the web token
// each time it lands. Exits cleanly when ctx is canceled (Ctrl+C
// or SIGTERM), so the runWeb goroutine pool does not leak. SIGUSR1
// is the operator's manual escape hatch when they suspect the token
// has leaked but don't want to interrupt in-flight work.
func watchManualRotate(ctx context.Context, server *web.Server,
	stdout io.Writer) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGUSR1)
	defer signal.Stop(ch)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ch:
			_ = server.Rotate(stdout)
		}
	}
}
