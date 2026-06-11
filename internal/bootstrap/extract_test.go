package bootstrap

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func makeTarball(t *testing.T, path string, files map[string]struct {
	body []byte
	mode int64
}) {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	for name, f := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     f.mode,
			Size:     int64(len(f.body)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatalf("tar header: %v", err)
		}

		if _, err := tw.Write(f.body); err != nil {
			t.Fatalf("tar write: %v", err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gz close: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write tarball: %v", err)
	}
}

func TestExtractTarball_RunedStackLayout(t *testing.T) {
	dir := t.TempDir()
	tarPath := filepath.Join(dir, "runed-test.tar.gz")
	dest := filepath.Join(dir, "out")

	makeTarball(t, tarPath, map[string]struct {
		body []byte
		mode int64
	}{
		"runed":        {body: []byte("RUNED-BIN"), mode: 0o755},
		"llama-server": {body: []byte("LLAMA-BIN"), mode: 0o755},
	})

	if err := ExtractTarball(tarPath, dest); err != nil {
		t.Fatalf("ExtractTarball: %v", err)
	}

	for _, name := range []string{"runed", "llama-server"} {
		p := filepath.Join(dest, name)
		info, err := os.Stat(p)
		if err != nil {
			t.Errorf("expected %s extracted: %v", name, err)
			continue
		}

		if mode := info.Mode().Perm(); mode&0o100 == 0 {
			t.Errorf("%s mode = %v, want executable bit set", name, mode)
		}
	}
}

func TestExtractTarball_RejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	tarPath := filepath.Join(dir, "malicious.tar.gz")

	makeTarball(t, tarPath, map[string]struct {
		body []byte
		mode int64
	}{
		"../escaped": {body: []byte("OUT"), mode: 0o644},
	})

	err := ExtractTarball(tarPath, filepath.Join(dir, "out"))
	if err == nil {
		t.Fatal("expected error for ../ entry, got nil")
	}
}

func TestExtractTarball_RejectsAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	tarPath := filepath.Join(dir, "abs.tar.gz")

	makeTarball(t, tarPath, map[string]struct {
		body []byte
		mode int64
	}{
		"/etc/passwd": {body: []byte("OUT"), mode: 0o644},
	})

	if err := ExtractTarball(tarPath, filepath.Join(dir, "out")); err == nil {
		t.Fatal("expected error for absolute entry path, got nil")
	}
}

func TestExtractTarball_SkipSymlink(t *testing.T) {
	dir := t.TempDir()
	tarPath := filepath.Join(dir, "symlink.tar.gz")

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	reg := []byte("REAL")

	if err := tw.WriteHeader(&tar.Header{Name: "runed", Mode: 0o755, Size: int64(len(reg)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatalf("tar header (reg): %v", err)
	}
	if _, err := tw.Write(reg); err != nil {
		t.Fatalf("tar write: %v", err)
	}
	if err := tw.WriteHeader(&tar.Header{Name: "malicious", Linkname: "/etc/passwd", Typeflag: tar.TypeSymlink}); err != nil {
		t.Fatalf("tar header (symlink): %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gz close: %v", err)
	}
	if err := os.WriteFile(tarPath, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write tarball: %v", err)
	}

	dest := filepath.Join(dir, "out")
	if err := ExtractTarball(tarPath, dest); err != nil {
		t.Fatalf("ExtractTarball should skip symlinks without error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "runed")); err != nil {
		t.Errorf("regular file should still extract alongside a skipped symlink: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dest, "malicious")); !os.IsNotExist(err) {
		t.Errorf("symlink entry must be skipped, not created; lstat err=%v", err)
	}
}
