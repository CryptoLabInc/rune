package bootstrap

import (
	"context"
	"os"
	"path/filepath"
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

		plan := planUpdate(installed, manifest)
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
		if planUpdate(installed, manifest).HasUpdates() {
			t.Error("v-prefix / build-metadata differences must not count as update")
		}
	})

	t.Run("prerelease difference is update", func(t *testing.T) {
		m := &Manifest{Version: 1, RuneMCPVersion: "v0.2.0-alpha.5", RunedVersion: "v0.2.0"}
		installed := &InstalledManifest{RuneMCPVersion: "v0.2.0-alpha.4", RunedVersion: "v0.2.0"}
		if !planUpdate(installed, m).HasUpdates() {
			t.Error("pre-release tags must count as update")
		}
	})

	t.Run("unknown installed is not flagged", func(t *testing.T) {
		if planUpdate(nil, manifest).HasUpdates() {
			t.Error("unknown installed versions must not be flagged")
		}
	})

	t.Run("empty installed version is not flagged", func(t *testing.T) {
		installed := &InstalledManifest{} // blank version
		if planUpdate(installed, manifest).HasUpdates() {
			t.Error("blank installed version is unknown, not outdated")
		}
	})

	t.Run("empty manifest version gives nothing", func(t *testing.T) {
		m := &Manifest{Version: 1}
		installed := &InstalledManifest{RuneMCPVersion: "v0.1.0", RunedVersion: "v0.1.0"}
		if planUpdate(installed, m).HasUpdates() {
			t.Error("empty manifest versions must not be flagged")
		}
	})

	t.Run("nil manifest (no panic, no updates)", func(t *testing.T) {
		installed := &InstalledManifest{RuneMCPVersion: "v0.1.0", RunedVersion: "v0.1.0"}
		plan := planUpdate(installed, nil)
		if plan.HasUpdates() || len(plan.Artifacts) != 0 {
			t.Errorf("nil manifest should be empty plan, got %+v", plan)
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

	plan, err := CheckUpdate(context.Background(), fx.manifestURL(), nil)
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

	plan, err := CheckUpdate(context.Background(), fx.manifestURL(), nil)
	if err != nil {
		t.Fatalf("CheckUpdate: %v", err)
	}
	if plan.HasUpdates() {
		t.Errorf("with no install audit, nothing should be outdated: %+v", plan.Artifacts)
	}
}

func TestUpdateArtifact_UpdateSingleArifact(t *testing.T) {
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

	// Simulate installed artfiact - rune-mcp: old, runed: latest
	rec := &Manifest{Version: 1, RuneMCPVersion: "v0.0.1", RunedVersion: "v0.1.0-test"}
	arts := map[string]InstalledArtifact{
		StepRuneMCP: {Path: paths.RuneMCPBinary, SHA256: "old-mcp", DestSHA256: "old-mcp"},
		StepRuned:   {Path: paths.RunedBinary, SHA256: "runed-sha", DestSHA256: "runed-sha"},
	}
	if err := WriteInstalledManifest(paths, fx.manifestURL(), rec, arts); err != nil {
		t.Fatalf("WriteInstalledManifest: %v", err)
	}

	got, err := UpdateArtifact(context.Background(), fx.manifestURL(), StepRuneMCP, nil)
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
	plan, err := CheckUpdate(context.Background(), fx.manifestURL(), nil)
	if err != nil {
		t.Fatalf("CheckUpdate: %v", err)
	}
	if plan.HasUpdates() {
		t.Errorf("rune-mcp should be up to date after UpdateArtifact: %+v", plan.Artifacts)
	}
}

func TestUpdateArtifact_UnknownArtifact(t *testing.T) {
	if _, err := UpdateArtifact(context.Background(), "http://unused", "llama-server", nil); err == nil {
		t.Error("expected error for an unknown artifact")
	}
}
