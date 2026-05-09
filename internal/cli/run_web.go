package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cartine/thimble/internal/store"
	"github.com/cartine/thimble/internal/web"
)

// defaultIdleRotate is the default value of --idle-rotate. Aligned
// with the K-33 spec ("15m default"); operators who want to disable
// idle rotation pass --idle-rotate=0.
const defaultIdleRotate = 15 * time.Minute

func runWeb(st *store.Store, args []string, stdout, stderr io.Writer) error {
	cfg, err := parseWebFlags(args, stderr)
	if err != nil {
		return err
	}
	loopback := isLoopbackAddr(cfg.addr)
	if cfg.token == "" {
		if !loopback {
			return errors.New("non-loopback web UI requires --token or THIMBLE_WEB_TOKEN")
		}
		generated, err := web.RandomToken()
		if err != nil {
			return err
		}
		cfg.token = generated
	}
	guard := buildHostGuard(cfg.addr, cfg.allowHosts)
	server := web.New(st, cfg.token, loopback)
	mux := http.NewServeMux()
	server.Routes(mux)
	handler := guard.Middleware(web.NoStoreMiddleware(mux))
	httpServer := &http.Server{
		Addr:              cfg.addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	printWebBanner(stdout, cfg.idleRotate)
	fmt.Fprintf(stdout, "Thimble web UI: http://%s/\n", cfg.addr)
	fmt.Fprintf(stdout, "Token: %s\n", cfg.token)
	return runWebServer(server, httpServer, cfg.idleRotate, stdout)
}

// runWebServer wires the rotation goroutines (idle timer + SIGUSR1
// listener) and runs the HTTP server until ctx is canceled (Ctrl+C
// or SIGTERM). Both rotation goroutines exit cleanly on ctx.Done(),
// so there is no leak on shutdown.
func runWebServer(server *web.Server, httpServer *http.Server,
	idleRotate time.Duration, stdout io.Writer) error {
	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		_ = server.RunIdleRotation(ctx, idleRotate, stdout)
	}()
	go watchManualRotate(ctx, server, stdout)
	go func() {
		<-ctx.Done()
		_ = httpServer.Shutdown(context.Background())
	}()
	if err := httpServer.ListenAndServe(); err != nil &&
		!errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// webConfig is the parsed --addr/--token/--allow-host/--idle-rotate
// bundle, separated so runWeb stays under the function-length limit.
type webConfig struct {
	addr       string
	token      string
	allowHosts []string
	idleRotate time.Duration
}

func parseWebFlags(args []string, stderr io.Writer) (webConfig, error) {
	fs := flag.NewFlagSet("web", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addr := fs.String("addr", defaultAddr, "listen address")
	token := fs.String("token", os.Getenv("THIMBLE_WEB_TOKEN"), "web UI token")
	idleRotate := fs.Duration("idle-rotate", defaultIdleRotate,
		"rotate the web token after this much idle time (0 disables)")
	var allowHosts stringList
	fs.Var(&allowHosts, "allow-host",
		"extra Host header to allow (repeatable; defaults cover loopback + --addr)")
	if err := fs.Parse(args); err != nil {
		return webConfig{}, err
	}
	if fs.NArg() != 0 {
		return webConfig{}, errors.New(
			"usage: thimble web [--addr 127.0.0.1:8787] [--idle-rotate 15m]")
	}
	return webConfig{
		addr:       *addr,
		token:      *token,
		allowHosts: allowHosts,
		idleRotate: *idleRotate,
	}, nil
}

// buildHostGuard composes the canonical loopback allowlist with the
// configured listen address and any --allow-host flags. The bind host
// is added explicitly so non-loopback addresses still match.
func buildHostGuard(addr string, extra []string) *web.HostGuard {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		host, port = addr, ""
	}
	authorities := web.LoopbackAuthorities(port)
	if host != "" {
		authorities = append(authorities, host)
		if port != "" {
			authorities = append(authorities, net.JoinHostPort(host, port))
		}
	}
	authorities = append(authorities, extra...)
	return web.NewHostGuard(authorities)
}

// stringList is a flag.Value that accumulates repeated --allow-host
// values into a slice without losing order.
type stringList []string

// String renders the accumulator for flag --help output.
func (s *stringList) String() string {
	if s == nil {
		return ""
	}
	return fmt.Sprint(*s)
}

// Set appends one --allow-host value.
func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// printWebBanner is K-35 + K-33: a three-line warning that the web UI
// is scoped for single-operator local use, plus the rotation hint on
// the third line. Printed before the URL so the banner cannot be
// missed when the URL is the visible affordance.
func printWebBanner(stdout io.Writer, idleRotate time.Duration) {
	fmt.Fprintln(stdout, "Thimble web is a SINGLE-OPERATOR LOCAL TOOL.")
	fmt.Fprintln(stdout, "For shared/production workflows, use the CLI.")
	fmt.Fprintf(stdout,
		"Token rotates after %s; SIGUSR1 to rotate sooner; Ctrl+C to stop.\n",
		idleRotate)
}

func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
