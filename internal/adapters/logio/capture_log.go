// Package logio handles ~/.rune/capture_log.jsonl (0600, append-only).
// Spec: docs/v04/spec/components/rune-mcp.md §Capture log.
// Python: mcp/server/server.py:L115-168 (_append_capture_log + _read_capture_log).
// Format: D20 bit-identical (same file may be written by Python and Go).
//
// Concurrency:
//   - Intra-process: sync.Mutex
//   - Inter-process: syscall.Flock(LOCK_EX) — Go-specific guard (Python uses
//     O_APPEND only; kernel atomic up to PIPE_BUF 4KB)
//
// Failure policy (D19): append failure → slog error, capture still succeeds.
package logio

import (
	"sync"

	"github.com/envector/rune-go/internal/domain"
)

// Path — ~/.rune/capture_log.jsonl.
// TODO: resolve via os.UserHomeDir at package init or on first Append.
const DefaultFilename = "capture_log.jsonl"

// CaptureLog — append handle.
type CaptureLog struct {
	path string
	mu   sync.Mutex
}

// New — TODO: resolve path + ensure file exists with 0600.
func New(path string) *CaptureLog {
	return &CaptureLog{path: path}
}

// Append — one JSONL line. Atomic (flock + O_APPEND + fsync).
// TODO:
//  1. sync.Mutex lock
//  2. os.OpenFile(O_APPEND | O_CREAT | O_WRONLY, 0600)
//  3. syscall.Flock(LOCK_EX)
//  4. json.Marshal(entry) + "\n" write + fsync
//  5. Flock(LOCK_UN) + close
//  6. Any error → return (caller logs + ignores per D19)
func (l *CaptureLog) Append(entry domain.CaptureLogEntry) error {
	// TODO: bit-identical to Python _append_capture_log + flock
	return nil
}

// Tail — reverse-read last N entries (used by tool_capture_history).
// Python: server.py:L140-168 _read_capture_log.
// Filters: domain (equality), since (ISO date lexicographic).
// TODO: implement reverse line reader + filter + limit cap (default 20, max 100).
func Tail(path string, limit int, domainFilter, since *string) ([]domain.CaptureLogEntry, error) {
	// TODO: port _read_capture_log
	return nil, nil
}
