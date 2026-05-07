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
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: thimble web [--addr 127.0.0.1:8787]")
	}
	if *token == "" {
		if !isLoopbackAddr(*addr) {
			return errors.New("non-loopback web UI requires --token or THIMBLE_WEB_TOKEN")
		}
		generated, err := randomToken()
		if err != nil {
			return err
		}
		*token = generated
	}
	server := web.New(st, *token)
	mux := http.NewServeMux()
	server.Routes(mux)
	httpServer := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	fmt.Fprintf(stdout, "Thimble web UI: http://%s/?token=%s\n", *addr, *token)
	return httpServer.ListenAndServe()
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
