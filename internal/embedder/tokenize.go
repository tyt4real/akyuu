package embedder

import (
	"unicode"
)

// -- WordPiece tokenizer (BERT-family models: all-MiniLM-L6-v2, bge-small) --

const (
	clsToken = "[CLS]"
	sepToken = "[SEP]"
	unkToken = "[UNK]"
	padToken = "[PAD]"
)

// wordPiece is a minimal tokenizer for sentence-transformers BERT exports. It
// mirrors the HuggingFace BertPreTokenizer split + WordPiece subword matching
// that all-MiniLM-L6-v2 and bge-small-en use: whitespace/punctuation/CJK
// tokenization, lowercase, ## continuation subwords, CLS/SEP framing.
type wordPiece struct {
	vocab       map[string]int32
	unkID       int64
	padID       int64
	clsID       int64
	sepID       int64
	maxLen      int
	doLowerCase bool
}

func newWordPiece(vocab map[string]int32, doLowerCase bool, maxLen int) *wordPiece {
	t := &wordPiece{vocab: vocab, doLowerCase: doLowerCase, maxLen: maxLen}
	t.unkID = t.idOr(unkToken, 100)
	t.padID = t.idOr(padToken, 0)
	t.clsID = t.idOr(clsToken, 101)
	t.sepID = t.idOr(sepToken, 102)
	return t
}

func (t *wordPiece) idOr(token string, fallback int64) int64 {
	if id, ok := t.vocab[token]; ok {
		return int64(id)
	}
	return fallback
}

// tokenize returns input_ids plus an attention mask, framed with [CLS]/[SEP]
// and truncated/padded to maxLen.
func (t *wordPiece) tokenize(text string) (ids []int64, mask []int64) {
	var tokens []int64
	for _, raw := range preTokenize(text) {
		if t.doLowerCase {
			raw = lowerRunes(raw)
		}
		tokens = append(tokens, t.subword(raw)...)
		if len(tokens) >= t.maxLen-2 {
			break
		}
	}
	if len(tokens) > t.maxLen-2 {
		tokens = tokens[:t.maxLen-2]
	}
	seq := make([]int64, 0, len(tokens)+2)
	seq = append(seq, t.clsID)
	seq = append(seq, tokens...)
	seq = append(seq, t.sepID)
	// Pad to maxLen with pad tokens.
	ids = make([]int64, t.maxLen)
	mask = make([]int64, t.maxLen)
	for i, id := range seq {
		if i >= t.maxLen {
			break
		}
		ids[i] = id
		mask[i] = 1
	}
	for i := len(seq); i < t.maxLen; i++ {
		ids[i] = t.padID
	}
	return ids, mask
}

// subword splits one pre-token into WordPiece pieces using longest-match
// against the vocab, emitting [UNK] for unknown characters.
func (t *wordPiece) subword(token string) []int64 {
	if id, ok := t.vocab[token]; ok {
		return []int64{int64(id)}
	}
	var out []int64
	start := 0
	for start < len(token) {
		end := len(token)
		piece := ""
		for ; end > start; end-- {
			sub := token[start:end]
			if start > 0 {
				sub = "##" + sub
			}
			if _, ok := t.vocab[sub]; ok {
				piece = sub
				break
			}
		}
		if piece == "" {
			// Unknown byte run: consume one rune and emit [UNK].
			out = append(out, t.unkID)
			start += runeLen(token[start:])
			continue
		}
		out = append(out, int64(t.vocab[piece]))
		start = end
	}
	return out
}

// preTokenize splits text into raw tokens on whitespace, punctuation and CJK
// boundaries, mirroring BertPreTokenizer.
func preTokenize(text string) []string {
	var tokens []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			tokens = append(tokens, string(cur))
			cur = nil
		}
	}
	for _, r := range text {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			flush()
			continue
		}
		if unicode.IsPunct(r) || isCJK(r) {
			flush()
			tokens = append(tokens, string(r))
			continue
		}
		cur = append(cur, r)
	}
	flush()
	return tokens
}

// isCJK reports whether r is a CJK ideograph or hangul syllable that BERT
// treats as a standalone token.
func isCJK(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) ||
		(r >= 0x3400 && r <= 0x4DBF) ||
		(r >= 0xAC00 && r <= 0xD7AF)
}

func lowerRunes(s string) string {
	rs := []rune(s)
	for i, r := range rs {
		rs[i] = unicode.ToLower(r)
	}
	return string(rs)
}

func runeLen(s string) int {
	for i := range s {
		return i + 1
	}
	return 1
}
