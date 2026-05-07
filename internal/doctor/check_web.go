package doctor

import (
	"fmt"
	"net"
)

// defaultWebAddr is the bind address Thimble's web UI uses by
// default; doctor's web check probes it.
const defaultWebAddr = "127.0.0.1:8787"

// checkWebPort attempts a non-blocking TCP listen on opts.WebAddr
// (or the default 127.0.0.1:8787). If the bind succeeds, the port
// is available and the result is OK — the listener is closed
// immediately so doctor leaves no listening socket behind.
// If the bind fails, the result is a warn with the standard
// suggestion to use --addr 127.0.0.1:8788.
func checkWebPort(opts Options) CheckResult {
	addr := opts.WebAddr
	if addr == "" {
		addr = defaultWebAddr
	}
	r := CheckResult{Name: "web port"}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		r.Status = StatusWarn
		r.Detail = fmt.Sprintf(
			"%s in use; thimble web --addr 127.0.0.1:8788 if you need a "+
				"different port", addr,
		)
		return r
	}
	if cerr := listener.Close(); cerr != nil {
		r.Status = StatusWarn
		r.Detail = fmt.Sprintf("%s probe close failed: %v", addr, cerr)
		return r
	}
	r.Status = StatusOK
	r.Detail = fmt.Sprintf("%s available", addr)
	return r
}
