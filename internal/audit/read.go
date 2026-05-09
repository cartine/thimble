package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ScannerBufferBytes is the per-line buffer size we install on the
// audit log scanner. Audit entries are tiny by design but we mirror
// the K-25 1 MiB ceiling Thimble uses elsewhere as a safety margin.
const ScannerBufferBytes = 1 << 20

// Read returns the audit events stored under storeRoot, in file
// order. A missing file is not an error; an empty slice is returned.
// Malformed lines are skipped silently — better to surface most of
// the log than to abort because of a single bad entry.
func Read(storeRoot string) ([]Event, error) {
	path := filepath.Join(storeRoot, LogFileName)
	// #nosec G304 -- path is constructed from the configured store
	// root and a fixed filename, not user input.
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open audit log: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), ScannerBufferBytes)
	var events []Event
	for scanner.Scan() {
		var ev Event
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			continue
		}
		events = append(events, ev)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan audit log: %w", err)
	}
	return events, nil
}

// Filter returns events restricted to the given (app, env). Either
// argument may be empty to skip that filter, but `thimble audit`
// always supplies both.
func Filter(events []Event, app, env string) []Event {
	out := make([]Event, 0, len(events))
	for _, e := range events {
		if app != "" && e.App != app {
			continue
		}
		if env != "" && e.Env != env {
			continue
		}
		out = append(out, e)
	}
	return out
}
