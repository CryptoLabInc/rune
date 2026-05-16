package lifecycle

// boot_log.go — append-only JSON log of boot failures to ~/.rune/logs/boot.log.
// Complementary to lifecycle.Manager.LastBootError() (in-memory, latest only)
// and slog (stderr / handler chain). The on-disk log is the fallback for
// cases the structured classifier didn't fully describe — humans / admins
// can grep across attempts.
//
// Design:
//   - One JSON object per line ({ "time": ..., "kind": ..., ... })
//   - Append-only — never truncates. Log rotation deferred (manual or
//     external logrotate); typical session writes a few dozen lines max.
//   - Best-effort: open/write errors do NOT propagate (logging must not
//     take down the boot loop).
//   - Sensitive substrings in `detail` are redacted via obs.Redact before
//     writing — same patterns the slog handler uses.

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

// bootLogState — package-scope state for the on-disk boot log. Guarded by mu
// because the boot loop (one goroutine) and graceful shutdown (another) may
// touch the file concurrently.
var bootLog = struct {
	mu   sync.Mutex
	file *os.File
	path string
	// resolved tracks whether we've tried to open at least once — failures
	// after that switch to silent (avoid spamming slog with the same error).
	resolved bool
}{}

// bootLogEntry is the JSON shape written per line. Pinned schema so admins
// can write scripts against it without worrying about field renames.
type bootLogEntry struct {
	Time     time.Time            `json:"time"`
	Kind     domain.BootErrorKind `json:"kind"`
	Phase    domain.BootPhase     `json:"phase,omitempty"`
	Detail   string               `json:"detail"`
	Hint     string               `json:"hint,omitempty"`
	Attempts int                  `json:"attempts,omitempty"`
}

// PersistBootError appends a JSON line describing the boot error to
// ~/.rune/logs/boot.log. No-op when be is nil. Failures are swallowed
// (logged once via slog, then silent) so the log writer can't break the
// boot loop.
func PersistBootError(be *domain.BootError) {
	if be == nil {
		return
	}
	bootLog.mu.Lock()
	defer bootLog.mu.Unlock()

	if bootLog.file == nil {
		if err := openBootLogLocked(); err != nil {
			if !bootLog.resolved {
				slog.Warn("boot_log: file open failed — boot errors will not be persisted",
					"err", err, "path", bootLog.path)
			}
			bootLog.resolved = true
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
		// json.Marshal failing on these stdlib types should be impossible,
		// but be defensive.
		slog.Warn("boot_log: marshal failed", "err", err)
		return
	}
	line = append(line, '\n')
	if _, err := bootLog.file.Write(line); err != nil {
		slog.Warn("boot_log: write failed", "err", err)
	}
}

// CloseBootLog closes the underlying file. Called from graceful shutdown
// so the OS flushes the buffer. Idempotent.
func CloseBootLog() {
	bootLog.mu.Lock()
	defer bootLog.mu.Unlock()
	if bootLog.file != nil {
		_ = bootLog.file.Close()
		bootLog.file = nil
	}
}

// BootLogPath returns the on-disk path the boot log writes to (resolved once
// per process). Exposed for diagnostics tools that want to surface the path
// to the user.
func BootLogPath() string {
	bootLog.mu.Lock()
	defer bootLog.mu.Unlock()
	if bootLog.path == "" {
		_ = resolveBootLogPathLocked()
	}
	return bootLog.path
}

// resolveBootLogPathLocked computes ~/.rune/logs/boot.log and stores it
// in bootLog.path. Caller must hold bootLog.mu.
func resolveBootLogPathLocked() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	bootLog.path = filepath.Join(home, ".rune", "logs", "boot.log")
	return nil
}

// openBootLogLocked mkdir -p the logs directory + opens the log file in
// O_APPEND mode with 0600 perms (file is owner-only since detail may contain
// internal endpoints / error strings). Caller must hold bootLog.mu.
func openBootLogLocked() error {
	if bootLog.path == "" {
		if err := resolveBootLogPathLocked(); err != nil {
			return err
		}
	}
	dir := filepath.Dir(bootLog.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(bootLog.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	bootLog.file = f
	return nil
}
