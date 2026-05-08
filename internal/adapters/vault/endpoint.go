package vault

import (
	"net"
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
//
// Implementation notes:
//   - tcp:// / http:// / https:// all flow through url.Parse so a raw IPv6
//     literal (e.g. "tcp://[::1]:50051") is split correctly.
//   - Schemeless inputs are treated as authority only — url.Parse can't be
//     used directly on "host:port" since it'd interpret "host" as the
//     scheme.
//   - Default port (50051) is appended in exactly one place — the host
//     never carries a port at the end of the function path means we add it.
const defaultGRPCPort = "50051"

func NormalizeEndpoint(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", &Error{Code: "VAULT_BAD_ENDPOINT", Message: "endpoint is empty"}
	}

	var host string
	switch {
	case strings.HasPrefix(raw, "tcp://"),
		strings.HasPrefix(raw, "http://"),
		strings.HasPrefix(raw, "https://"):
		u, err := url.Parse(raw)
		if err != nil {
			return "", &Error{Code: "VAULT_BAD_ENDPOINT", Message: "parse endpoint: " + err.Error()}
		}
		host = u.Host
	default:
		host = raw
	}

	if host == "" {
		return "", &Error{Code: "VAULT_BAD_ENDPOINT", Message: "endpoint missing host: " + raw}
	}

	// Append default port unless one is already present. Treat trailing
	// `:port` on an IPv6 literal correctly: bracketed forms like "[::1]"
	// have no `:` outside the brackets after the closing `]`.
	//
	// url.Parse already returns IPv6 hosts in their bracketed form
	// ("[::1]"), so calling net.JoinHostPort on them double-brackets.
	// Append manually in that case; otherwise JoinHostPort handles
	// hostnames + IPv4 + un-bracketed IPv6 (which url.Parse never
	// produces, but a direct caller might).
	if !hasPort(host) {
		if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
			host = host + ":" + defaultGRPCPort
		} else {
			host = net.JoinHostPort(host, defaultGRPCPort)
		}
	}
	return host, nil
}

// hasPort reports whether host already carries a `:port` suffix.
// Handles IPv6 literals: "[::1]" → false, "[::1]:50051" → true.
func hasPort(host string) bool {
	if strings.HasPrefix(host, "[") {
		return strings.Contains(host[strings.Index(host, "]"):], ":")
	}
	return strings.Contains(host, ":")
}
