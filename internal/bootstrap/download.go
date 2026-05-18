package bootstrap

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

var ErrChecksumMismatch = errors.New("checksum mismatch")

type ProgressFunc func(downloaded, total int64)

func DownloadAndVerify(ctx context.Context, spec ArtifactSpec, destPath string, progress ProgressFunc) error {
	if spec.URL == "" || spec.SHA256 == "" {
		return errors.New("download: spec missing url or sha256")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, spec.URL, nil)
	if err != nil {
		return fmt.Errorf("download: build request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download: GET %s: %w", spec.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download: GET %s: HTTP %d", spec.URL, resp.StatusCode)
	}

	partial := destPath + ".partial"
	f, err := os.OpenFile(partial, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("download: open partial %s: %w", partial, err)
	}

	committed := false
	defer func() {
		_ = f.Close()
		if !committed {
			_ = os.Remove(partial)
		}
	}()

	h := sha256.New()
	total := resp.ContentLength // -1 when unknown
	written, err := streamWithProgress(resp.Body, f, h, total, progress)
	if err != nil {
		return fmt.Errorf("download: write %s: %w", partial, err)
	}
	if spec.Size > 0 && written != spec.Size {
		return fmt.Errorf("download: size mismatch: got %d bytes, manifest claims %d", written, spec.Size)
	}

	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, spec.SHA256) {
		return fmt.Errorf("%w: got %s, want %s", ErrChecksumMismatch, got, spec.SHA256)
	}

	if err := f.Sync(); err != nil {
		return fmt.Errorf("download: fsync %s: %w", partial, err)
	}
	if err := os.Rename(partial, destPath); err != nil {
		return fmt.Errorf("download: rename %s to %s: %w", partial, destPath, err)
	}
	committed = true
	return nil
}

// Return total number of bytes written
func streamWithProgress(src io.Reader, dst io.Writer, h io.Writer, total int64, progress ProgressFunc) (int64, error) {
	buf := make([]byte, 64*1024)
	var written int64
	for {
		n, rerr := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return written, werr
			}

			_, _ = h.Write(buf[:n])
			written += int64(n)
			if progress != nil {
				progress(written, total)
			}
		}

		if rerr == io.EOF {
			return written, nil
		}
		if rerr != nil {
			return written, rerr
		}
	}
}

func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("sha256 open %s: %w", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("sha256 read %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func FileMatchesSHA256(path, expected string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil // not error
		}
		return false, err
	}
	got, err := FileSHA256(path)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(got, expected), nil
}

// bin/runed, bin/llama-server
func ExtractBundle(srcPath, destDir string) ([]string, error) {
	src, err := os.Open(srcPath)
	if err != nil {
		return nil, fmt.Errorf("extract: open %s: %w", srcPath, err)
	}
	defer src.Close()

	gz, err := gzip.NewReader(src)
	if err != nil {
		return nil, fmt.Errorf("extract: gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var extracted []string
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return extracted, fmt.Errorf("extract: tar next: %w", err)
		}

		name := filepath.ToSlash(hdr.Name)
		if !strings.HasPrefix(name, "bin/") {
			continue
		}

		base := strings.TrimPrefix(name, "bin/")
		if base == "" || strings.Contains(base, "/") || strings.Contains(base, "..") {
			continue // prevent path traversal
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA {
			continue
		}

		dest := filepath.Join(destDir, base)
		if err := extractOne(tr, dest, hdr.Mode); err != nil {
			return extracted, fmt.Errorf("extract: %s: %w", base, err)
		}
		extracted = append(extracted, dest)
	}

	if len(extracted) == 0 {
		return nil, fmt.Errorf("extract: no bin/ entries found in %s", srcPath)
	}

	return extracted, nil
}

func extractOne(r io.Reader, dest string, mode int64) error {
	tmp := dest + ".extract.partial"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open tmp %s: %w", tmp, err)
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("copy: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("fsync: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	effective := os.FileMode(mode) & 0o770
	if effective == 0 {
		effective = 0o600
	}
	if err := os.Chmod(tmp, effective); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("chmod: %w", err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s to %s: %w", tmp, dest, err)
	}

	return nil
}
