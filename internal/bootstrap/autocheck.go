package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultAutoCheckInterval = 24 * time.Hour
	envAutoCheckInterval     = "RUNE_UPDATE_CHECK_INTERVAL"
	envNoAutoUpdate          = "RUNE_NO_AUTO_UPDATE"
)

func AutoUpdateDisabled() bool {
	return os.Getenv(envNoAutoUpdate) != ""
}

func AutoCheckInterval() time.Duration {
	if v := strings.TrimSpace(os.Getenv(envAutoCheckInterval)); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}

	return defaultAutoCheckInterval
}

func ShouldAutoCheck(stampPath string, interval time.Duration, now time.Time) bool {
	data, err := os.ReadFile(stampPath)
	if err != nil {
		return true
	}

	last, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	if err != nil {
		return true
	}

	return now.Sub(last) >= interval
}

func RecordAutoCheck(stampPath string, now time.Time) error {
	if err := os.MkdirAll(filepath.Dir(stampPath), 0o700); err != nil {
		return err
	}

	// Stamp last check time
	return os.WriteFile(stampPath, []byte(now.UTC().Format(time.RFC3339)), 0o600)
}
