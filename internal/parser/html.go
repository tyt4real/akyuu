package parser

import (
	"strings"

	"golang.org/x/net/html"
)

// allowedTags is the whitelist kept when sanitizing comment HTML. Everything
// else is dropped (scripts, styles, iframes, forms, event handlers, ...).
var allowedTags = map[string]bool{
	"a":          true,
	"b":          true,
	"blockquote": true,
	"br":         true,
	"code":       true,
	"del":        true,
	"em":         true,
	"i":          true,
	"li":         true,
	"ol":         true,
	"p":          true,
	"pre":        true,
	"s":          true,
	"span":       true,
	"strong":     true,
	"u":          true,
	"ul":         true,
}

// allowedAttrs is the attribute whitelist kept when sanitizing. href and src
// are additionally scheme-checked in Sanitize.
var allowedAttrs = map[string]bool{
	"class":      true,
	"href":       true,
	"title":      true,
	"id":         true,
	"data-board": true,
}

// Sanitize rewrites untrusted comment HTML into safe, displayable HTML. It
// drops disallowed tags and attributes, removes script/iframe/style content,
// blocks dangerous URL schemes, and re-serializes the tree so the output is a
// canonical form suitable for comment_parsed.
func Sanitize(in string) (string, error) {
	if in == "" {
		return "", nil
	}
	doc, err := html.Parse(strings.NewReader(in))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			b.WriteString(n.Data)
			return
		case html.ElementNode:
			name := strings.ToLower(n.Data)
			if name == "script" || name == "style" || name == "iframe" ||
				name == "object" || name == "embed" || name == "form" || name == "svg" {
				return // drop the node and all its children
			}
			if !allowedTags[name] {
				// Unknown tag: keep children, drop the tag itself.
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
				return
			}
			b.WriteString("<" + name)
			for _, a := range n.Attr {
				if !allowedAttrs[strings.ToLower(a.Key)] {
					continue
				}
				val := a.Val
				switch strings.ToLower(a.Key) {
				case "href", "src":
					if !safeURL(val) {
						continue
					}
				}
				b.WriteString(` ` + a.Key + `="` + html.EscapeString(val) + `"`)
			}
			b.WriteString(">")
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
			b.WriteString("</" + name + ">")
			return
		case html.CommentNode:
			return
		case html.DoctypeNode, html.DocumentNode:
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
	}
	walk(doc)
	return b.String(), nil
}

// safeURL rejects javascript:, data: (except benign images), vbscript: and
// other dangerous URL schemes in href/src attributes.
func safeURL(u string) bool {
	u = strings.TrimSpace(u)
	if u == "" {
		return false
	}
	lower := strings.ToLower(u)
	for _, scheme := range []string{"javascript:", "vbscript:", "data:video/", "data:text/"} {
		if strings.HasPrefix(lower, scheme) {
			return false
		}
	}
	if strings.HasPrefix(lower, "data:image/") {
		return true
	}
	if strings.HasPrefix(lower, "data:") {
		return false
	}
	return true
}

// TextContent extracts the plain-text content of an HTML string, decoding
// entities. Used to derive searchable text and to normalize comment_raw.
func TextContent(in string) (string, error) {
	if in == "" {
		return "", nil
	}
	doc, err := html.Parse(strings.NewReader(in))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			return
		}
		if n.Type == html.ElementNode {
			switch strings.ToLower(n.Data) {
			case "br", "p", "div", "blockquote", "li":
				b.WriteString("\n")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return strings.TrimSpace(b.String()), nil
}
