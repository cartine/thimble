//go:build windows

package cli

import (
	"context"
	"io"

	"github.com/cartine/thimble/internal/web"
)

// watchManualRotate is a no-op on Windows: SIGUSR1 does not exist on
// that platform. Operators on Windows rely on idle rotation alone
// (--idle-rotate) and on stop-and-restart for an immediate manual
// rotation. The function still returns when ctx is canceled so the
// runWeb shutdown path stays uniform across OSes.
func watchManualRotate(ctx context.Context, _ *web.Server,
	_ io.Writer) {
	<-ctx.Done()
}
