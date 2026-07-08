package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.4.1", "0.5.0", -1},
		{"0.5.0", "0.4.1", 1},
		{"1.0.0", "1.0.0", 0},
		{"v1.2", "1.2.0", 0},          // v-prefix + zero-pad
		{"1.2.0+build.7", "1.2.0", 0}, // build metadata stripped
		{"1.10.0", "1.9.0", 1},        // numeric, not lexical
		{"2.0.0", "1.9.9", 1},
		{"1.2.0-rc.1", "1.2.0", -1}, // pre-release < release
		{"1.2.0", "1.2.0-rc.1", 1},
		{"1.2.0-rc.1", "1.2.0-rc.2", -1},
		{"1.2.0-rc.2", "1.2.0-rc.10", -1}, // numeric pre-release identifiers
		{"1.2.0-alpha", "1.2.0-beta", -1}, // lexical pre-release identifiers
	}

	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestPlugin_InstalledVersion(t *testing.T) {
	t.Run("reads version from plugin.json", func(t *testing.T) {
		root := t.TempDir()
		dir := filepath.Join(root, ".claude-plugin")

		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(`{"name":"rune","version":"0.4.1"}`), 0o644); err != nil {
			t.Fatal(err)
		}

		v, err := InstalledPluginVersion(root)
		if err != nil {
			t.Fatalf("InstalledPluginVersion: %v", err)
		}
		if v != "0.4.1" {
			t.Errorf("version = %q, want 0.4.1", v)
		}
	})

	t.Run("unknown root is not an error", func(t *testing.T) {
		t.Setenv(envPluginRoot, "")
		v, err := InstalledPluginVersion("")
		if err != nil || v != "" {
			t.Errorf("got (%q, %v), want (\"\", nil) when the plugin root is unknown", v, err)
		}
	})

	t.Run("known root but missing manifest is an error", func(t *testing.T) {
		if _, err := InstalledPluginVersion(t.TempDir()); err == nil {
			t.Error("expected an error when plugin.json is absent under a known root")
		}
	})
}

func TestPlugin_EvaluateCompatibility(t *testing.T) {
	m := &Manifest{Version: 1, PluginVersion: "0.5.0", MinPluginVersion: "0.5.0"}

	t.Run("below minimum is both behind and below-floor", func(t *testing.T) {
		pc := EvaluatePluginCompatibility("0.4.1", m)
		if !pc.Known || !pc.Behind || !pc.BelowMinimum {
			t.Errorf("compat = %+v, want known+behind+belowMinimum", pc)
		}
	})

	t.Run("at minimum and expected is clean", func(t *testing.T) {
		pc := EvaluatePluginCompatibility("0.5.0", m)
		if pc.Behind || pc.BelowMinimum {
			t.Errorf("compat = %+v, want no concern", pc)
		}
	})

	t.Run("ahead of expected is clean", func(t *testing.T) {
		pc := EvaluatePluginCompatibility("0.6.0", m)
		if pc.Behind || pc.BelowMinimum {
			t.Errorf("compat = %+v, want no concern", pc)
		}
	})

	t.Run("behind expected but at/above floor is advisory only", func(t *testing.T) {
		m2 := &Manifest{Version: 1, PluginVersion: "0.6.0", MinPluginVersion: "0.4.0"}
		pc := EvaluatePluginCompatibility("0.4.1", m2)
		if !pc.Behind || pc.BelowMinimum {
			t.Errorf("compat = %+v, want behind but not belowMinimum", pc)
		}
	})

	t.Run("unknown installed version flags nothing", func(t *testing.T) {
		pc := EvaluatePluginCompatibility("", m)
		if pc.Known || pc.Behind || pc.BelowMinimum {
			t.Errorf("compat = %+v, want unknown/no-flags", pc)
		}
	})

	t.Run("manifest without plugin fields flags nothing", func(t *testing.T) {
		pc := EvaluatePluginCompatibility("0.1.0", &Manifest{Version: 1})
		if pc.Behind || pc.BelowMinimum {
			t.Errorf("compat = %+v, want no concern when manifest omits plugin fields", pc)
		}
	})
}
