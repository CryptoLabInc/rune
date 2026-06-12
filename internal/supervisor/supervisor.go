package supervisor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

// Supervisor runtime parameters
type Config struct {
	RunedBinary string // default: ~/.runed/bin/runed
	RunedArgs   []string
	LogPath     string // default: ~/.runed/logs/daemon.log
	LockPath    string // default: ~/.runed/supervisor.lock

	BackoffSchedule []time.Duration // nil for DefaultBackoff
	MaxCrashes      int             // 0 for DefaultMaxCrashes
	MaxCrashWindow  time.Duration   // 0 for DefaultMaxCrashWindow
	ShutdownGrace   time.Duration   // 0 for DefaultShutdownGrace
}

var (
	DefaultBackoff = []time.Duration{
		2 * time.Second,
		5 * time.Second,
		10 * time.Second,
		15 * time.Second,
	}
	DefaultMaxCrashes     = 5
	DefaultMaxCrashWindow = 60 * time.Second
	DefaultShutdownGrace  = 5 * time.Second
)

const envMarker = "_RUNE_RUNED_SUPERVISOR" // supervisor mode marker

// `rune runed --detach` entrypoint
func RunDetached(ctx context.Context, cfg Config) error {
	if os.Getenv(envMarker) == "" {
		return spawnSupervisor(cfg)
	}
	return runSupervisor(ctx, cfg)
}

// spawnSupervisor re-execs the current binary with the marker env var
// set and SysProcAttr.Setsid = true. After cmd.Start() returns, we
// release the child handle so the kernel re-parents it to init when
// the current process exits.
func spawnSupervisor(cfg Config) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("supervisor: resolve executable path: %w", err)
	}
	args := []string{"runed", "--detach"}
	args = append(args, cfg.RunedArgs...)

	cmd := exec.Command(exe, args...)
	cmd.Env = append(os.Environ(), envMarker+"=1")
	applyDetachAttrs(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("supervisor: start detached: %w", err)
	}

	// Set userspace daemon's parent to init (PID 1)
	if err := cmd.Process.Release(); err != nil {
		return fmt.Errorf("supervisor: release child: %w", err)
	}
	return nil
}

// Deatched process body
func runSupervisor(ctx context.Context, cfg Config) error {
	if err := redirectStdio(cfg.LogPath); err != nil {
		return fmt.Errorf("supervisor: redirect stdio: %w", err)
	}

	lockFile, locked, err := acquireSupervisorLock(cfg.LockPath)
	if err != nil {
		return fmt.Errorf("supervisor: acquire lock: %w", err)
	}
	if !locked {
		fmt.Fprintf(os.Stderr, "supervisor: %s already held - another supervisor is running, exiting\n", cfg.LockPath)
		return nil
	}
	defer lockFile.Close()

	return runWatcher(ctx, cfg)
}

// runWatcher exec's RunedBinary in a loop, restarts on non-zero exit
// with backoff, gives up after MaxCrashes crashes in MaxCrashWindow,
// and forwards SIGINT/SIGTERM cleanly. Extracted from runSupervisor
// so it can be exercised in tests without going through the detach
// machinery.
func runWatcher(ctx context.Context, cfg Config) error {
	cfg = applyDefaults(cfg)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	var crashes []time.Time
	backoffIdx := 0

	recordCrash := func(now time.Time) (int, bool) {
		crashes = append(crashes, now)
		cutoff := now.Add(-cfg.MaxCrashWindow) // crash older than 'now - cfg.MaxCrashWindow' is considered expired
		i := 0

		for i < len(crashes) && crashes[i].Before(cutoff) {
			i++
		}
		crashes = crashes[i:] // sliding-window

		return len(crashes), len(crashes) >= cfg.MaxCrashes
	}

	// sleepBackoff waits the next backoff step; false means shutdown was
	// requested while waiting.
	sleepBackoff := func(crashCount int) bool {
		wait := cfg.BackoffSchedule[min(backoffIdx, len(cfg.BackoffSchedule)-1)]
		backoffIdx++

		fmt.Fprintf(os.Stderr, "supervisor: backing off %s before restart (crash %d of max %d in %s window)\n", wait, crashCount, cfg.MaxCrashes, cfg.MaxCrashWindow)

		select {
		case <-time.After(wait): // wait next backoff
			return true
		case <-ctx.Done(): // shutdown requested
			return false
		case <-sigCh: // shutdown requested
			return false
		}
	}

	for {
		cmd := exec.Command(cfg.RunedBinary, cfg.RunedArgs...)
		cmd.Stdin = nil
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		fmt.Fprintf(os.Stderr, "supervisor: starting %s %v\n", cfg.RunedBinary, cfg.RunedArgs)
		started := time.Now()

		// Share crash budget rather than end up supervision for retriable error
		if err := cmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "supervisor: start %s: %v\n", cfg.RunedBinary, err)

			count, giveUp := recordCrash(time.Now())
			if giveUp {
				return fmt.Errorf("supervisor: start %s: %w (%d failures within %s - giving up)", cfg.RunedBinary, err, count, cfg.MaxCrashWindow)
			}
			if !sleepBackoff(count) {
				return nil
			}

			continue
		}

		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()

		// Forward SIGINT/SIGTERM to child, detect child exited
		select {
		case <-ctx.Done():
			return shutdownChild(cmd, cfg.ShutdownGrace, done)
		case sig := <-sigCh:
			fmt.Fprintf(os.Stderr, "supervisor: received %s, forwarding to child\n", sig)
			return shutdownChild(cmd, cfg.ShutdownGrace, done)
		case err := <-done:
			exitCode := -1
			if cmd.ProcessState != nil {
				exitCode = cmd.ProcessState.ExitCode()
			}
			fmt.Fprintf(os.Stderr, "supervisor: child exited (code=%d err=%v)\n", exitCode, err)

			if exitCode == 0 {
				return nil // clean exit
			}

			// Reset backoff count if child live longer than crash window
			now := time.Now()
			if now.Sub(started) > cfg.MaxCrashWindow {
				backoffIdx = 0
			}

			count, giveUp := recordCrash(now)
			if giveUp {
				return fmt.Errorf("supervisor: %d crashes within %s - giving up", count, cfg.MaxCrashWindow)
			}
			if !sleepBackoff(count) {
				return nil
			}
		}
	}
}

func shutdownChild(cmd *exec.Cmd, grace time.Duration, done <-chan error) error {
	if cmd.Process == nil {
		return nil
	}

	_ = cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-done:
		return nil
	case <-time.After(grace):
		fmt.Fprintf(os.Stderr, "supervisor: child didn't exit within %s, sending SIGKILL\n", grace)

		_ = cmd.Process.Kill()

		// Also check if child still hasn't died for certain period
		select {
		case <-done:
		case <-time.After(grace):
			fmt.Fprintf(os.Stderr, "supervisor: child unresponsive to SIGKILL after %s, abandoning\n", grace)
		}

		return nil
	}
}

func applyDefaults(cfg Config) Config {
	if len(cfg.BackoffSchedule) == 0 {
		cfg.BackoffSchedule = DefaultBackoff
	}
	if cfg.MaxCrashes == 0 {
		cfg.MaxCrashes = DefaultMaxCrashes
	}
	if cfg.MaxCrashWindow == 0 {
		cfg.MaxCrashWindow = DefaultMaxCrashWindow
	}
	if cfg.ShutdownGrace == 0 {
		cfg.ShutdownGrace = DefaultShutdownGrace
	}
	return cfg
}

func EnsureLogDir(logPath string) error {
	return os.MkdirAll(filepath.Dir(logPath), 0o700)
}
