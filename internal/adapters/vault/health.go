package vault

import (
	"context"
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
//
// TODO: implement with net/http (with ctx-aware client + short timeout).
//
//	if !strings.HasPrefix(endpoint, "http") { return ErrNotHTTPScheme }
//	u, _ := url.Parse(endpoint)
//	u.Path = strings.TrimSuffix(strings.TrimSuffix(u.Path, "/mcp"), "/sse") + "/health"
//	req, _ := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
//	resp, err := http.DefaultClient.Do(req)
//	if err != nil { return err }
//	defer resp.Body.Close()
//	if resp.StatusCode >= 300 { return fmt.Errorf("health %d", resp.StatusCode) }
//	return nil
func HealthFallback(ctx context.Context, rawEndpoint string) error {
	if !strings.HasPrefix(rawEndpoint, "http") {
		return ErrNotHTTPScheme
	}
	// TODO: HTTP GET /health with path suffix strip
	_ = ctx
	return nil
}
