package bootstrap

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func TestDownloadAndVerify_HappyPath(t *testing.T) {
	body := []byte("hello, runed")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")
	spec := ArtifactSpec{URL: srv.URL, SHA256: sha256Hex(body), Size: int64(len(body))}
	if err := DownloadAndVerify(context.Background(), spec, dest, nil); err != nil {
		t.Fatalf("DownloadAndVerify: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("content mismatch: %q vs %q", got, body)
	}
	if _, err := os.Stat(dest + ".partial"); !os.IsNotExist(err) {
		t.Errorf("expected no .partial leftover; got %v", err)
	}
}

func TestDownloadAndVerify_ChecksumMismatch_LeavesNoFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("actual content"))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")
	spec := ArtifactSpec{URL: srv.URL, SHA256: sha256Hex([]byte("a different body"))}
	err := DownloadAndVerify(context.Background(), spec, dest, nil)
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("want ErrChecksumMismatch, got %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Errorf("dest %s should not exist after checksum failure", dest)
	}
	if _, err := os.Stat(dest + ".partial"); !os.IsNotExist(err) {
		t.Errorf(".partial should be cleaned up after checksum failure")
	}
}

func TestDownloadAndVerify_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusGone)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")
	spec := ArtifactSpec{URL: srv.URL, SHA256: "anything"}
	err := DownloadAndVerify(context.Background(), spec, dest, nil)
	if err == nil || !strings.Contains(err.Error(), "410") {
		t.Errorf("want HTTP 410, got %v", err)
	}
}

func TestDownloadAndVerify_ProgressCallbackInvoked(t *testing.T) {
	body := bytes.Repeat([]byte("x"), 200*1024) // 200 KiB
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "204800")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")
	spec := ArtifactSpec{URL: srv.URL, SHA256: sha256Hex(body)}

	var calls int
	var lastDownloaded int64
	var lastTotal int64
	err := DownloadAndVerify(context.Background(), spec, dest, func(downloaded, total int64) {
		calls++
		lastDownloaded = downloaded
		lastTotal = total
	})
	if err != nil {
		t.Fatalf("DownloadAndVerify: %v", err)
	}
	if calls < 2 {
		t.Errorf("expected progress callback >=2 times, got %d", calls)
	}
	if lastDownloaded != int64(len(body)) {
		t.Errorf("final downloaded = %d, want %d", lastDownloaded, len(body))
	}
	if lastTotal != int64(len(body)) {
		t.Errorf("total = %d, want %d", lastTotal, len(body))
	}
}

func TestFileSHA256_KnownContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	body := []byte("rune-go")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := FileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != sha256Hex(body) {
		t.Errorf("hash mismatch: got %s", got)
	}
}

func TestFileMatchesSHA256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	body := []byte("rune-go")
	want := sha256Hex(body)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}

	ok, err := FileMatchesSHA256(path, want) // match
	if err != nil || !ok {
		t.Errorf("want ok+nil, got ok=%v err=%v", ok, err)
	}

	ok, err = FileMatchesSHA256(path, "00") // mismatch
	if err != nil || ok {
		t.Errorf("want !ok+nil, got ok=%v err=%v", ok, err)
	}

	ok, err = FileMatchesSHA256(filepath.Join(dir, "absent"), want) // missing
	if err != nil || ok {
		t.Errorf("missing should be !ok+nil, got ok=%v err=%v", ok, err)
	}
}

func makeBundleTarball(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range entries {
		hdr := &tar.Header{
			Name:     name,
			Mode:     0o755,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractBundle_HappyPath(t *testing.T) {
	tarball := makeBundleTarball(t, map[string][]byte{
		"bin/runed":        []byte("runed-binary-bytes"),
		"bin/llama-server": []byte("llama-binary-bytes"),
		"bin/rundemo":      []byte("rundemo-bytes"),
	})

	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "bundle.tar.gz")
	if err := os.WriteFile(srcPath, tarball, 0o600); err != nil {
		t.Fatal(err)
	}

	destDir := t.TempDir()
	extracted, err := ExtractBundle(srcPath, destDir)
	if err != nil {
		t.Fatalf("ExtractBundle: %v", err)
	}
	if len(extracted) != 3 {
		t.Errorf("extracted %d files, want 3 (%v)", len(extracted), extracted)
	}

	for name, want := range map[string]string{
		"runed":        "runed-binary-bytes",
		"llama-server": "llama-binary-bytes",
		"rundemo":      "rundemo-bytes",
	} {
		p := filepath.Join(destDir, name)
		got, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("read %s: %v", p, err)
			continue
		}
		if string(got) != want {
			t.Errorf("%s content = %q, want %q", name, got, want)
		}
		info, err := os.Stat(p)
		if err != nil {
			t.Errorf("stat %s: %v", p, err)
			continue
		}
		if info.Mode()&0o100 == 0 {
			t.Errorf("%s missing user-execute bit: mode=%o", name, info.Mode())
		}
	}
}

func TestExtractBundle_SkipsTopLevelAndNonBin(t *testing.T) {
	tarball := makeBundleTarball(t, map[string][]byte{
		"bin/runed": []byte("runed"),
		"README":    []byte("readme"),
		"docs/foo":  []byte("foo"),
	})
	srcPath := filepath.Join(t.TempDir(), "bundle.tar.gz")
	_ = os.WriteFile(srcPath, tarball, 0o600)
	destDir := t.TempDir()

	extracted, err := ExtractBundle(srcPath, destDir)
	if err != nil {
		t.Fatalf("ExtractBundle: %v", err)
	}
	if len(extracted) != 1 || filepath.Base(extracted[0]) != "runed" {
		t.Errorf("extracted = %v, want only runed", extracted)
	}
	if _, err := os.Stat(filepath.Join(destDir, "README")); !os.IsNotExist(err) {
		t.Errorf("README should not have been extracted")
	}
}

func TestExtractBundle_RejectsPathTraversal(t *testing.T) {
	tarball := makeBundleTarball(t, map[string][]byte{
		"bin/runed":         []byte("runed"),
		"bin/../etc/passwd": []byte("evil"),
		"bin/sub/dir/inner": []byte("nested"),
	})
	srcPath := filepath.Join(t.TempDir(), "bundle.tar.gz")
	_ = os.WriteFile(srcPath, tarball, 0o600)
	destDir := t.TempDir()

	extracted, err := ExtractBundle(srcPath, destDir)
	if err != nil {
		t.Fatalf("ExtractBundle: %v", err)
	}
	if len(extracted) != 1 {
		t.Errorf("extracted = %v, want only runed", extracted)
	}
	if _, err := os.Stat(filepath.Join(destDir, "..", "etc", "passwd")); err == nil {
		t.Errorf("traversal succeeded — security issue")
	}
}

func TestExtractBundle_EmptyTarballErrors(t *testing.T) {
	tarball := makeBundleTarball(t, map[string][]byte{
		"README": []byte("no bin/ entries here"),
	})
	srcPath := filepath.Join(t.TempDir(), "bundle.tar.gz")
	_ = os.WriteFile(srcPath, tarball, 0o600)
	destDir := t.TempDir()

	_, err := ExtractBundle(srcPath, destDir)
	if err == nil || !strings.Contains(err.Error(), "no bin/ entries") {
		t.Errorf("want no-bin-entries error, got %v", err)
	}
}
