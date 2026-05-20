package lifecycle

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/envector/rune-go/internal/domain"
)

func sampleBootError(detail string) *domain.BootError {
	return &domain.BootError{
		Kind:     domain.BootErrVaultNetwork,
		Detail:   detail,
		Hint:     "check the endpoint",
		Phase:    domain.BootPhaseVaultDial,
		At:       time.Now(),
		Attempts: 1,
	}
}

func TestBootLogger_PersistWritesJSONLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "boot.log")
	bl := NewBootLogger(path, DefaultBootLogMaxBytes)
	defer bl.Close()

	bl.Persist(sampleBootError("vault unreachable"))

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read boot log: %v", err)
	}
	var entry bootLogEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &entry); err != nil {
		t.Fatalf("unmarshal line: %v (raw=%q)", err, data)
	}
	if entry.Kind != domain.BootErrVaultNetwork {
		t.Errorf("kind: got %q, want %q", entry.Kind, domain.BootErrVaultNetwork)
	}
	if entry.Detail != "vault unreachable" {
		t.Errorf("detail: got %q", entry.Detail)
	}
}

func TestBootLogger_NilReceiverIsNoop(t *testing.T) {
	var bl *BootLogger // nil — simulates Manager with bootLog unset
	// None of these should panic.
	bl.Persist(sampleBootError("x"))
	if got := bl.Path(); got != "" {
		t.Errorf("nil Path(): got %q, want empty", got)
	}
	if err := bl.Close(); err != nil {
		t.Errorf("nil Close(): %v", err)
	}
}

func TestBootLogger_NilErrorIsNoop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "boot.log")
	bl := NewBootLogger(path, DefaultBootLogMaxBytes)
	defer bl.Close()

	bl.Persist(nil)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("nil BootError should not create the file (stat err=%v)", err)
	}
}

func TestBootLogger_CloseIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "boot.log")
	bl := NewBootLogger(path, DefaultBootLogMaxBytes)
	bl.Persist(sampleBootError("once"))

	if err := bl.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := bl.Close(); err != nil {
		t.Errorf("second Close should be no-op: %v", err)
	}
}

func TestBootLogger_RotatesWhenExceedingMaxBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "boot.log")
	// Tiny threshold so a couple of entries force rotation.
	bl := NewBootLogger(path, 200)
	defer bl.Close()

	// Each entry's JSON line is ~150+ bytes, so the second/third writes cross
	// the 200-byte threshold and rotate.
	bl.Persist(sampleBootError("first-entry-aaaaaaaaaaaaaaaaaaaa"))
	bl.Persist(sampleBootError("second-entry-bbbbbbbbbbbbbbbbbbbb"))
	bl.Persist(sampleBootError("third-entry-cccccccccccccccccccc"))

	// Rotation produced a ".1" sidecar holding earlier generation(s).
	rotated := path + ".1"
	if _, err := os.Stat(rotated); err != nil {
		t.Fatalf("expected rotated file %s: %v", rotated, err)
	}

	// Current file holds the most recent write and is under the threshold.
	cur, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read current: %v", err)
	}
	if int64(len(cur)) > 200 {
		t.Errorf("current file size %d exceeds maxBytes 200 — rotation didn't reset", len(cur))
	}
	if !strings.Contains(string(cur), "third-entry") {
		t.Errorf("current file should hold the latest entry; got %q", cur)
	}

	// Every line across both files must be valid JSON (no torn writes).
	for _, f := range []string{rotated, path} {
		b, _ := os.ReadFile(f)
		sc := bufio.NewScanner(strings.NewReader(string(b)))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			var e bootLogEntry
			if err := json.Unmarshal([]byte(line), &e); err != nil {
				t.Errorf("%s: invalid JSON line %q: %v", f, line, err)
			}
		}
	}
}
