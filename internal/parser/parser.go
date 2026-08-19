// Package parser normalizes comment markup shared across platforms: quote
// references (>>123, >>>/board/123), greentext, spoilers, and HTML safety.
//
// The *syntax* of a platform's markup stays inside that platform's adapter
// (quote regexes, spoiler tag spelling differ between engines); this package
// only provides the shared processing machinery the adapters call into.
package parser

import (
	"regexp"
	"strings"

	"akyuu/internal/adapter/model"
)

// QuoteRef is a single quote reference found inside a comment. It is shared
// with the adapter model so parsed quotes can be handed straight to the store.
type QuoteRef = model.QuoteRef

// crossBoardRe matches cross-board references in both literal and HTML-entity
// encoded form: >>>/b/123, >>>/b/ 123, &gt;&gt;&gt;/b/123.
var crossBoardRe = regexp.MustCompile(`(?m)(?:&gt;&gt;&gt;|>>>)\s*/?\s*([A-Za-z0-9_+\-]+)/\s*(\d+)`)

// sameBoardRe matches same-board references: >>123 and the occasional >>>123
// (triple-arrow) spelling, in literal or entity-encoded form.
var sameBoardRe = regexp.MustCompile(`(?m)(?:&gt;&gt;&gt;|>>>|&gt;&gt;|>>)\s*(\d+)`)

// PostIDRe matches a bare same-board reference; used by ExtractQuotesWith.
var PostIDRe = regexp.MustCompile(`(?m)(?:&gt;&gt;&gt;|>>>|&gt;&gt;|>>)\s*(\d+)`)

// ExtractQuotes pulls quote references out of raw comment text, handling both
// HTML-entity-encoded arrows ("&gt;&gt;") and literal ">>", same-board and
// cross-board forms. References are returned in document order, duplicates kept
// (a post may quote the same post twice).
func ExtractQuotes(comment string) []QuoteRef {
	return ExtractQuotesWith(comment, nil, nil)
}

// ExtractQuotesWith is ExtractQuotes with caller-supplied patterns, for
// platforms whose arrow or bracket spelling differs from the default.
func ExtractQuotesWith(comment string, sameBoard, crossBoard *regexp.Regexp) []QuoteRef {
	if crossBoard == nil {
		crossBoard = crossBoardRe
	}
	if sameBoard == nil {
		sameBoard = sameBoardRe
	}
	var out []QuoteRef
	for _, m := range crossBoard.FindAllStringSubmatch(comment, -1) {
		out = append(out, QuoteRef{Board: m[1], PostID: m[2]})
	}
	// Strip cross-board matches so the same-board pass does not double-count
	// their trailing digits (e.g. ">>>/b/123" would otherwise also match ">>123").
	rest := crossBoard.ReplaceAllString(comment, "")
	for _, m := range sameBoard.FindAllStringSubmatch(rest, -1) {
		out = append(out, QuoteRef{PostID: m[1]})
	}
	return out
}

// IsSage reports whether a name field marks a "sage" post (no bump). The
// convention is consistent across vichan, 4chan and LynxChan.
func IsSage(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), "sage")
}

// HasGreentext reports whether a comment contains a greentext line. Greentext
// is a line beginning with ">" (as text or &gt;) that is not a quote ref.
func HasGreentext(comment string) bool {
	for _, line := range strings.Split(comment, "\n") {
		line = strings.TrimSpace(strings.ReplaceAll(line, "&gt;", ">"))
		if strings.HasPrefix(line, ">") && !strings.HasPrefix(line, ">>") {
			return true
		}
	}
	return false
}
