package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type InstallCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`             // "ok" | "warn" | "fail"
	Detail  string `json:"detail,omitempty"`
	FixHint string `json:"fix_hint,omitempty"`
}

type InstallChecks struct {
	OK     bool             `json:"ok"`
	Checks []InstallCheck `json:"checks"`
}

const (
	StatusOK   = "ok"
	StatusWarn = "warn"
	StatusFail = "fail"
)

const (
	CheckRuneConfig  = "rune_config"
	CheckVaultCreds  = "vault_creds"
	CheckRunedBinary = "runed_binary"
	CheckLlamaServer = "llama_server"
	CheckModelFile   = "model_file"
	CheckSocket      = "daemon_socket"
	CheckSpawnLock   = "spawn_lock"
)

type runeMCPConfig struct {
	Vault    *runeVaultBlock    `json:"vault,omitempty"`
	Embedder *runeEmbedderBlock `json:"embedder,omitempty"`
}

type runeVaultBlock struct {
	Endpoint string `json:"endpoint"`
	Token    string `json:"token"`
}

type runeEmbedderBlock struct {
	SocketPath string `json:"socket_path,omitempty"`
}

func RunInstallChecks(ctx context.Context) *InstallChecks {
	paths, err := Resolve()
	if err != nil {
		return &InstallChecks{
			OK: false,
			Checks: []InstallCheck{{
				Name:    "path_resolve",
				Status:  StatusFail,
				Detail:  err.Error(),
				FixHint: "ensure $HOME is set",
			}},
		}
	}

	cfg, cfgChecks := loadRuneConfigChecks(paths)
	checks := append([]InstallCheck{}, cfgChecks...)
	checks = append(checks, vaultCredsCheck(cfg))
	checks = append(checks, executableCheck(CheckRunedBinary, paths.RunedBinary, "run `rune install` to fetch the runed bundle"))
	checks = append(checks, executableCheck(CheckLlamaServer, paths.LlamaServer, "run `rune install` to fetch the runed bundle"))
	checks = append(checks, modelFileCheck(paths.RunedModels))
	checks = append(checks, socketCheck(paths.RunedSocket, cfg))
	checks = append(checks, spawnLockCheck(paths.RunedLock))

	r := &InstallChecks{OK: true, Checks: checks}
	for _, c := range r.Checks {
		if c.Status == StatusFail {
			r.OK = false
		}
	}
	return r
}

func loadRuneConfigChecks(paths *Paths) (*runeMCPConfig, []InstallCheck) {
	data, err := os.ReadFile(paths.RuneConfig)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, []InstallCheck{{
				Name:    CheckRuneConfig,
				Status:  StatusFail,
				Detail:  fmt.Sprintf("%s does not exist", paths.RuneConfig),
				FixHint: "run /rune:configure to set up Vault credentials",
			}}
		}

		return nil, []InstallCheck{{
			Name:    CheckRuneConfig,
			Status:  StatusFail,
			Detail:  fmt.Sprintf("read %s: %v", paths.RuneConfig, err),
			FixHint: "check file permissions",
		}}
	}

	var cfg runeMCPConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, []InstallCheck{{
			Name:    CheckRuneConfig,
			Status:  StatusFail,
			Detail:  fmt.Sprintf("parse %s: %v", paths.RuneConfig, err),
			FixHint: "re-run /rune:configure to rewrite a valid config",
		}}
	}

	return &cfg, []InstallCheck{{
		Name:   CheckRuneConfig,
		Status: StatusOK,
		Detail: paths.RuneConfig,
	}}
}

func vaultCredsCheck(cfg *runeMCPConfig) InstallCheck {
	if cfg == nil {
		return InstallCheck{
			Name:    CheckVaultCreds,
			Status:  StatusFail,
			Detail:  "config file unreadable",
			FixHint: "fix rune_config above first",
		}
	}

	var missing []string
	if cfg.Vault == nil || cfg.Vault.Endpoint == "" {
		missing = append(missing, "vault.endpoint")
	}
	if cfg.Vault == nil || cfg.Vault.Token == "" {
		missing = append(missing, "vault.token")
	}
	if len(missing) > 0 {
		return InstallCheck{
			Name:    CheckVaultCreds,
			Status:  StatusFail,
			Detail:  fmt.Sprintf("missing: %s", strings.Join(missing, ", ")),
			FixHint: "run /rune:configure to provide Vault endpoint and token",
		}
	}

	return InstallCheck{
		Name:   CheckVaultCreds,
		Status: StatusOK,
		Detail: cfg.Vault.Endpoint,
	}
}

func executableCheck(name, path, fixHint string) InstallCheck {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return InstallCheck{
				Name:    name,
				Status:  StatusFail,
				Detail:  fmt.Sprintf("%s does not exist", path),
				FixHint: fixHint,
			}
		}

		return InstallCheck{
			Name:    name,
			Status:  StatusFail,
			Detail:  fmt.Sprintf("stat %s: %v", path, err),
			FixHint: fixHint,
		}
	}

	if info.IsDir() {
		return InstallCheck{
			Name:    name,
			Status:  StatusFail,
			Detail:  fmt.Sprintf("%s is a directory, expected file", path),
			FixHint: fixHint,
		}
	}

	if info.Mode()&0o111 == 0 {
		return InstallCheck{
			Name:    name,
			Status:  StatusFail,
			Detail:  fmt.Sprintf("%s exists but is not executable (mode=%o)", path, info.Mode().Perm()),
			FixHint: fmt.Sprintf("chmod +x %s, or re-run `rune install --force`", path),
		}
	}

	return InstallCheck{
		Name:   name,
		Status: StatusOK,
		Detail: fmt.Sprintf("%s (%.1f MB)", path, float64(info.Size())/(1024*1024)),
	}
}

const minModelSize int64 = 100 * 1024 * 1024 // Qwen3-Embedding-0.6B: ~340 MB

func modelFileCheck(modelsDir string) InstallCheck {
	entries, err := os.ReadDir(modelsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return InstallCheck{
				Name:    CheckModelFile,
				Status:  StatusWarn,
				Detail:  fmt.Sprintf("%s does not exist", modelsDir),
				FixHint: "runed will populate this on first startup (auto-downloads from baked-in URL)",
			}
		}

		return InstallCheck{
			Name:    CheckModelFile,
			Status:  StatusFail,
			Detail:  fmt.Sprintf("read %s: %v", modelsDir, err),
			FixHint: "check directory permissions",
		}
	}

	var found []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".gguf") {
			continue
		}

		info, err := e.Info()
		if err != nil {
			continue
		}

		if info.Size() < minModelSize {
			return InstallCheck{
				Name:    CheckModelFile,
				Status:  StatusFail,
				Detail:  fmt.Sprintf("%s is only %d bytes (looks partial)", filepath.Join(modelsDir, e.Name()), info.Size()),
				FixHint: "delete the partial and re-run /rune:activate (runed will re-download)",
			}
		}
		found = append(found, e.Name())
	}

	if len(found) == 0 {
		return InstallCheck{
			Name:    CheckModelFile,
			Status:  StatusWarn,
			Detail:  fmt.Sprintf("no *.gguf in %s", modelsDir),
			FixHint: "runed will populate this on first startup (auto-downloads from baked-in URL)",
		}
	}

	return InstallCheck{
		Name:   CheckModelFile,
		Status: StatusOK,
		Detail: strings.Join(found, ", "),
	}
}

func socketCheck(defaultSocket string, cfg *runeMCPConfig) InstallCheck {
	socket := defaultSocket
	if cfg != nil && cfg.Embedder != nil && cfg.Embedder.SocketPath != "" {
		socket = cfg.Embedder.SocketPath
	}

	conn, err := net.DialTimeout("unix", socket, 200*time.Millisecond)
	if err != nil {
		return InstallCheck{
			Name:    CheckSocket,
			Status:  StatusWarn,
			Detail:  fmt.Sprintf("not reachable at %s", socket),
			FixHint: "runed will spawn on next /rune:activate",
		}
	}
	_ = conn.Close()

	return InstallCheck{
		Name:   CheckSocket,
		Status: StatusOK,
		Detail: socket,
	}
}

func spawnLockCheck(lockPath string) InstallCheck {
	if _, err := os.Stat(lockPath); err != nil {
		if os.IsNotExist(err) {
			return InstallCheck{
				Name:   CheckSpawnLock,
				Status: StatusOK,
				Detail: "absent",
			}
		}

		return InstallCheck{
			Name:    CheckSpawnLock,
			Status:  StatusWarn,
			Detail:  fmt.Sprintf("stat %s: %v", lockPath, err),
			FixHint: "if no runed is running, rm the lock file",
		}
	}

	return InstallCheck{
		Name:    CheckSpawnLock,
		Status:  StatusWarn,
		Detail:  fmt.Sprintf("%s exists", lockPath),
		FixHint: "normal if a runed is running; if no daemon, rm the lock file",
	}
}

var _ = errors.New
