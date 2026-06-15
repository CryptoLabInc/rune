package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

var ErrInstallInProgress = errors.New("install: another install in progress")

type InstallOptions struct {
	ManifestURL string
	Force       bool     // `rune install --force` to force re-download
	Target      []string // empty = all; StepRuneMCP | StepRuned
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

	// Total step count
	total := 3 // all steps
	if len(opts.Target) > 0 {
		total = 1 + len(opts.Target) // manifest + num artifacts
	}

	// Fetch manifest
	logf("[1/%d] manifest: fetching from %s", total, opts.ManifestURL)
	manifest, err := FetchManifest(ctx, opts.ManifestURL, logf)
	if err != nil {
		r.Failed[StepManifest] = err.Error()
		r.Status = "partial"
		return r, err
	}
	r.Completed = append(r.Completed, StepManifest)
	logf("[1/%d] manifest: ok (rune-mcp %s, runed %s)", total, manifest.RuneMCPVersion, manifest.RunedVersion)

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

	// Filter install target
	if len(opts.Target) > 0 {
		installs = slices.DeleteFunc(installs, func(in install) bool {
			return !slices.Contains(opts.Target, in.step)
		})
	}

	// Get previous installed info
	recordedDestSHA := map[string]string{}
	if prior, perr := ReadInstalledManifest(paths); perr == nil && prior != nil {
		for step, a := range prior.Artifacts {
			recordedDestSHA[step] = a.DestSHA256
		}
	}

	for i, in := range installs {
		stepNum := i + 2 // step 1: manifest
		if !opts.Force && fileExists(in.dest) {
			if skipExistingArtifact(in.step, in.spec, in.dest, recordedDestSHA[in.step], stepNum, total, logf) {
				r.Completed = append(r.Completed, in.step)
				r.Skipped = append(r.Skipped, in.step)

				continue
			}
			// Corrupted or staled binary (SHA mismatch / unverifiable): re-install
		}

		logf("[%d/%d] %s (%d bytes): downloading...", stepNum, total, in.step, in.spec.Size)
		if err := installArtifact(ctx, paths, in.spec, in.dest, opts.Progress, logf); err != nil {
			r.Failed[in.step] = err.Error()
			r.Status = "partial"
			return r, err
		}

		r.Completed = append(r.Completed, in.step)
		if info, statErr := os.Stat(in.dest); statErr == nil {
			r.Installed[filepath.Base(in.dest)] = ArtifactInfo{Path: in.dest, Size: info.Size()}
		}
		logf("[%d/%d] %s: installed at %s", stepNum, total, in.step, in.dest)
	}

	// Record install audit is only available with full `rune install` (no not allow partial record)
	if len(opts.Target) == 0 {
		auditArtifacts := make(map[string]InstalledArtifact, len(installs))
		for _, in := range installs {
			entry := InstalledArtifact{
				URL:    in.spec.URL,
				SHA256: in.spec.SHA256,
				Path:   in.dest,
				Size:   in.spec.Size,
			}

			if info, statErr := os.Stat(in.dest); statErr == nil {
				entry.Size = info.Size()
			}

			entry.DestSHA256 = in.spec.SHA256 // record on-disk binary hash for later verification
			if in.spec.Extract != "" {
				if h, hErr := FileSHA256(in.dest); hErr == nil {
					entry.DestSHA256 = h
				} else {
					entry.DestSHA256 = "" // leave as unverified
					logf("warning: cannot hash %s for audit: %v", in.dest, hErr)
				}
			}

			auditArtifacts[in.step] = entry
		}

		if err := WriteInstalledManifest(paths, opts.ManifestURL, manifest, auditArtifacts); err != nil {
			logf("warning: installed.json write failed: %v", err) // not fatal error
		} else {
			logf("audit: installed.json updated at %s", paths.InstalledManifest)
		}
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

// spec.Extract == ""      : raw binaries are downloaded directly to dest
// spec.Extract == "tar.gz": archive is extracted into dest
// Others treat as error
func installArtifact(ctx context.Context, paths *Paths, spec ArtifactSpec, dest string, progress ProgressFunc, logf func(string, ...any)) error {
	switch spec.Extract {
	case "": // raw binaries
		if err := withRetry(ctx, logf, "download "+filepath.Base(dest), func() error {
			return DownloadAndVerify(ctx, spec, dest, progress)
		}); err != nil {
			return err
		}
		if err := os.Chmod(dest, binaryMode); err != nil {
			return fmt.Errorf("install: chmod %s: %w", dest, err)
		}

		return nil
	case "tar.gz": // Tarball archive
		// Download archive under paths.Cache first
		tarPath := filepath.Join(paths.Cache, filepath.Base(dest)+".tar.gz")
		if err := withRetry(ctx, logf, "download "+filepath.Base(dest), func() error {
			return DownloadAndVerify(ctx, spec, tarPath, progress)
		}); err != nil {
			return err
		}
		defer os.Remove(tarPath)

		// Extract to destDir
		destDir := filepath.Dir(dest)
		if err := ExtractTarball(tarPath, destDir); err != nil {
			return fmt.Errorf("install: extract %s into %s: %w", tarPath, destDir, err)
		}
		if !fileExists(dest) {
			return fmt.Errorf("install: tarball did not have expected file at %s", dest)
		}
		if err := os.Chmod(dest, binaryMode); err != nil {
			return fmt.Errorf("install: chmod %s: %w", dest, err)
		}

		return nil
	default:
		return fmt.Errorf("install: unsupported extract type %q", spec.Extract)
	}
}

func skipExistingArtifact(step string, spec ArtifactSpec, dest, recordedDestSHA string, stepNum, total int, logf func(string, ...any)) bool {
	expected, ref := spec.SHA256, "manifest" // installed from raw binary
	if spec.Extract != "" {
		expected, ref = recordedDestSHA, "installed.json" // installed from tarball artifact
	}
	if expected == "" {
		logf("[%d/%d] %s: skipped (already at %s; no recorded sha256 to verify)", stepNum, total, step, dest)
		return true
	}

	got, err := FileSHA256(dest)
	switch {
	case err != nil:
		logf("[%d/%d] %s: cannot verify existing %s (%v); re-downloading", stepNum, total, step, dest, err)
		return false
	case !strings.EqualFold(got, expected):
		logf("[%d/%d] %s: existing %s failed sha256 check vs %s (got %s, want %s); re-downloading", stepNum, total, step, dest, ref, got, expected)
		return false
	default:
		logf("[%d/%d] %s: skipped (already at %s, sha256 verified vs %s)", stepNum, total, step, dest, ref)
		return true
	}
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
