package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestShouldAutoCheck(t *testing.T) {
	dir := t.TempDir()
	stamp := filepath.Join(dir, "last-update-check")
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	interval := 24 * time.Hour

	t.Run("missing stamp is due", func(t *testing.T) {
		if !ShouldAutoCheck(stamp, interval, now) {
			t.Error("a missing stamp must count as due")
		}
	})

	t.Run("fresh stamp is not due", func(t *testing.T) {
		if err := RecordAutoCheck(stamp, now.Add(-1*time.Hour)); err != nil {
			t.Fatal(err)
		}
		if ShouldAutoCheck(stamp, interval, now) {
			t.Error("a 1h-old stamp must not be due under a 24h interval")
		}
	})

	t.Run("stale stamp is due", func(t *testing.T) {
		if err := RecordAutoCheck(stamp, now.Add(-25*time.Hour)); err != nil {
			t.Fatal(err)
		}
		if !ShouldAutoCheck(stamp, interval, now) {
			t.Error("a 25h-old stamp must be due under a 24h interval")
		}
	})

	t.Run("corrupt stamp is due", func(t *testing.T) {
		if err := os.WriteFile(stamp, []byte("not-a-timestamp"), 0o600); err != nil {
			t.Fatal(err)
		}
		if !ShouldAutoCheck(stamp, interval, now) {
			t.Error("a corrupt stamp must fail toward due")
		}
	})
}

func TestAutoCheckInterval(t *testing.T) {
	t.Run("default when unset", func(t *testing.T) {
		t.Setenv(envAutoCheckInterval, "")
		if got := AutoCheckInterval(); got != defaultAutoCheckInterval {
			t.Errorf("interval = %s, want default %s", got, defaultAutoCheckInterval)
		}
	})

	t.Run("env override", func(t *testing.T) {
		t.Setenv(envAutoCheckInterval, "6h")
		if got := AutoCheckInterval(); got != 6*time.Hour {
			t.Errorf("interval = %s, want 6h", got)
		}
	})

	t.Run("invalid env falls back to default", func(t *testing.T) {
		t.Setenv(envAutoCheckInterval, "nonsense")
		if got := AutoCheckInterval(); got != defaultAutoCheckInterval {
			t.Errorf("interval = %s, want default on invalid env", got)
		}
	})
}

func TestAutoUpdateDisabled(t *testing.T) {
	t.Run("unset is not disabled", func(t *testing.T) {
		t.Setenv(envNoAutoUpdate, "")
		if AutoUpdateDisabled() {
			t.Error("unset RUNE_NO_AUTO_UPDATE must not disabled")
		}
	})

	t.Run("set opts out", func(t *testing.T) {
		t.Setenv(envNoAutoUpdate, "1")
		if !AutoUpdateDisabled() {
			t.Error("set RUNE_NO_AUTO_UPDATE must disabled")
		}
	})
}
