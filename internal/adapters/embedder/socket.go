package embedder

import (
	"os"
	"path/filepath"
)

// DefaultSocketPath is the runed daemon convention path relative to $HOME:
// ~/.runed/embedding.sock (Plan A scope, macOS/Linux). Spec: embedder.md
// §소켓 경로.
const DefaultSocketPath = ".runed/embedding.sock"

// SocketEnvVar is the env var that overrides socket discovery for tests
// and non-default installations.
const SocketEnvVar = "RUNE_EMBEDDER_SOCKET"

// ResolveSocketPath returns the unix-socket path to use when dialing the
// runed daemon. Priority (per spec/components/embedder.md §소켓 경로):
//
//  1. env RUNE_EMBEDDER_SOCKET
//  2. configPath argument (typically from config.embedder.socket_path; pass
//     empty string when unset)
//  3. ~/.runed/embedding.sock (runed convention default)
//
// The returned path is always absolute when the default branch is taken
// (UserHomeDir + relative join). When a caller passes an explicit
// configPath that is relative, it's returned as-is — caller's responsibility
// to ensure it resolves correctly from the rune-mcp working directory.
//
// Returns the empty string only if the home-dir lookup fails AND none of
// env/config provided a value. Callers should treat empty as a fatal
// configuration error.
func ResolveSocketPath(configPath string) string {
	if envPath := os.Getenv(SocketEnvVar); envPath != "" {
		return envPath
	}
	if configPath != "" {
		return configPath
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, DefaultSocketPath)
}
