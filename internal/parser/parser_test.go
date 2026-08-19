package parser

import (
	"strings"
	"testing"
	"time"
)

func TestExtractQuotes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []QuoteRef
	}{
		{
			name: "bare same-board",
			in:   ">>123 hello >>456",
			want: []QuoteRef{{PostID: "123"}, {PostID: "456"}},
		},
		{
			name: "entity encoded",
			in:   "&gt;&gt;123 text",
			want: []QuoteRef{{PostID: "123"}},
		},
		{
			name: "cross-board",
			in:   ">>>/b/123 and >>>/g/ 789",
			want: []QuoteRef{{Board: "b", PostID: "123"}, {Board: "g", PostID: "789"}},
		},
		{
			name: "mixed without double count",
			in:   ">>>/tech/42 plus >>7",
			want: []QuoteRef{{Board: "tech", PostID: "42"}, {PostID: "7"}},
		},
		{
			name: "no quotes",
			in:   "plain text",
			want: nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ExtractQuotes(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("got %d quotes, want %d: %+v", len(got), len(c.want), got)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("quote[%d] = %+v, want %+v", i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestSanitizeDropsScriptsAndKeepsLinks(t *testing.T) {
	in := `<script>alert(1)</script><a href="https://example.com/x">>>123</a><a href="javascript:evil()">bad</a>><span class="quote">&gt;greentext</span>`
	out, err := Sanitize(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "script") || strings.Contains(out, "javascript") {
		t.Errorf("sanitizer kept dangerous content: %s", out)
	}
	if !strings.Contains(out, `href="https://example.com/x"`) {
		t.Errorf("sanitizer dropped a safe link: %s", out)
	}
	if !strings.Contains(out, "greentext") || !strings.Contains(out, `class="quote"`) {
		t.Errorf("sanitizer dropped text/markup: %s", out)
	}
}

func TestTextContent(t *testing.T) {
	out, err := TextContent(`<div class="com">hi<br>there <b>bold</b></div>`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "hi") || !strings.Contains(out, "bold") {
		t.Errorf("unexpected text content: %q", out)
	}
}

func TestParseDisplayDate(t *testing.T) {
	// 4chan style: MM/DD/YY(Day)HH:MM:SS. 08/16/26 (Sun) is a real Sunday.
	ts := ParseDisplayDate("08/16/26(Sun)11:08:53")
	if ts <= 0 {
		t.Fatal("4chan date not parsed")
	}
	got := time.Unix(ts, 0)
	if got.Day() != 16 || got.Month() != time.August || got.Year() != 2026 {
		t.Errorf("wrong date: %v", got)
	}

	// 75chan style: YYYY/MM/DD (Vie) HH:MM:SS.
	ts2 := ParseDisplayDate("2026/04/17 (Vie) 12:46:57")
	if ts2 <= 0 {
		t.Fatal("75chan date not parsed")
	}
	g2 := time.Unix(ts2, 0)
	if g2.Year() != 2026 || g2.Month() != time.April || g2.Day() != 17 {
		t.Errorf("wrong 75chan date: %v", g2)
	}

	// Wired-7 style DD/MM/YY (Sáb) HH:MM — weekday disambiguates to DD/MM.
	// 02/05/26 (Sáb) = 2 May 2026 (a Saturday).
	ts3 := ParseDisplayDate("02/05/26 (Sáb) 17:57")
	if ts3 <= 0 {
		t.Fatal("wired-7 date not parsed")
	}
	g3 := time.Unix(ts3, 0)
	if g3.Day() != 2 || g3.Month() != time.May {
		t.Errorf("wired-7 date should be 02 May, got %v", g3)
	}

	if ParseDisplayDate("not a date") != 0 {
		t.Error("garbage string should return 0")
	}
}

func TestIsSage(t *testing.T) {
	if !IsSage("sage") || !IsSage("SAGE") {
		t.Error("sage detection failed")
	}
	if IsSage("Anonymous") {
		t.Error("Anonymous should not be sage")
	}
}
