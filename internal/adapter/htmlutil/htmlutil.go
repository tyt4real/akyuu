// Package htmlutil provides DOM helpers shared by the HTML-parsing adapters
// (vichan, 4chan). It is generic infrastructure, not platform parsing logic.
package htmlutil

import (
	"io"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Parse parses an HTML document.
func Parse(r io.Reader) (*html.Node, error) {
	return html.Parse(r)
}

// Attr returns an element's attribute value.
func Attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

// HasClass reports whether an element has the given class token.
func HasClass(n *html.Node, class string) bool {
	for _, c := range strings.Fields(Attr(n, "class")) {
		if c == class {
			return true
		}
	}
	return false
}

// HasAnyClass reports whether an element has any of the given class tokens.
func HasAnyClass(n *html.Node, classes ...string) bool {
	have := strings.Fields(Attr(n, "class"))
	for _, c := range classes {
		for _, h := range have {
			if h == c {
				return true
			}
		}
	}
	return false
}

// IsTag reports whether the node is an element with the given tag name.
func IsTag(n *html.Node, tag atom.Atom) bool {
	return n.Type == html.ElementNode && n.DataAtom == tag
}

// Text returns the concatenated text of a node.
func Text(n *html.Node) string {
	var b strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.TrimSpace(b.String())
}

// InnerHTML returns the serialized inner HTML of a node.
func InnerHTML(n *html.Node) string {
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		html.Render(&b, c)
	}
	return b.String()
}

// Walk traverses the tree, calling fn for every element.
func Walk(n *html.Node, fn func(*html.Node)) {
	if n == nil {
		return
	}
	if n.Type == html.ElementNode {
		fn(n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		Walk(c, fn)
	}
}

// FindAll returns all elements matching the predicate in document order.
func FindAll(root *html.Node, fn func(*html.Node) bool) []*html.Node {
	var out []*html.Node
	Walk(root, func(n *html.Node) {
		if fn(n) {
			out = append(out, n)
		}
	})
	return out
}

// FindFirst returns the first matching element, or nil.
func FindFirst(root *html.Node, fn func(*html.Node) bool) *html.Node {
	for _, n := range FindAll(root, fn) {
		return n
	}
	return nil
}

// DescendantByClass finds the first descendant element with the given class.
func DescendantByClass(root *html.Node, class string) *html.Node {
	return FindFirst(root, func(n *html.Node) bool {
		return n.Type == html.ElementNode && HasClass(n, class)
	})
}

// IsDigits reports whether s is a non-empty run of decimal digits.
func IsDigits(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
