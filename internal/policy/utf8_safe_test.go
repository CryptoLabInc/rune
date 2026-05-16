package policy

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/envector/rune-go/internal/domain"
)

func TestTruncRunes(t *testing.T) {
	cases := []struct {
		name    string
		s       string
		n       int
		want    string
		wantLen int // expected rune count
	}{
		{"empty input", "", 5, "", 0},
		{"zero cap", "hello", 0, "", 0},
		{"negative cap", "hello", -1, "", 0},
		{"ASCII under cap", "hi", 5, "hi", 2},
		{"ASCII at cap", "hello", 5, "hello", 5},
		{"ASCII over cap", "hello world", 5, "hello", 5},
		// 1 Korean char = 3 UTF-8 bytes
		{"Korean under cap", "안녕", 5, "안녕", 2},
		{"Korean exactly at cap", "안녕하세요", 5, "안녕하세요", 5},
		{"Korean over cap", "안녕하세요세계", 5, "안녕하세요", 5},
		// Mixed
		{"mixed under cap", "안녕 hi", 10, "안녕 hi", 5},
		{"mixed cut at korean boundary", "ab안녕cd", 3, "ab안", 3},
		{"mixed cut at ascii boundary", "안녕ab", 3, "안녕a", 3},
		// 1 emoji = 4 UTF-8 bytes (1 rune)
		{"emoji over cap", "🚀🎉🌟", 2, "🚀🎉", 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := truncRunes(c.s, c.n)
			if got != c.want {
				t.Errorf("truncRunes(%q, %d) = %q, want %q", c.s, c.n, got, c.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("truncRunes(%q, %d) returned invalid UTF-8", c.s, c.n)
			}
			if utf8.RuneCountInString(got) != c.wantLen {
				t.Errorf("truncRunes(%q, %d) rune count = %d, want %d",
					c.s, c.n, utf8.RuneCountInString(got), c.wantLen)
			}
		})
	}
}

// TestRedactSensitive_PreservesUTF8 — regression for the proto3 marshal
// failure observed 2026-05-16. Korean text passing through
// RedactSensitive must remain valid UTF-8 even when truncation triggers.
func TestRedactSensitive_PreservesUTF8(t *testing.T) {
	// Mostly-Korean text padded past MaxInputChars (12000 chars).
	// Pre-fix the byte-level cut would land mid-codepoint.
	korean := "안녕하세요 세계 다양한 문자열이 들어있는 입력 텍스트 입니다 "
	long := strings.Repeat(korean, 500) // ~14000 runes, ~42000 bytes
	out, _ := RedactSensitive(long)
	if !utf8.ValidString(out) {
		t.Fatalf("RedactSensitive output is invalid UTF-8 (len=%d bytes)", len(out))
	}
	if got := utf8.RuneCountInString(out); got > MaxInputChars {
		t.Errorf("rune count %d exceeds cap %d", got, MaxInputChars)
	}
}

// TestBuildPhases_KoreanTextProducesValidUTF8 — full pipeline check.
// Korean capture text goes through extractTitle / extractEvidence /
// truncStr; the resulting record's Payload.Text must be valid UTF-8 so
// that downstream embedder.EmbedBatch marshaling succeeds.
func TestBuildPhases_KoreanTextProducesValidUTF8(t *testing.T) {
	// Real-world reproducer from session 1778902721 #17.
	text := "/rune:configure 실패 원인을 surface 하는 4-layer 변경:  " +
		"문제: /rune:configure 실패 시 (CA 인증서 불일치, 잘못된 토큰, " +
		"endpoint 도달 불가 등) rune-mcp 가 state: \"waiting_for_vault\" 만 " +
		"반환하고 실패 이유를 안 알려줌."

	extractedRaw := map[string]any{
		"domain": "rune-architecture",
		"topic":  "boot-error-classification",
		"kind":   "design-decision",
		"tags":   []any{"boot", "korean", "test"},
	}
	detection, extraction, err := domain.ParseExtractionFromAgent(extractedRaw)
	if err != nil {
		t.Fatalf("ParseExtractionFromAgent: %v", err)
	}

	rawEvent := &domain.RawEvent{
		Text:    text,
		Source:  "claude-code:/rune:capture",
		Channel: "rune-debug",
		User:    "redcourage",
	}

	records, err := BuildPhases(rawEvent, detection, extraction, time.Now())
	if err != nil {
		t.Fatalf("BuildPhases: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("BuildPhases returned 0 records")
	}
	for i, r := range records {
		if !utf8.ValidString(r.Title) {
			t.Errorf("record[%d].Title is invalid UTF-8: %q", i, r.Title)
		}
		if !utf8.ValidString(r.Payload.Text) {
			t.Errorf("record[%d].Payload.Text is invalid UTF-8 (%d bytes)", i, len(r.Payload.Text))
		}
		for j, ev := range r.Evidence {
			if !utf8.ValidString(ev.Quote) {
				t.Errorf("record[%d].Evidence[%d].Quote is invalid UTF-8", i, j)
			}
		}
	}
}
