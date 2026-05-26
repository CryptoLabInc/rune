package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Overrides default root: `~/.rune/`
const envRuneHome = "RUNE_HOME"

// Overrides default root: `~/.runed/`
const envRunedHome = "RUNED_HOME"
const envManifest = "RUNE_MANIFEST"

// Two separate roots, NO cross-contamination:
//
//	~/.rune/              - Rune plugin realm (config + rune-mcp binary)
//	├── bin/
//	│   └── rune-mcp      - placed here by `rune install`
//	└── config.json       - /rune:configure handles this
//
//	~/.runed/             - Runed daemon realm
//	├── bin/
//	│   ├── runed         - placed by `rune install`
//	│   └── llama-server  - placed by runed on first boot (self-bootstrap)
//	├── models/
//	│   └── *.gguf        - placed by runed on first boot
//	├── embedding.sock
//	├── spawn.lock
//	├── install.lock
//	├── cache/
//	└── logs/daemon.log

type Paths struct {
	// Rune
	RuneHome      string // ~/.rune
	RuneBin       string // ~/.rune/bin
	RuneMCPBinary string // ~/.rune/bin/rune-mcp
	RuneConfig    string // ~/.rune/config.json

	// Runed
	RunedHome   string // ~/.runed
	RunedBin    string // ~/.runed/bin
	RunedModels string // ~/.runed/models
	RunedSocket string // ~/.runed/embedding.sock
	RunedLock   string // ~/.runed/spawn.lock
	RunedLogs   string // ~/.runed/logs
	RunedBinary string // ~/.runed/bin/runed
	LlamaServer string // ~/.runed/bin/llama-server
	InstallLock string // ~/.runed/install.lock
	Cache       string // ~/.runed/cache
}

func Resolve() (*Paths, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user home: %w", err)
	}

	// Rune
	runeHome := os.Getenv(envRuneHome)
	if runeHome == "" {
		runeHome = filepath.Join(userHome, ".rune")
	}

	runeHome, err = filepath.Abs(runeHome)
	if err != nil {
		return nil, fmt.Errorf("absolute path for rune home %s: %w", runeHome, err)
	}

	// Runed
	runedHome := os.Getenv(envRunedHome)
	if runedHome == "" {
		runedHome = filepath.Join(userHome, ".runed")
	}

	runedHome, err = filepath.Abs(runedHome)
	if err != nil {
		return nil, fmt.Errorf("absolute path for runed home %s: %w", runedHome, err)
	}

	return newPaths(runeHome, runedHome), nil
}

func newPaths(runeHome, runedHome string) *Paths {
	runeBin := filepath.Join(runeHome, "bin")
	runedBin := filepath.Join(runedHome, "bin")
	return &Paths{
		RuneHome:      runeHome,
		RuneBin:       runeBin,
		RuneMCPBinary: filepath.Join(runeBin, "rune-mcp"),
		RuneConfig:    filepath.Join(runeHome, "config.json"),

		RunedHome:   runedHome,
		RunedBin:    runedBin,
		RunedModels: filepath.Join(runedHome, "models"),
		RunedSocket: filepath.Join(runedHome, "embedding.sock"),
		RunedLock:   filepath.Join(runedHome, "spawn.lock"),
		RunedLogs:   filepath.Join(runedHome, "logs"),
		RunedBinary: filepath.Join(runedBin, "runed"),
		LlamaServer: filepath.Join(runedBin, "llama-server"),
		InstallLock: filepath.Join(runedHome, "install.lock"),
		Cache:       filepath.Join(runedHome, "cache"),
	}
}

func (p *Paths) EnsureDirs() error {
	for _, d := range []string{
		p.RuneBin,
		p.RunedHome, p.RunedBin, p.RunedModels, p.RunedLogs, p.Cache,
	} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return nil
}

func PlatformTuple() string {
	return runtime.GOOS + "-" + runtime.GOARCH // {os}-{arch}, e.g. "linux-amd64"
}
