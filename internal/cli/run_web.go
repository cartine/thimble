package cli

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/cartine/thimble/internal/store"
	"github.com/cartine/thimble/internal/web"
)

func runWeb(st *store.Store, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("web", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addr := fs.String("addr", defaultAddr, "listen address")
	token := fs.String("token", os.Getenv("THIMBLE_WEB_TOKEN"), "web UI token")
	var allowHosts stringList
	fs.Var(&allowHosts, "allow-host",
		"extra Host header to allow (repeatable; defaults cover loopback + --addr)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: thimble web [--addr 127.0.0.1:8787]")
	}
	loopback := isLoopbackAddr(*addr)
	if *token == "" {
		if !loopback {
			return errors.New("non-loopback web UI requires --token or THIMBLE_WEB_TOKEN")
		}
		generated, err := randomToken()
		if err != nil {
			return err
		}
		*token = generated
	}
	guard := buildHostGuard(*addr, allowHosts)
	server := web.New(st, *token, loopback)
	mux := http.NewServeMux()
	server.Routes(mux)
	handler := guard.Middleware(web.NoStoreMiddleware(mux))
	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	printWebBanner(stdout)
	fmt.Fprintf(stdout, "Thimble web UI: http://%s/\n", *addr)
	fmt.Fprintf(stdout, "Token: %s\n", *token)
	return httpServer.ListenAndServe()
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

// printWebBanner is K-35: a three-line warning that the web UI is
// scoped for single-operator local use. Printed before the URL so the
// banner cannot be missed when the URL is the visible affordance.
func printWebBanner(stdout io.Writer) {
	fmt.Fprintln(stdout, "Thimble web is a SINGLE-OPERATOR LOCAL TOOL.")
	fmt.Fprintln(stdout, "For shared/production workflows, use the CLI.")
	fmt.Fprintln(stdout, "Token is a session cookie; press Ctrl+C to stop.")
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
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
