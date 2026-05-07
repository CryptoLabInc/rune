package keymanager_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/envector/rune-go/internal/adapters/config"
	"github.com/envector/rune-go/internal/adapters/keymanager"
)

// withTempHome redirects $HOME to a t.TempDir for the duration of t.
// config.RuneDir() reads $HOME, so this isolates filesystem side effects.
func withTempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

func TestSaveEncKey_WritesBytesVerbatim(t *testing.T) {
	home := withTempHome(t)

	// Simulate the exact bytes Vault would forward as manifest_json["EncKey.json"]:
	// a pyenvector KeyEnvelope JSON string. Use a representative payload that
	// includes the structural fields libevi expects (provider_meta + entries),
	// without committing to specific values for unit-test purposes.
	encKey := []byte(`{
  "provider_meta": {"name":"test","format_version":"1"},
  "entries": [{"role":"EncKey","key_data":"AAAA","metadata":{"parameter":{"Q":1,"P":1,"DB_SCALE_FACTOR":1.0,"QUERY_SCALE_FACTOR":1.0,"preset":"FGb"},"eval_mode":"rmp"}}]
}`)

	if err := keymanager.SaveEncKey("key-test", encKey); err != nil {
		t.Fatalf("SaveEncKey: %v", err)
	}

	// Cross-check: file should be byte-identical to what we passed in.
	got, err := os.ReadFile(filepath.Join(home, ".rune", "keys", "key-test", "EncKey.json"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if !bytes.Equal(got, encKey) {
		t.Errorf("file contents differ from input — keymanager must NOT transform encKey.\nwant=%q\n got=%q", encKey, got)
	}
}

func TestSaveEncKey_EmptyIsNoop(t *testing.T) {
	home := withTempHome(t)

	if err := keymanager.SaveEncKey("key-empty", nil); err != nil {
		t.Errorf("nil encKey: got error %v, want nil (no-op)", err)
	}
	if err := keymanager.SaveEncKey("key-empty", []byte{}); err != nil {
		t.Errorf("empty encKey: got error %v, want nil (no-op)", err)
	}

	// Should not have created the keys/key-empty directory either, since
	// the no-op fast-paths before MkdirAll.
	if _, err := os.Stat(filepath.Join(home, ".rune", "keys", "key-empty")); !os.IsNotExist(err) {
		t.Errorf("empty payload should not create keys dir; stat err=%v", err)
	}
}

func TestSaveEncKey_FilePerm0600(t *testing.T) {
	home := withTempHome(t)

	if err := keymanager.SaveEncKey("perm-test", []byte("x")); err != nil {
		t.Fatalf("SaveEncKey: %v", err)
	}

	encPath := filepath.Join(home, ".rune", "keys", "perm-test", "EncKey.json")
	info, err := os.Stat(encPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != config.FilePerm {
		t.Errorf("file perm: got %#o, want %#o", got, config.FilePerm)
	}
}

func TestSaveEncKey_DirPerm0700(t *testing.T) {
	home := withTempHome(t)

	if err := keymanager.SaveEncKey("dir-perm", []byte("x")); err != nil {
		t.Fatalf("SaveEncKey: %v", err)
	}

	keyDir := filepath.Join(home, ".rune", "keys", "dir-perm")
	info, err := os.Stat(keyDir)
	if err != nil {
		t.Fatalf("stat keyDir: %v", err)
	}
	if got := info.Mode().Perm(); got != config.DirPerm {
		t.Errorf("dir perm: got %#o, want %#o", got, config.DirPerm)
	}
}

func TestKeyDir_ReturnsExpectedPath(t *testing.T) {
	home := withTempHome(t)

	got, err := keymanager.KeyDir("my-key")
	if err != nil {
		t.Fatalf("KeyDir: %v", err)
	}
	want := filepath.Join(home, ".rune", "keys", "my-key")
	if got != want {
		t.Errorf("KeyDir: got %q, want %q", got, want)
	}
}

func TestSaveEncKey_OverwritesExisting(t *testing.T) {
	withTempHome(t)

	// First write
	if err := keymanager.SaveEncKey("over", []byte("old")); err != nil {
		t.Fatalf("first SaveEncKey: %v", err)
	}
	// Overwrite
	if err := keymanager.SaveEncKey("over", []byte("new content")); err != nil {
		t.Fatalf("second SaveEncKey: %v", err)
	}

	dir, _ := keymanager.KeyDir("over")
	got, err := os.ReadFile(filepath.Join(dir, "EncKey.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "new content" {
		t.Errorf("overwrite: got %q, want %q", got, "new content")
	}
}
