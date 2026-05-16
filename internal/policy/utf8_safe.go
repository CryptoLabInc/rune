package policy

// truncRunes truncates s to maxRunes codepoints (Python s[:N] semantic).
// Used in place of byte slicing on record fields so multi-byte UTF-8 is
// not split mid-encoding (proto3 string-field validation rejects orphan
// continuation bytes).
func truncRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	// Byte length is an upper bound on rune count.
	if len(s) <= maxRunes {
		return s
	}
	count := 0
	for i := range s {
		if count == maxRunes {
			return s[:i]
		}
		count++
	}
	return s
}
