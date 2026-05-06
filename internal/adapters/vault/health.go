package vault

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// HealthFallback — Tier 2 HTTP /health probe (diagnostic only).
// Spec: docs/v04/spec/components/vault.md §Health check 2-tier.
// Python: mcp/adapter/vault_client.py:L322-337.
//
// When to call:
//   - Tier 1 gRPC health.v1 check fails (see Client.HealthCheck)
//   - AND original endpoint is http(s):// scheme
//
// Transform:
//  1. If endpoint not http(s) → return ErrNotHTTPScheme (skip probe)
//  2. Parse URL, trim `/mcp` and `/sse` suffixes from path
//  3. Append `/health` and HTTP GET
//  4. Return nil if 2xx; otherwise error with status code
//
// Purpose: when gRPC port is unreachable but HTTP health is up, we can report
// "endpoint reachable, only gRPC layer has issue" in diagnostics hints.
// Not a control-plane path — purely informational.
func HealthFallback(ctx context.Context, rawEndpoint string) error {
	if !strings.HasPrefix(rawEndpoint, "http") {
		return ErrNotHTTPScheme
	}

	u, err := url.Parse(rawEndpoint)
	if err != nil {
		return fmt.Errorf("vault: invalid endpoint URL: %w", err)
	}

	// Strip suffixes
	path := u.Path
	for _, suffix := range []string{"/mcp", "/sse"} {
		if strings.HasSuffix(path, suffix) {
			path = strings.TrimSuffix(path, suffix)
			break
		}
	}
	u.Path = path + "/health"

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return fmt.Errorf("vault: failed to create health request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("vault: health endpoint returned %d", resp.StatusCode)
	}
	return nil
}
