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
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"syscall"

	"github.com/envector/rune-go/internal/domain"
)

// Path — ~/.rune/capture_log.jsonl.
const DefaultFilename = "capture_log.jsonl"

// CaptureLog — append handle.
type CaptureLog struct {
	path string
	mu   sync.Mutex
}

func New(path string) *CaptureLog {
	return &CaptureLog{path: path}
}

// Append — one JSONL line. Atomic (flock + O_APPEND + fsync).
//
// Steps:
//  1. sync.Mutex lock
//  2. os.OpenFile(O_APPEND | O_CREAT | O_WRONLY, 0600)
//  3. syscall.Flock(LOCK_EX) (inter-process)
//  4. json.Marshal(entry) + "\n" write + fsync
//  5. Flock(LOCK_UN) + close
//  6. Any error: log + return
func (l *CaptureLog) Append(entry domain.CaptureLogEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	f, err := os.OpenFile(l.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		slog.Error("capture log: open failed", "path", l.path, "err", err)
		return fmt.Errorf("capture log open: %w", err)
	}
	defer f.Close()

	// Inter-process lock
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		slog.Error("capture log: flock failed", "err", err)
		return fmt.Errorf("capture log flock: %w", err)
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()

	data, err := json.Marshal(entry)
	if err != nil {
		slog.Error("capture log: marshal failed", "err", err)
		return fmt.Errorf("capture log marshal: %w", err)
	}

	data = append(data, '\n')
	if _, err := f.Write(data); err != nil {
		slog.Error("capture log: write failed", "err", err)
		return fmt.Errorf("capture log write: %w", err)
	}

	if err := f.Sync(); err != nil {
		slog.Error("capture log: fsync failed", "err", err)
		return fmt.Errorf("capture log fsync: %w", err)
	}

	return nil
}

// Tail — reverse-read last N entries (used by tool_capture_history).
// Python: server.py:L140-168 _read_capture_log.
// Filters: domain (equality), since (ISO date lexicographic).
func Tail(path string, limit int, domainFilter, since *string) ([]domain.CaptureLogEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("capture log open: %w", err)
	}
	defer f.Close()

	// Read all lines
	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024) // allow up to 256KB per line
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("capture log scan: %w", err)
	}

	// Reverse iterate
	var entries []domain.CaptureLogEntry
	for i := len(lines) - 1; i >= 0 && len(entries) < limit; i-- {
		var entry domain.CaptureLogEntry
		if err := json.Unmarshal([]byte(lines[i]), &entry); err != nil {
			continue // skip malformed lines
		}

		if domainFilter != nil && entry.Domain != *domainFilter {
			continue
		}

		if since != nil && entry.TS < *since {
			continue
		}

		entries = append(entries, entry)
	}

	return entries, nil
}
