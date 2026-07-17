package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type ArtifactVersion struct {
	Step      string `json:"step"` // StepRuned | StepRuneMCP
	Installed string `json:"installed"`
	Available string `json:"available"` // available versions from updated manifest
	Outdated  bool   `json:"outdated"`
}

type UpdateList struct {
	Artifacts []ArtifactVersion `json:"artifacts"`
}

func (p UpdateList) HasUpdates() bool {
	for _, a := range p.Artifacts {
		if a.Outdated {
			return true
		}
	}

	return false
}

func (p UpdateList) Outdated() []ArtifactVersion {
	var out []ArtifactVersion
	for _, a := range p.Artifacts {
		if a.Outdated {
			out = append(out, a)
		}
	}

	return out
}

// Strip "v" prefix and build metadata
func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)

	// Strip "v/V" prefix
	if len(v) > 1 && (v[0] == 'v' || v[0] == 'V') && v[1] >= '0' && v[1] <= '9' {
		v = v[1:]
	}

	// Drop build metadata
	if i := strings.IndexByte(v, '+'); i > 0 {
		v = v[:i]
	}
	return v
}

func planUpdate(installed *InstalledManifest, manifest *Manifest, cliVersion string) UpdateList {
	plan := UpdateList{}
	if manifest == nil {
		return plan // no updates
	}

	for _, step := range []string{StepRuned, StepRuneMCP, StepRuneCLI} {
		var inst, avail string
		var outdated bool

		switch step {
		case StepRuned:
			avail = manifest.RunedVersion
			if installed != nil {
				inst = installed.RunedVersion
			}
			outdated = avail != "" && inst != "" && normalizeVersion(inst) != normalizeVersion(avail)
		case StepRuneMCP:
			avail = manifest.RuneMCPVersion
			if installed != nil {
				inst = installed.RuneMCPVersion
			}
			outdated = avail != "" && inst != "" && normalizeVersion(inst) != normalizeVersion(avail)
		case StepRuneCLI:
			avail = manifest.CLIVersion
			inst = cliVersion
			outdated = avail != "" && inst != "" && compareVersions(inst, avail) < 0
		}

		plan.Artifacts = append(plan.Artifacts, ArtifactVersion{
			Step:      step,
			Installed: inst,
			Available: avail,
			Outdated:  outdated,
		})
	}
	return plan
}

func CheckUpdate(ctx context.Context, manifestURL, cliVersion string, logf func(format string, args ...any)) (*UpdateList, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}

	// Fetch manifest
	manifest, err := FetchManifest(ctx, manifestURL, logf)
	if err != nil {
		return nil, err
	}

	return PlanFromManifest(manifest, cliVersion)
}

func PlanFromManifest(manifest *Manifest, cliVersion string) (*UpdateList, error) {
	paths, err := Resolve()
	if err != nil {
		return nil, err
	}

	// Get local installed info
	installed, _ := ReadInstalledManifest(paths)        // nil: unknown version
	plan := planUpdate(installed, manifest, cliVersion) // build update plan

	return &plan, nil
}

