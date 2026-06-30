package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/CryptoLabInc/rune-cli/internal/bootstrap"
)

func TestExtractDetachFlag(t *testing.T) {
	cases := []struct {
		name       string
		input      []string
		wantDetach bool
		wantRest   []string
	}{
		{
			name:       "absent",
			input:      []string{"--foreground", "--verbose"},
			wantDetach: false,
			wantRest:   []string{"--foreground", "--verbose"},
		},
		{
			name:       "double-dash form",
			input:      []string{"--detach"},
			wantDetach: true,
			wantRest:   []string{},
		},
		{
			name:       "single-dash form",
			input:      []string{"-detach"},
			wantDetach: true,
			wantRest:   []string{},
		},
		{
			name:       "mixed flags",
			input:      []string{"--detach", "--foreground", "--log-level=debug"},
			wantDetach: true,
			wantRest:   []string{"--foreground", "--log-level=debug"},
		},
		{
			name:       "no args",
			input:      []string{},
			wantDetach: false,
			wantRest:   []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotDetach, gotRest := extractDetachFlag(tc.input)
			if gotDetach != tc.wantDetach {
				t.Errorf("detach = %v, want %v", gotDetach, tc.wantDetach)
			}
			if !reflect.DeepEqual(gotRest, tc.wantRest) {
				t.Errorf("rest = %#v, want %#v", gotRest, tc.wantRest)
			}
		})
	}
}

func runedEnv(t *testing.T) *bootstrap.Paths {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("RUNE_HOME", filepath.Join(dir, "rune"))
	t.Setenv("RUNED_HOME", filepath.Join(dir, "runed"))

	paths, err := bootstrap.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	return paths
}

func TestRunRuned_StatusNotRunning(t *testing.T) {
	runedEnv(t) // no supervisor listening

	var stdout, stderr bytes.Buffer
	if code := runRuned(context.Background(), []string{"--status"}, &stdout, &stderr); code != 1 {
		t.Errorf("exit = %d, want 1 (no supervisor)", code)
	}
	if !strings.Contains(stdout.String(), "not running") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestRunRuned_StatusRunning(t *testing.T) {
	paths := runedEnv(t)

	// Simulate listener
	ln, err := net.Listen("unix", paths.SupervisorSock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		var req map[string]any
		_ = json.NewDecoder(conn).Decode(&req)
		_ = json.NewEncoder(conn).Encode(map[string]any{"ok": true, "pid": 4321})
	}()

	var stdout, stderr bytes.Buffer
	if code := runRuned(context.Background(), []string{"--status"}, &stdout, &stderr); code != 0 {
		t.Errorf("exit = %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "running") || !strings.Contains(stdout.String(), "4321") {
		t.Errorf("stdout = %q, want running + pid 4321", stdout.String())
	}
}
