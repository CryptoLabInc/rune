package lifecycle

// boot_log.go — append-only JSON log of boot failures, one object per line.
// Best-effort: open/write errors never propagate to the boot loop. detail is
// redacted via obs.Redact before writing.

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/envector/rune-go/internal/domain"
	"github.com/envector/rune-go/internal/obs"
)

const DefaultBootLogMaxBytes int64 = 1 << 20 // 1 MiB

// BootLogger owns the on-disk boot-failure log. A nil *BootLogger is a safe
// no-op so callers (and tests) can leave it unset.
type BootLogger struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	file     *os.File
	resolved bool // tried to open at least once; gates repeated open-error logs
}

// NewBootLogger appends to path, rotating at maxBytes (default when <= 0).
// The file opens lazily on first Persist.
func NewBootLogger(path string, maxBytes int64) *BootLogger {
	if maxBytes <= 0 {
		maxBytes = DefaultBootLogMaxBytes
	}
	return &BootLogger{path: path, maxBytes: maxBytes}
}

// bootLogEntry is the pinned JSON shape written per line.
type bootLogEntry struct {
	Time     time.Time            `json:"time"`
	Kind     domain.BootErrorKind `json:"kind"`
	Phase    domain.BootPhase     `json:"phase,omitempty"`
	Detail   string               `json:"detail"`
	Hint     string               `json:"hint,omitempty"`
	Attempts int                  `json:"attempts,omitempty"`
}

// Persist appends one JSON line. No-op when bl or be is nil. Open failures are
// logged once then silent so the writer can't break the boot loop.
func (bl *BootLogger) Persist(be *domain.BootError) {
	if bl == nil || be == nil {
		return
	}
	bl.mu.Lock()
	defer bl.mu.Unlock()

	if bl.file == nil {
		if err := bl.openLocked(); err != nil {
			if !bl.resolved {
				slog.Warn("boot_log: file open failed — boot errors will not be persisted",
					"err", err, "path", bl.path)
			}
			bl.resolved = true
			return
		}
	}

	entry := bootLogEntry{
		Time:     be.At,
		Kind:     be.Kind,
		Phase:    be.Phase,
		Detail:   obs.Redact(be.Detail),
		Hint:     be.Hint, // hint never contains secrets by construction
		Attempts: be.Attempts,
	}
	line, err := json.Marshal(entry)
	if err != nil {
		slog.Warn("boot_log: marshal failed", "err", err)
		return
	}
	line = append(line, '\n')

	bl.rotateIfNeededLocked(int64(len(line)))
	if bl.file == nil {
		return // rotation failed to reopen; openLocked already logged
	}
	if _, err := bl.file.Write(line); err != nil {
		slog.Warn("boot_log: write failed", "err", err)
	}
}

// Close is idempotent and nil-safe; satisfies the Closer interface.
func (bl *BootLogger) Close() error {
	if bl == nil {
		return nil
	}
	bl.mu.Lock()
	defer bl.mu.Unlock()
	if bl.file != nil {
		err := bl.file.Close()
		bl.file = nil
		return err
	}
	return nil
}

func (bl *BootLogger) Path() string {
	if bl == nil {
		return ""
	}
	return bl.path
}

// openLocked opens the log O_APPEND with 0600 (owner-only; detail may carry
// internal endpoints). Caller must hold bl.mu.
func (bl *BootLogger) openLocked() error {
	dir := filepath.Dir(bl.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(bl.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	bl.file = f
	return nil
}

// rotateIfNeededLocked rotates to "<path>.1" (one generation kept) when the
// next write would exceed maxBytes, then reopens a fresh file. Caller must
// hold bl.mu.
func (bl *BootLogger) rotateIfNeededLocked(incoming int64) {
	info, err := bl.file.Stat()
	if err != nil {
		return
	}
	if info.Size()+incoming <= bl.maxBytes {
		return
	}
	_ = bl.file.Close()
	bl.file = nil
	if err := os.Rename(bl.path, bl.path+".1"); err != nil {
		slog.Warn("boot_log: rotate rename failed", "err", err, "path", bl.path)
		// reopen original anyway so we don't lose new writes
	}
	if err := bl.openLocked(); err != nil {
		if !bl.resolved {
			slog.Warn("boot_log: reopen after rotate failed", "err", err, "path", bl.path)
		}
		bl.resolved = true
	}
}