func UpdateArtifact(ctx context.Context, manifestURL, step string, afterInstall func() error, logf func(format string, args ...any)) (string, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}

	if step != StepRuned && step != StepRuneMCP {
		return "", fmt.Errorf("update: unknown artifact %q", step)
	}

	manifest, err := FetchManifest(ctx, manifestURL, logf)
	if err != nil {
		return "", err
	}

	arts, err := manifest.ArtifactsForCurrentPlatform()
	if err != nil {
		return "", err
	}

	paths, err := Resolve()
	if err != nil {
		return "", err
	}

	// Update (re-download / verify / atomic-swap)
	if _, err := Install(ctx, InstallOptions{
		ManifestURL: manifestURL,
		Target:      []string{step},
		Force:       true,
		Log:         logf,
	}); err != nil {
		return "", err
	}

	// Reload updated binary before recording
	if afterInstall != nil {
		if err := afterInstall(); err != nil {
			return "", err
		}
	}

	var spec ArtifactSpec
	var dest, version string
	switch step {
	case StepRuneMCP:
		spec, dest, version = arts.RuneMCP, paths.RuneMCPBinary, manifest.RuneMCPVersion
	case StepRuned:
		spec, dest, version = arts.Runed, paths.RunedBinary, manifest.RunedVersion
	}

	// Update install audit
	unlock, err := acquireInstallLock(ctx, paths.InstallLock, InstallLockTimeout)
	if err != nil {
		return "", fmt.Errorf("update: acquire lock for audit write: %w", err)
	}
	defer unlock()

	rec, _ := ReadInstalledManifest(paths)
	if rec == nil {
		rec = &InstalledManifest{ManifestVersion: manifest.Version, Platform: PlatformTuple()}
	}
	if rec.Artifacts == nil {
		rec.Artifacts = map[string]InstalledArtifact{}
	}

	entry := InstalledArtifact{URL: spec.URL, SHA256: spec.SHA256, Path: dest, Size: spec.Size}
	if info, statErr := os.Stat(dest); statErr == nil {
		entry.Size = info.Size()
	}

	entry.DestSHA256 = spec.SHA256
	if spec.Extract != "" {
		if h, hErr := FileSHA256(dest); hErr == nil {
			entry.DestSHA256 = h
		} else {
			entry.DestSHA256 = ""
			logf("warning: cannot hash %s for audit: %v", dest, hErr)
		}
	}
	rec.Artifacts[step] = entry

	switch step {
	case StepRuneMCP:
		rec.RuneMCPVersion = version
	case StepRuned:
		rec.RunedVersion = version
	}
	rec.ManifestURL = manifestURL
	rec.ManifestVersion = manifest.Version
	rec.InstalledAt = time.Now().UTC().Format(time.RFC3339)

	return version, writeManifest(paths, rec)
}

func ChannelBehind(m *Manifest, cliVersion string) bool {
	if m == nil || cliVersion == "" {
		return false // unknown running version: leave the plan alone
	}
	if m.CLIVersion == "" {
		return true // predates self-update
	}

	return compareVersions(m.CLIVersion, cliVersion) < 0
}

var ErrCLIOutdated = errors.New("update: CLI is not newer than exsiting binary")

// cliVersion is the running binary's own version. The strictly-newer gate is
// re-checked here against the manifest actually being installed from: the
// caller planned against an earlier fetch and the channel can move in
// between, so trusting the plan alone would let a stale plan swap an older
// binary over a newer one.
func UpdateCLI(ctx context.Context, manifestURL, cliVersion string, logf func(format string, args ...any)) (string, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}

	manifest, err := FetchManifest(ctx, manifestURL, logf)
	if err != nil {
		return "", err
	}
	if manifest.CLIVersion == "" {
		return "", fmt.Errorf("update: manifest declares no cli_version")
	}
	if cliVersion != "" && compareVersions(cliVersion, manifest.CLIVersion) >= 0 {
		return "", fmt.Errorf("%w: channel %s, running %s", ErrCLIOutdated, manifest.CLIVersion, cliVersion)
	}

	tuple := PlatformTuple()
	arts, ok := manifest.Platforms[tuple]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrNoArtifactForPlatform, tuple)
	}
	spec := arts.RuneCLI
	if spec.URL == "" || spec.SHA256 == "" {
		return "", fmt.Errorf("manifest: rune_cli artifact for %s missing url or sha256", tuple)
	}

	paths, err := Resolve()
	if err != nil {
		return "", err
	}
	if err := paths.EnsureDirs(); err != nil {
		return "", err
	}

	unlock, err := acquireInstallLock(ctx, paths.InstallLock, InstallLockTimeout)
	if err != nil {
		return "", fmt.Errorf("update: acquire lock: %w", err)
	}
	defer unlock()

	if err := installArtifact(ctx, paths, spec, paths.RuneCLIBinary, nil, logf); err != nil {
		return "", err
	}

	rec, _ := ReadInstalledManifest(paths)
	if rec == nil {
		rec = &InstalledManifest{ManifestVersion: manifest.Version, Platform: tuple}
	}
	if rec.Artifacts == nil {
		rec.Artifacts = map[string]InstalledArtifact{}
	}

	entry := InstalledArtifact{URL: spec.URL, SHA256: spec.SHA256, DestSHA256: spec.SHA256, Path: paths.RuneCLIBinary, Size: spec.Size}
	if info, statErr := os.Stat(paths.RuneCLIBinary); statErr == nil {
		entry.Size = info.Size()
	}
	rec.Artifacts[StepRuneCLI] = entry

	rec.ManifestURL = manifestURL
	rec.ManifestVersion = manifest.Version
	rec.InstalledAt = time.Now().UTC().Format(time.RFC3339)

	return manifest.CLIVersion, writeManifest(paths, rec) // return new CLI version
}
