package embedder

import "time"

// Overrides infoCache retry cooldown for tests and returns restore function
func SetInfoRetryCooldown(d time.Duration) (restore func()) {
	prev := infoRetryCooldown
	infoRetryCooldown = d
	return func() { infoRetryCooldown = prev }
}
