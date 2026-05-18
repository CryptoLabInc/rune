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
//	~/.rune/
//	└── config.json       - /rune:configure handle this
//
//	~/.runed/             - Runed itself handle this through `rune install`
//	├── bin/
//	│   ├── runed
//	│   └── llama-server
//	├── models/
//	│   └── *.gguf
//	├── embedding.sock
//	├── spawn.lock
//	├── install.lock
//	├── cache/
//	└── logs/daemon.log
//
// XXX: `rm -rf ~/.rune` clears rune-mcp state including Vault credentials;
// `rm -rf ~/.runed` clears Runed daemon state including model
type Paths struct {
  // Rune
	RuneHome   string // ~/.rune
	RuneConfig string // ~/.rune/config.json

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
	runedBin := filepath.Join(runedHome, "bin")
	return &Paths{
		RuneHome:   runeHome,
		RuneConfig: filepath.Join(runeHome, "config.json"),

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

// RuneHome is not created here
func (p *Paths) EnsureDirs() error {
	for _, d := range []string{
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
