package htmlutil

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func mustParse(t *testing.T, s string) *html.Node {
	t.Helper()
	n, err := Parse(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// firstByTag returns the first element with the given tag under root.
func firstByTag(t *testing.T, root *html.Node, tag atom.Atom) *html.Node {
	t.Helper()
	el := FindFirst(root, func(n *html.Node) bool { return IsTag(n, tag) })
	if el == nil {
		t.Fatalf("no <%s> element found", tag)
	}
	return el
}

func TestParseInvalid(t *testing.T) {
	if _, err := Parse(strings.NewReader("")); err != nil {
		t.Fatalf("empty doc should parse: %v", err)
	}
}

func TestAttr(t *testing.T) {
	n := firstByTag(t, mustParse(t, `<div id="a" CLASS="x y" data-utc="123"></div>`), atom.Div)
	if got := Attr(n, "id"); got != "a" {
		t.Errorf("id = %q", got)
	}
	if got := Attr(n, "class"); got != "x y" {
		t.Errorf("class = %q", got)
	}
	if got := Attr(n, "missing"); got != "" {
		t.Errorf("missing = %q", got)
	}
}

func TestHasClass(t *testing.T) {
	n := firstByTag(t, mustParse(t, `<div class="op  post  reply  stickied "></div>`), atom.Div)
	for _, c := range []string{"op", "post", "reply", "stickied"} {
		if !HasClass(n, c) {
			t.Errorf("HasClass(%q) = false", c)
		}
	}
	if HasClass(n, "openn") {
		t.Error("HasClass(openn) = true")
	}
}

func TestHasAnyClass(t *testing.T) {
	n := firstByTag(t, mustParse(t, `<div class="a b"></div>`), atom.Div)
	if !HasAnyClass(n, "zz", "b") {
		t.Error("HasAnyClass should match 'b'")
	}
	if HasAnyClass(n, "zz", "yy") {
		t.Error("HasAnyClass matched nothing")
	}
}

func TestIsTag(t *testing.T) {
	n := mustParse(t, `<span><div></div></span>`)
	htmlEl := FindFirst(n, func(x *html.Node) bool { return IsTag(x, atom.Html) })
	if htmlEl == nil {
		t.Fatal("no <html> element found")
	}
	div := FindFirst(n, func(x *html.Node) bool { return IsTag(x, atom.Div) })
	if div == nil {
		t.Fatal("no <div> found")
	}
	if IsTag(div, atom.Span) {
		t.Error("div reported as span")
	}
}

func TestText(t *testing.T) {
	n := mustParse(t, `<div>hello <b>world</b>  <br>again</div>`)
	got := Text(n)
	if got != "hello world  again" {
		t.Errorf("Text = %q", got)
	}
}

func TestInnerHTML(t *testing.T) {
	n := mustParse(t, `<div><p>a</p><span>b</span></div>`)
	got := InnerHTML(n)
	if !strings.Contains(got, "<p>a</p>") || !strings.Contains(got, "<span>b</span>") {
		t.Errorf("InnerHTML = %q", got)
	}
}

func TestWalkAndFind(t *testing.T) {
	n := mustParse(t, `<div><a class="x"></a><p><a class="y"></a></p><a></a></div>`)
	var links []*html.Node
	Walk(n, func(x *html.Node) {
		if IsTag(x, atom.A) {
			links = append(links, x)
		}
	})
	if len(links) != 3 {
		t.Fatalf("Walk found %d <a> tags, want 3", len(links))
	}

	all := FindAll(n, func(x *html.Node) bool { return IsTag(x, atom.A) })
	if len(all) != 3 {
		t.Errorf("FindAll found %d", len(all))
	}
	first := FindFirst(n, func(x *html.Node) bool { return IsTag(x, atom.A) && HasClass(x, "x") })
	if first == nil {
		t.Error("FindFirst missed class x")
	}
	if FindFirst(n, func(x *html.Node) bool { return IsTag(x, atom.Section) }) != nil {
		t.Error("FindFirst matched nonexistent <section>")
	}
	// Walk(nil) must not panic.
	Walk(nil, func(*html.Node) {})
	if FindAll(nil, func(*html.Node) bool { return true }) != nil {
		t.Error("FindAll(nil) should return nil")
	}
}

func TestDescendantByClass(t *testing.T) {
	n := mustParse(t, `<div class="wrap"><div class="inner"><span class="target">t</span></div></div>`)
	d := DescendantByClass(n, "target")
	if d == nil || d.DataAtom != atom.Span {
		t.Fatalf("DescendantByClass = %+v", d)
	}
	if DescendantByClass(n, "nope") != nil {
		t.Error("DescendantByClass matched nothing")
	}
}

func TestIsDigits(t *testing.T) {
	for _, s := range []string{"12345", " 007 ", "0"} {
		if !IsDigits(s) {
			t.Errorf("IsDigits(%q) = false", s)
		}
	}
	for _, s := range []string{"", "  ", "12a", "-1", "1.5", "abc"} {
		if IsDigits(s) {
			t.Errorf("IsDigits(%q) = true", s)
		}
	}
}
