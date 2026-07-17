package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeVersion(t *testing.T) {
	cases := map[string]string{
		"v0.1.0":         "0.1.0",
		"0.1.0":          "0.1.0",
		"V0.1.0":         "0.1.0",
		"v0.1.0+build.7": "0.1.0",
		"v0.1.0-alpha.4": "0.1.0-alpha.4",
		" v0.1.0 ":       "0.1.0",
		"":               "",
		"version1":       "version1",
		"Version1.0":     "Version1.0",
		"v":              "v",
	}

	for in, want := range cases {
		if got := normalizeVersion(in); got != want {
			t.Errorf("normalizeVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPlanUpdate(t *testing.T) {
	manifest := &Manifest{Version: 1, RuneMCPVersion: "v0.2.0", RunedVersion: "v0.2.0"}

	t.Run("outdated when versions differ", func(t *testing.T) {
		installed := &InstalledManifest{RuneMCPVersion: "v0.1.0", RunedVersion: "v0.2.0"}

		plan := planUpdate(installed, manifest, "")
		if !plan.HasUpdates() {
			t.Fatal("expected an update")
		}

		out := plan.Outdated()
		if len(out) != 1 || out[0].Step != StepRuneMCP {
			t.Fatalf("expected only rune_mcp outdated, got %+v", out)
		}
		if out[0].Installed != "v0.1.0" || out[0].Available != "v0.2.0" {
			t.Errorf("version fields wrong: %+v", out[0])
		}
	})

	t.Run("not outdated when v-prefix and build metadata", func(t *testing.T) {
		installed := &InstalledManifest{RuneMCPVersion: "0.2.0+build.3", RunedVersion: "v0.2.0"}
		if planUpdate(installed, manifest, "").HasUpdates() {
			t.Error("v-prefix / build-metadata differences must not count as update")
		}
	})

	t.Run("prerelease difference is update", func(t *testing.T) {
		m := &Manifest{Version: 1, RuneMCPVersion: "v0.2.0-alpha.5", RunedVersion: "v0.2.0"}
		installed := &InstalledManifest{RuneMCPVersion: "v0.2.0-alpha.4", RunedVersion: "v0.2.0"}
		if !planUpdate(installed, m, "").HasUpdates() {
			t.Error("pre-release tags must count as update")
		}
	})

	t.Run("unknown installed is not flagged", func(t *testing.T) {
		if planUpdate(nil, manifest, "").HasUpdates() {
			t.Error("unknown installed versions must not be flagged")
		}
	})

	t.Run("empty installed version is not flagged", func(t *testing.T) {
		installed := &InstalledManifest{} // blank version
		if planUpdate(installed, manifest, "").HasUpdates() {
			t.Error("blank installed version is unknown, not outdated")
		}
	})

	t.Run("empty manifest version gives nothing", func(t *testing.T) {
		m := &Manifest{Version: 1}
		installed := &InstalledManifest{RuneMCPVersion: "v0.1.0", RunedVersion: "v0.1.0"}
		if planUpdate(installed, m, "").HasUpdates() {
			t.Error("empty manifest versions must not be flagged")
		}
	})

	t.Run("nil manifest (no panic, no updates)", func(t *testing.T) {
		installed := &InstalledManifest{RuneMCPVersion: "v0.1.0", RunedVersion: "v0.1.0"}
		plan := planUpdate(installed, nil, "")
		if plan.HasUpdates() || len(plan.Artifacts) != 0 {
			t.Errorf("nil manifest should be empty plan, got %+v", plan)
		}
	})

	t.Run("cli outdated when channel is newer", func(t *testing.T) {
		m := &Manifest{Version: 1, CLIVersion: "v0.5.0"}
		out := planUpdate(nil, m, "v0.4.1").Outdated()
		if len(out) != 1 || out[0].Step != StepRuneCLI {
			t.Fatalf("expected only rune_cli outdated, got %+v", out)
		}
		if out[0].Installed != "v0.4.1" || out[0].Available != "v0.5.0" {
			t.Errorf("version fields wrong: %+v", out[0])
		}
	})

	t.Run("cli equal (v-prefix aside) is not outdated", func(t *testing.T) {
		m := &Manifest{Version: 1, CLIVersion: "v0.5.0"}
		if planUpdate(nil, m, "0.5.0").HasUpdates() {
			t.Error("equal CLI versions must not count as update")
		}
	})

	t.Run("cli newer than channel is not a downgrade", func(t *testing.T) {
		// Unlike runed/rune_mcp (inequality), the CLI check is ordered: the
		// shared latest channel lagging a fresh binary must not roll it back.
		m := &Manifest{Version: 1, CLIVersion: "v0.5.0"}
		if planUpdate(nil, m, "v0.6.0").HasUpdates() {
			t.Error("older channel CLI must not be offered as update")
		}
	})

	t.Run("cli dev prerelease is older than its release", func(t *testing.T) {
		m := &Manifest{Version: 1, CLIVersion: "v0.5.0"}
		if !planUpdate(nil, m, "v0.5.0-dev").HasUpdates() {
			t.Error("prerelease build must count as older than the release")
		}
	})

	t.Run("cli skipped without cli_version or running version", func(t *testing.T) {
		if planUpdate(nil, manifest, "v0.4.1").HasUpdates() {
			t.Error("manifest without cli_version must not flag the CLI")
		}
		m := &Manifest{Version: 1, CLIVersion: "v9.9.9"}
		if planUpdate(nil, m, "").HasUpdates() {
			t.Error("unknown running version must not be flagged")
		}
	})
}

func TestCheckUpdate(t *testing.T) {
	setRealms(t)
	t.Setenv("RUNE_MANIFEST", "")
	fx := newFixture(t)

	paths, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := paths.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}

	// rune-mcp: old, runed: latest
	rec := &Manifest{Version: 1, RuneMCPVersion: "v0.0.1", RunedVersion: "v0.1.0-test"}
	arts := map[string]InstalledArtifact{
		StepRuneMCP: {Path: paths.RuneMCPBinary},
		StepRuned:   {Path: paths.RunedBinary},
	}
	if err := WriteInstalledManifest(paths, fx.manifestURL(), rec, arts); err != nil {
		t.Fatalf("WriteInstalledManifest: %v", err)
	}

	plan, err := CheckUpdate(context.Background(), fx.manifestURL(), "", nil)
	if err != nil {
		t.Fatalf("CheckUpdate: %v", err)
	}

	out := plan.Outdated()
	if len(out) != 1 || out[0].Step != StepRuneMCP {
		t.Fatalf("expected rune_mcp outdated (v0.0.1 -> v0.1.0-test), got %+v", plan.Artifacts)
	}
	if out[0].Installed != "v0.0.1" || out[0].Available != "v0.1.0-test" {
		t.Errorf("version fields wrong: %+v", out[0])
	}
}

func TestCheckUpdate_NotInstalled(t *testing.T) {
	setRealms(t)
	t.Setenv("RUNE_MANIFEST", "")
	fx := newFixture(t)

	plan, err := CheckUpdate(context.Background(), fx.manifestURL(), "", nil)
	if err != nil {
		t.Fatalf("CheckUpdate: %v", err)
	}
	if plan.HasUpdates() {
		t.Errorf("with no install audit, nothing should be outdated: %+v", plan.Artifacts)
	}
}

func TestUpdateArtifact_UpdateSingleArtifact(t *testing.T) {
	rune, _ := setRealms(t)
	t.Setenv("RUNE_MANIFEST", "")
	fx := newFixture(t)

	paths, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := paths.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}

	// Simulate installed artifact - rune-mcp: old, runed: latest
	rec := &Manifest{Version: 1, RuneMCPVersion: "v0.0.1", RunedVersion: "v0.1.0-test"}
	arts := map[string]InstalledArtifact{
		StepRuneMCP: {Path: paths.RuneMCPBinary, SHA256: "old-mcp", DestSHA256: "old-mcp"},
		StepRuned:   {Path: paths.RunedBinary, SHA256: "runed-sha", DestSHA256: "runed-sha"},
	}
	if err := WriteInstalledManifest(paths, fx.manifestURL(), rec, arts); err != nil {
		t.Fatalf("WriteInstalledManifest: %v", err)
	}

	got, err := UpdateArtifact(context.Background(), fx.manifestURL(), StepRuneMCP, nil, nil)
	if err != nil {
		t.Fatalf("UpdateArtifact: %v", err)
	}
	if got != "v0.1.0-test" {
		t.Errorf("returned version = %q, want v0.1.0-test", got)
	}

	// Reinstall rune-mcp
	if b, _ := os.ReadFile(filepath.Join(rune, "bin", "rune-mcp")); string(b) != string(fx.runeMCP) {
		t.Errorf("rune-mcp not re-installed on disk: got %q", b)
	}

	after, err := ReadInstalledManifest(paths)
	if err != nil {
		t.Fatalf("ReadInstalledManifest: %v", err)
	}
	// rune-mcp should be updated
	if after.RuneMCPVersion != "v0.1.0-test" {
		t.Errorf("rune_mcp_version = %q, want v0.1.0-test", after.RuneMCPVersion)
	}
	if after.Artifacts[StepRuneMCP].DestSHA256 == "old-mcp" {
		t.Errorf("rune_mcp DestSHA256 not refreshed: %q", after.Artifacts[StepRuneMCP].DestSHA256)
	}
	// runed should not be updated
	if after.RunedVersion != "v0.1.0-test" {
		t.Errorf("runed_version changed unexpectedly: %q", after.RunedVersion)
	}
	if after.Artifacts[StepRuned].DestSHA256 != "runed-sha" {
		t.Errorf("runed DestSHA256 should be preserved, got %q", after.Artifacts[StepRuned].DestSHA256)
	}

	// Check after update
	plan, err := CheckUpdate(context.Background(), fx.manifestURL(), "", nil)
	if err != nil {
		t.Fatalf("CheckUpdate: %v", err)
	}
	if plan.HasUpdates() {
		t.Errorf("rune-mcp should be up to date after UpdateArtifact: %+v", plan.Artifacts)
	}
}

func TestUpdateArtifact_UnknownArtifact(t *testing.T) {
	if _, err := UpdateArtifact(context.Background(), "http://unused", "llama-server", nil, nil); err == nil {
		t.Error("expected error for an unknown artifact")
	}
}

func TestUpdateCLI_SwapsCanonicalBinary(t *testing.T) {
	rune, _ := setRealms(t)
	t.Setenv("RUNE_MANIFEST", "")
	fx := newFixture(t)
	fx.cli = []byte("rune-cli-binary-v9")
	fx.cliVersion = "v9.9.9"

	paths, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := paths.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
	// Simulate the currently installed CLI
	if err := os.WriteFile(paths.RuneCLIBinary, []byte("current-cli"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := UpdateCLI(context.Background(), fx.manifestURL(), "v1.0.0", nil)
	if err != nil {
		t.Fatalf("UpdateCLI: %v", err)
	}
	if got != "v9.9.9" {
		t.Errorf("returned version = %q, want v9.9.9", got)
	}
	if b, _ := os.ReadFile(filepath.Join(rune, "bin", "rune")); string(b) != string(fx.cli) {
		t.Errorf("CLI not swapped on disk: got %q", b)
	}

	after, err := ReadInstalledManifest(paths)
	if err != nil {
		t.Fatalf("ReadInstalledManifest: %v", err)
	}
	if after.Artifacts[StepRuneCLI].Path != paths.RuneCLIBinary {
		t.Errorf("audit entry missing or wrong path: %+v", after.Artifacts[StepRuneCLI])
	}
}

// The channel can move between plan time and apply time; UpdateCLI must
// re-check rather than trust the plan.
func TestUpdateCLI_RefusesNonNewerChannel(t *testing.T) {
	setRealms(t)
	t.Setenv("RUNE_MANIFEST", "")
	fx := newFixture(t)
	fx.cli = []byte("older-cli-bytes")
	fx.cliVersion = "v1.0.0"

	paths, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := paths.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
	if err := os.WriteFile(paths.RuneCLIBinary, []byte("current-cli"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Running v1.1.0 while the channel serves v1.0.0 (demoted mid-run)
	_, err = UpdateCLI(context.Background(), fx.manifestURL(), "v1.1.0", nil)
	if !errors.Is(err, ErrCLIOutdated) {
		t.Fatalf("want ErrCLIOutdated, got %v", err)
	}
	if b, _ := os.ReadFile(paths.RuneCLIBinary); string(b) != "current-cli" {
		t.Errorf("must not swap an older CLI over the running one: got %q", b)
	}

	// Equal versions are likewise nothing to do
	if _, err := UpdateCLI(context.Background(), fx.manifestURL(), "v1.0.0", nil); !errors.Is(err, ErrCLIOutdated) {
		t.Errorf("equal version: want ErrCLIOutdated, got %v", err)
	}
}

func TestChannelBehind(t *testing.T) {
	cases := []struct {
		name       string
		cliVersion string
		manifest   *Manifest
		want       bool
	}{
		{"channel older than build", "v1.0.0", &Manifest{CLIVersion: "v0.9.0"}, true},
		{"channel predates self-update", "v1.0.0", &Manifest{}, true},
		{"channel equal", "v1.0.0", &Manifest{CLIVersion: "v1.0.0"}, false},
		{"channel newer", "v1.0.0", &Manifest{CLIVersion: "v1.1.0"}, false},
		{"unknown running version", "", &Manifest{}, false},
		{"nil manifest", "v1.0.0", nil, false},
	}
	for _, c := range cases {
		if got := ChannelBehind(c.manifest, c.cliVersion); got != c.want {
			t.Errorf("%s: ChannelBehind = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestUpdateCLI_NoCLIInManifest(t *testing.T) {
	setRealms(t)
	t.Setenv("RUNE_MANIFEST", "")
	fx := newFixture(t) // fixture without cliVersion omits cli_version

	_, err := UpdateCLI(context.Background(), fx.manifestURL(), "v1.0.0", nil)
	if err == nil || !strings.Contains(err.Error(), "cli_version") {
		t.Fatalf("want no-cli_version error, got %v", err)
	}
}

func TestUpdateCLI_ChecksumMismatchDoesNotSwap(t *testing.T) {
	setRealms(t)
	t.Setenv("RUNE_MANIFEST", "")
	fastRetry(t)
	fx := newFixture(t)
	fx.cli = []byte("good-cli-bytes")
	fx.cliVersion = "v9.9.9"
	fx.mismatchStep = StepRuneCLI

	paths, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := paths.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
	if err := os.WriteFile(paths.RuneCLIBinary, []byte("current-cli"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := UpdateCLI(context.Background(), fx.manifestURL(), "v1.0.0", nil); err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if b, _ := os.ReadFile(paths.RuneCLIBinary); string(b) != "current-cli" {
		t.Errorf("binary must not swap on checksum mismatch: got %q", b)
	}
}

func outdatedRuneMCP(t *testing.T) (*Paths, string) {
	t.Helper()
	setRealms(t)
	t.Setenv("RUNE_MANIFEST", "")
	fx := newFixture(t) // v0.1.0-test

	paths, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := paths.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}

	rec := &Manifest{Version: 1, RuneMCPVersion: "v0.0.1", RunedVersion: "v0.1.0-test"}
	arts := map[string]InstalledArtifact{
		StepRuneMCP: {Path: paths.RuneMCPBinary},
		StepRuned:   {Path: paths.RunedBinary},
	}
	if err := WriteInstalledManifest(paths, fx.manifestURL(), rec, arts); err != nil {
		t.Fatalf("WriteInstalledManifest: %v", err)
	}

	return paths, fx.manifestURL()
}

func TestUpdateArtifact_AfterInstallOK(t *testing.T) {
	paths, url := outdatedRuneMCP(t)

	called := false
	v, err := UpdateArtifact(context.Background(), url, StepRuneMCP, func() error { called = true; return nil }, nil)
	if err != nil {
		t.Fatalf("UpdateArtifact: %v", err)
	}
	if !called {
		t.Error("afterInstall was not called")
	}
	if v != "v0.1.0-test" {
		t.Errorf("version = %q, want v0.1.0-test", v)
	}

	after, _ := ReadInstalledManifest(paths)
	if after.RuneMCPVersion != "v0.1.0-test" {
		t.Errorf("audit not bumped after afterInstall ok: %q", after.RuneMCPVersion)
	}
}

func TestUpdateArtifact_AfterInstallErrorSkipsAudit(t *testing.T) {
	paths, url := outdatedRuneMCP(t)

	_, err := UpdateArtifact(context.Background(), url, StepRuneMCP, func() error {
		return errors.New("restart failed")
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "restart failed") {
		t.Fatalf("err = %v, want the afterInstall error surfaced", err)
	}

	if b, _ := os.ReadFile(paths.RuneMCPBinary); string(b) == "" {
		t.Error("binary should have been staged before afterInstall")
	}
	after, _ := ReadInstalledManifest(paths)
	if after.RuneMCPVersion != "v0.0.1" {
		t.Errorf("rune_mcp_version = %q, want unchanged v0.0.1 (afterInstall failed)", after.RuneMCPVersion)
	}
}
