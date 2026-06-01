package bootstrap

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Unapck tarPath into destDir
func ExtractTarball(tarPath, destDir string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return fmt.Errorf("extract: open %s: %w", tarPath, err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("extract: gzip %s: %w", tarPath, err)
	}
	defer gz.Close()

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("extract: mkdir %s: %w", destDir, err)
	}

	destAbs, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("extract: abs %s: %w", destDir, err)
	}

	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("extract: read entry: %w", err)
		}

		target, err := safeJoin(destAbs, h.Name) // check path-traversal safety
		if err != nil {
			return err
		}

		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("extract: mkdir %s: %w", target, err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("extract: mkdir parent of %s: %w", target, err)
			}

			mode := os.FileMode(h.Mode) & 0o755
			if mode == 0 {
				mode = 0o644
			}

			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
			if err != nil {
				return fmt.Errorf("extract: open %s: %w", target, err)
			}

			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return fmt.Errorf("extract: write %s: %w", target, err)
			}

			if err := out.Close(); err != nil {
				return fmt.Errorf("extract: close %s: %w", target, err)
			}
		default:
			// non-regular binareis (symlink, devices, etc) are skipped
		}
	}
}

func safeJoin(destAbs, name string) (string, error) {
	path := filepath.FromSlash(name)
	if !filepath.IsLocal(path) {
		return "", fmt.Errorf("extract: non-local entry path %q not allowed", name)
	}

	return filepath.Join(destAbs, path), nil
}
