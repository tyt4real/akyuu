package embedder

import "testing"

// miniVocab is a toy WordPiece vocabulary with the special tokens and a few
// words, used to check framing, lowercasing, subword and unknown handling
// without the real model files.
func miniVocab() map[string]int32 {
	v := map[string]int32{
		"[CLS]": 101, "[SEP]": 102, "[UNK]": 100, "[PAD]": 0,
		"hello": 7592, "world": 2088, "##s": 2015, "play": 2191, "##ing": 4147,
		"at": 2013, "!": 999, "?": 1051, "japan": 1944,
	}
	return v
}

func TestWordPieceFramingAndPadding(t *testing.T) {
	tok := newWordPiece(miniVocab(), true, 8)
	ids, mask := tok.tokenize("Hello world")
	if len(ids) != 8 || len(mask) != 8 {
		t.Fatalf("want maxLen sequences, got ids=%d mask=%d", len(ids), len(mask))
	}
	if ids[0] != 101 || ids[3] != 102 {
		t.Errorf("expected [CLS]..[SEP] framing, got %v", ids)
	}
	if ids[1] != 7592 || ids[2] != 2088 {
		t.Errorf("lowercase + vocab lookup failed, got %v", ids)
	}
	if mask[1] != 1 || mask[4] != 0 || mask[3] != 1 {
		t.Errorf("attention mask wrong: %v", mask)
	}
	if ids[4] != 0 {
		t.Errorf("expected padding after [SEP], got ids=%v", ids)
	}
}

func TestWordPieceSubwords(t *testing.T) {
	tok := newWordPiece(miniVocab(), true, 16)
	ids, _ := tok.tokenize("playing")
	// playing -> play + ##ing
	if len(ids) < 4 || ids[1] != 2191 || ids[2] != 4147 {
		t.Errorf("expected play + ##ing, got %v", ids[:4])
	}
}

func TestWordPieceUnknown(t *testing.T) {
	tok := newWordPiece(miniVocab(), true, 16)
	ids, _ := tok.tokenize("zzqq xxqq")
	if ids[1] != 100 && ids[2] != 100 {
		t.Errorf("expected unknown tokens, got %v", ids[:4])
	}
}

func TestWordPieceCJKAndPunct(t *testing.T) {
	tok := newWordPiece(miniVocab(), true, 16)
	ids, _ := tok.tokenize("at !")
	// punctuation becomes its own token; whitespace is dropped
	if ids[1] != 2013 || ids[2] != 999 {
		t.Errorf("punctuation tokenization failed: %v", ids[:4])
	}
}

func TestWordPieceTruncation(t *testing.T) {
	tok := newWordPiece(miniVocab(), true, 4)
	ids, mask := tok.tokenize("hello world at")
	// maxLen 4: [CLS] + 2 tokens + [SEP], "at" dropped
	if len(ids) != 4 {
		t.Fatalf("expected truncation to maxLen, got %v", ids)
	}
	if ids[1] != 7592 || ids[2] != 2088 || ids[3] != 102 {
		t.Errorf("wrong truncated sequence: %v", ids)
	}
	if mask[0] != 1 || mask[1] != 1 || mask[2] != 1 || mask[3] != 1 {
		t.Errorf("mask should be all ones for a full sequence: %v", mask)
	}
}

func TestPreTokenizeSplits(t *testing.T) {
	got := preTokenize("Hello, 世界! foo-bar")
	// "Hello," -> "Hello" + ",", CJK each standalone, "foo-bar" split on '-'
	want := []string{"Hello", ",", "世", "界", "!", "foo", "-", "bar"}
	if len(got) != len(want) {
		t.Fatalf("preTokenize mismatch: got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token %d: got %q want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}
