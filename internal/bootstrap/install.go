package bootstrap

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

type InstallOptions struct {
	ManifestURL string
	Force       bool // `rune install --force` to force re-download
	Progress    ProgressFunc
	Log         func(format string, args ...any)
}

type Result struct {
	OK        bool                    `json:"ok"`
	Status    string                  `json:"status"` // "installed" | "no_op" | "partial"
	Completed []string                `json:"completed,omitempty"`
	Skipped   []string                `json:"skipped,omitempty"`
	Failed    map[string]string       `json:"failed,omitempty"`
	Installed map[string]ArtifactInfo `json:"installed,omitempty"`
}

type ArtifactInfo struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

const (
	StepManifest = "manifest"
	StepRuned    = "runed"
	StepRuneMCP  = "rune_mcp"
)

const binaryMode = 0o755 // executable

func Install(ctx context.Context, opts InstallOptions) (*Result, error) {
	logf := opts.Log
	if logf == nil {
		logf = func(string, ...any) {}
	}

	// Resolve paths and ensure directories
	paths, err := Resolve()
	if err != nil {
		return nil, err
	}
	if err := paths.EnsureDirs(); err != nil {
		return nil, err
	}

	// Acquire lock for installation
	unlock, err := acquireInstallLock(ctx, paths.InstallLock, InstallLockTimeout)
	if err != nil {
		return nil, fmt.Errorf("install: acquire lock: %w", err)
	}
	defer unlock()

	r := &Result{
		Status:    "installed",
		Failed:    map[string]string{},
		Installed: map[string]ArtifactInfo{},
	}

	// Fetch manifest
	logf("[1/3] manifest: fetching from %s", opts.ManifestURL)
	manifest, err := FetchManifest(ctx, opts.ManifestURL)
	if err != nil {
		r.Failed[StepManifest] = err.Error()
		r.Status = "partial"
		return r, err
	}
	r.Completed = append(r.Completed, StepManifest)
	logf("[1/3] manifest: ok (rune-mcp %s, runed %s)", manifest.RuneMCPVersion, manifest.RunedVersion)

	artifacts, err := manifest.ArtifactsForCurrentPlatform()
	if err != nil {
		r.Failed[StepRuned] = err.Error() // not exact, but just for notifying failure
		r.Status = "partial"
		return r, err
	}

	// Per-artifact installation (Result.Status = "partial" on any failure)
	type install struct {
		step string
		spec ArtifactSpec
		dest string
	}
	installs := []install{
		{StepRuned, artifacts.Runed, paths.RunedBinary},
		{StepRuneMCP, artifacts.RuneMCP, paths.RuneMCPBinary},
	}

	for i, in := range installs {
		stepNum := i + 2 // starting from [2/3]
		if !opts.Force {
			if fileExists(in.dest) {
				logf("[%d/3] %s: skipped (already at %s)", stepNum, in.step, in.dest)
				r.Completed = append(r.Completed, in.step)
				r.Skipped = append(r.Skipped, in.step)

				continue
			}
		}

		logf("[%d/3] %s (%d bytes): downloading...", stepNum, in.step, in.spec.Size)
		if err := DownloadAndVerify(ctx, in.spec, in.dest, opts.Progress); err != nil {
			r.Failed[in.step] = err.Error()
			r.Status = "partial"
			return r, err
		}
		if err := os.Chmod(in.dest, binaryMode); err != nil {
			r.Failed[in.step] = err.Error()
			r.Status = "partial"
			return r, fmt.Errorf("install: chmod %s: %w", in.dest, err)
		}

		r.Completed = append(r.Completed, in.step)
		if info, statErr := os.Stat(in.dest); statErr == nil {
			r.Installed[filepath.Base(in.dest)] = ArtifactInfo{Path: in.dest, Size: info.Size()}
		}
		logf("[%d/3] %s: installed at %s", stepNum, in.step, in.dest)
	}

	// Probe socket
	if probeSocket(paths.RunedSocket) {
		logf("probe: daemon already running at %s", paths.RunedSocket)
	} else {
		logf("probe: daemon not running (expected: first /rune:activate will spawn it)")
	}

	r.OK = true
	if onlySkipped(r) {
		r.Status = "no_op"
	}

	return r, nil
}

func onlySkipped(r *Result) bool {
	if len(r.Installed) > 0 {
		return false
	}
	skipMap := map[string]bool{}
	for _, s := range r.Skipped {
		skipMap[s] = true
	}
	for _, s := range r.Completed {
		if s == StepManifest {
			continue // always run
		}
		if !skipMap[s] {
			return false
		}
	}
	return len(r.Skipped) > 0
}

func probeSocket(path string) bool {
	conn, err := net.DialTimeout("unix", path, 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
