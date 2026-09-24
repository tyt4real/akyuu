package embedder

import (
	"regexp"
	"strings"
)

// Canonicalize reduces text to a normalized form for dedup lookup: this is
// deliberately more aggressive than CleanText, which preserves meaning for
// embedding. Canonicalize is only for the exact-match dedup table.
func Canonicalize(cleaned string) string {
	t := strings.ToLower(strings.TrimSpace(cleaned))
	t = trailingPunct.ReplaceAllString(t, "")
	return strings.Join(strings.Fields(t), " ")
}

var trailingPunct = regexp.MustCompile(`[.!?,;:]+$`)
