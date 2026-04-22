package vault

import (
	"net/url"
	"strings"
)

// NormalizeEndpoint — Python: mcp/adapter/vault_client.py:L116-140 _derive_grpc_target.
// Spec: docs/v04/spec/components/vault.md §Endpoint 파싱·정규화.
//
// Input formats (priority order — env RUNEVAULT_GRPC_TARGET > config):
//
//	"tcp://host:port"         → "host:port"
//	"http://host:port/path"   → "host:port"
//	"https://host:port/path"  → "host:port"
//	"host:port"               → "host:port"
//	"host"                    → "host:50051" (default port)
//
// Returns normalized "host:port" suitable for grpc.NewClient.
// TODO: full implementation with url.Parse and default port handling.
func NormalizeEndpoint(raw string) (string, error) {
	if strings.HasPrefix(raw, "tcp://") {
		return strings.TrimPrefix(raw, "tcp://"), nil
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", err
		}
		host := u.Host
		if !strings.Contains(host, ":") {
			host += ":50051"
		}
		return host, nil
	}
	if !strings.Contains(raw, ":") {
		raw += ":50051"
	}
	return raw, nil
}
