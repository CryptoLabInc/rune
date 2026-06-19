package bootstrap

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

type ArtifactVersion struct {
	Step      string `json:"step"` // StepRuned | StepRuneMCP
	Installed string `json:"installed"`
	Available string `json:"available"` // available verions from updated manifest
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

func planUpdate(installed *InstalledManifest, manifest *Manifest) UpdateList {
	plan := UpdateList{}
	if manifest == nil {
		return plan // no updates
	}

	for _, step := range []string{StepRuned, StepRuneMCP} {
		var inst, avail string

		switch step {
		case StepRuned:
			avail = manifest.RunedVersion
			if installed != nil {
				inst = installed.RunedVersion
			}
		case StepRuneMCP:
			avail = manifest.RuneMCPVersion
			if installed != nil {
				inst = installed.RuneMCPVersion
			}
		}

		plan.Artifacts = append(plan.Artifacts, ArtifactVersion{
			Step:      step,
			Installed: inst,
			Available: avail,
			Outdated:  avail != "" && inst != "" && normalizeVersion(inst) != normalizeVersion(avail),
		})
	}
	return plan
}

func CheckUpdate(ctx context.Context, manifestURL string, logf func(format string, args ...any)) (*UpdateList, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}

	// Fetch manifest
	manifest, err := FetchManifest(ctx, manifestURL, logf)
	if err != nil {
		return nil, err
	}

	paths, err := Resolve()
	if err != nil {
		return nil, err
	}

	// Get local installed info
	installed, _ := ReadInstalledManifest(paths) // nil: unknown version
	plan := planUpdate(installed, manifest)

	return &plan, nil
}

// Swap binary only; rune-mcp applies on next spawn and runed needs manual restart
func UpdateArtifact(ctx context.Context, manifestURL, step string, logf func(format string, args ...any)) (string, error) {
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
