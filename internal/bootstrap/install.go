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
	Force bool // `rune install --force` to force re-download
	Progress ProgressFunc
	Log func(format string, args ...any)
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
	StepBundle   = "runed_bundle"
	StepProbe    = "probe"
)

func Install(ctx context.Context, opts InstallOptions) (*Result, error) {
	logf := opts.Log
	if logf == nil {
		logf = func(string, ...any) {}
	}

  // Resolve path and ensure directories
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
	logf("[1/2] manifest: fetching from %s", opts.ManifestURL)
	manifest, err := FetchManifest(ctx, opts.ManifestURL)
	if err != nil {
		r.Failed[StepManifest] = err.Error()
		r.Status = "partial"
		return r, err
	}
	r.Completed = append(r.Completed, StepManifest)
	logf("[1/2] manifest: ok (version %d, rune-mcp %s)", manifest.Version, manifest.RuneMCPVersion)

	bundleSpec, err := manifest.BundleForCurrentPlatform()
	if err != nil {
		r.Failed[StepBundle] = err.Error()
		r.Status = "partial"
		return r, err
	}

  // Runed binaries -> `~/.runed/bin/`
	if err := installBundle(ctx, paths, bundleSpec, opts, r, logf); err != nil {
		r.Failed[StepBundle] = err.Error()
		r.Status = "partial"
		return r, err
	}

  // Probe
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

func installBundle(ctx context.Context, paths *Paths, spec ArtifactSpec, opts InstallOptions, r *Result, logf func(string, ...any)) error {
	if !opts.Force {
		if fileExists(paths.RunedBinary) && fileExists(paths.LlamaServer) {
			logf("[2/2] runed bundle: skipped (binaries already present at %s)", paths.RunedBin)
			r.Completed = append(r.Completed, StepBundle)
			r.Skipped = append(r.Skipped, StepBundle)
			return nil
		}
	}

	cachePath := filepath.Join(paths.Cache, "runed-bundle.tar.gz")
	logf("[2/2] runed bundle (%d bytes): downloading...", spec.Size)
	if err := DownloadAndVerify(ctx, spec, cachePath, opts.Progress); err != nil {
		return err
	}

	logf("[2/2] runed bundle: extracting to %s ...", paths.RunedBin)
	extracted, err := ExtractBundle(cachePath, paths.RunedBin)
	if err != nil {
		return err
	}
	if err := os.Remove(cachePath); err != nil && !os.IsNotExist(err) {
		logf("warning: failed to remove cache %s: %v", cachePath, err)
	}

	r.Completed = append(r.Completed, StepBundle)
	for _, p := range extracted {
		if info, statErr := os.Stat(p); statErr == nil {
			r.Installed[filepath.Base(p)] = ArtifactInfo{Path: p, Size: info.Size()}
		}
	}
	logf("[2/2] runed bundle: extracted %d files", len(extracted))

	return nil
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
