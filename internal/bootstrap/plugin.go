package bootstrap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const envPluginRoot = "CLAUDE_PLUGIN_ROOT"

func InstalledPluginVersion(root string) (string, error) {
	if root == "" {
		root = os.Getenv(envPluginRoot)
	}
	if root == "" {
		return "", nil // unknown;
	}

	path := filepath.Join(root, ".claude-plugin", "plugin.json") // <root>/.claude-plugin/plugin.json
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("plugin manifest: read %s: %w", path, err)
	}

	var pm struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &pm); err != nil {
		return "", fmt.Errorf("plugin manifest: parse %s: %w", path, err)
	}

	return pm.Version, nil // ("", nil) returned if plugin root is unknown, not error
}

type PluginCompatibility struct {
	Installed    string `json:"installed,omitempty"`     // installed plugin version ("" = unknown)
	Expected     string `json:"expected,omitempty"`      // manifest plugin_version ("" = not declared)
	Minimum      string `json:"minimum,omitempty"`       // manifest min_plugin_version ("" = not declared)
	Known        bool   `json:"known"`                   // installed version checked
	Behind       bool   `json:"behind,omitempty"`        // installed < expected
	BelowMinimum bool   `json:"below_minimum,omitempty"` // installed < minimum
}

func EvaluatePluginCompatibility(installed string, m *Manifest) PluginCompatibility {
	pc := PluginCompatibility{Installed: installed, Known: installed != ""}
	if m != nil {
		pc.Expected = m.PluginVersion
		pc.Minimum = m.MinPluginVersion
	}

	if !pc.Known {
		return pc
	}
	if pc.Expected != "" && compareVersions(installed, pc.Expected) < 0 {
		pc.Behind = true
	}
	if pc.Minimum != "" && compareVersions(installed, pc.Minimum) < 0 {
		pc.BelowMinimum = true
	}

	return pc
}

// Semver comparison
func compareVersions(a, b string) int {
	a, b = normalizeVersion(a), normalizeVersion(b)
	aRel, aPre := cutPrerelease(a)
	bRel, bPre := cutPrerelease(b)

	if c := compareDotted(aRel, bRel, true); c != 0 {
		return c
	}

	switch {
	case aPre == "" && bPre == "":
		return 0
	case aPre == "": // a: release >  b: pre-release
		return 1
	case bPre == "":
		return -1
	default:
		return compareDotted(aPre, bPre, false)
	}
}

func cutPrerelease(v string) (release, prerelease string) {
	if i := strings.IndexByte(v, '-'); i >= 0 {
		return v[:i], v[i+1:]
	}
	return v, ""
}

func compareDotted(a, b string, numericPad bool) int {
	// Missing field is treated as 0 -> "1.2" == "1.2.0"
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}

	for i := 0; i < n; i++ {
		var x, y string
		if i < len(as) {
			x = as[i]
		}
		if i < len(bs) {
			y = bs[i]
		}
		if x == y {
			continue
		}

		if numericPad {
			if x == "" {
				x = "0"
			}
			if y == "" {
				y = "0"
			}
		} else {
			if x == "" {
				return -1
			}
			if y == "" {
				return 1
			}
		}

		xi, xerr := strconv.Atoi(x)
		yi, yerr := strconv.Atoi(y)
		switch {
		case xerr == nil && yerr == nil:
			if xi != yi {
				if xi < yi {
					return -1
				}
				return 1
			}
		case xerr == nil: // numeric identifier < non-numeric
			return -1
		case yerr == nil:
			return 1
		default:
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}
