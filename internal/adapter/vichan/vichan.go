// Package vichan implements the Adapter interface for vichan and its forks
// (Tinyboard, burichan, 4chon, 75chan, wired-7, ...) by parsing their
// server-rendered HTML. These engines expose no JSON API.
//
// Field availability on this platform:
//   - tripcode:   yes, !hash (some forks append a host fragment)
//   - poster_id:  no (vichan does not generate per-post IDs)
//   - capcode:    yes, for moderator posts (## Mod, ## Admin)
//   - country:    no in the standard theme
//   - sage:       name == "sage"
//   - epoch:      yes, when the post element carries data-time; otherwise the
//     displayed timestamp is parsed as a fallback
package vichan

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"akyuu/internal/adapter/htmlutil"
	"akyuu/internal/adapter/httpclient"
	"akyuu/internal/adapter/model"
	"akyuu/internal/parser"
)

// Adapter is a vichan platform implementation.
type Adapter struct {
	client *httpclient.Client
	logger *slog.Logger
}

// New builds a vichan adapter rooted at baseURL.
func New(baseURL, userAgent string, transport http.RoundTripper, logger *slog.Logger) (*Adapter, error) {
	return &Adapter{
		client: httpclient.New(baseURL, userAgent, transport),
		logger: logger,
	}, nil
}

// PlatformName implements Adapter.
func (a *Adapter) PlatformName() string { return "vichan" }

// CatalogURL implements Adapter.
func (a *Adapter) CatalogURL(board string) string {
	return a.client.BaseURL() + "/" + board + "/"
}

var (
	replyCountRe = regexp.MustCompile(`(?i)(\d+)\s+(?:replies?|posts?|respuestas?|responders?)`)
	fileCountRe  = regexp.MustCompile(`(?i)(\d+)\s+(?:images?|files?|imágenes|imagenes|imagenes?)`)
	fileSizeRe   = regexp.MustCompile(`(?i)([\d.,]+)\s*(kb|mb|gb)`)
	dimsRe       = regexp.MustCompile(`(\d+)\s*[x×]\s*(\d+)`)
)

// FetchCatalog implements Adapter.
func (a *Adapter) FetchCatalog(ctx context.Context, board string) ([]model.ThreadSummary, error) {
	resp, err := a.client.Get(ctx, board+"/")
	if err != nil {
		return nil, fmt.Errorf("vichan: catalog %s: %w", board, err)
	}
	defer resp.Body.Close()
	doc, err := htmlutil.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("vichan: parse catalog %s: %w", board, err)
	}

	var out []model.ThreadSummary
	for _, t := range htmlutil.FindAll(doc, func(n *html.Node) bool {
		return n.Type == html.ElementNode && htmlutil.HasClass(n, "thread")
	}) {
		// The OP post is the first descendant post container.
		opNode := htmlutil.FindFirst(t, func(n *html.Node) bool {
			return n.Type == html.ElementNode && htmlutil.HasAnyClass(n, "post", "op")
		})
		if opNode == nil {
			continue
		}
		op, err := a.postFromNode(board, "", opNode)
		if err != nil {
			a.logger.Warn("vichan: skip thread card", "board", board, "err", err)
			continue
		}
		text := htmlutil.Text(t)
		summary := model.ThreadSummary{
			ThreadID: op.NativeID,
			Subject:  op.Subject,
			Sticky: htmlutil.FindFirst(t, func(n *html.Node) bool {
				return n.Type == html.ElementNode &&
					(htmlutil.HasClass(n, "sticky") || strings.Contains(strings.ToLower(htmlutil.Text(n)), "[fijado]") || strings.Contains(htmlutil.Text(n), "[Sticky]"))
			}) != nil,
			Locked: htmlutil.FindFirst(t, func(n *html.Node) bool {
				return n.Type == html.ElementNode &&
					(htmlutil.HasClass(n, "locked") || strings.Contains(htmlutil.Text(n), "[Locked]"))
			}) != nil,
			Archived: strings.Contains(strings.ToLower(text), "[archive]"),
			LastBump: op.Timestamp,
		}
		countText := htmlutil.Text(t)
		if countEl := htmlutil.FindFirst(t, func(n *html.Node) bool {
			return n.Type == html.ElementNode &&
				htmlutil.HasAnyClass(n, "summary", "threadstats", "omitted", "replies")
		}); countEl != nil {
			countText = htmlutil.Text(countEl)
		}
		if m := replyCountRe.FindStringSubmatch(countText); m != nil {
			summary.ReplyCount, _ = strconv.Atoi(m[1])
		}
		if m := fileCountRe.FindStringSubmatch(countText); m != nil {
			summary.FileCount, _ = strconv.Atoi(m[1])
		}
		out = append(out, summary)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("vichan: catalog %s: no threads found", board)
	}
	return out, nil
}

// FetchThread implements Adapter.
func (a *Adapter) FetchThread(ctx context.Context, board, threadID string) (*model.Thread, error) {
	resp, err := a.client.Get(ctx, board+"/res/"+threadID+".html")
	if err != nil {
		return nil, fmt.Errorf("vichan: thread %s/%s: %w", board, threadID, err)
	}
	defer resp.Body.Close()
	doc, err := htmlutil.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("vichan: parse thread %s/%s: %w", board, threadID, err)
	}

	th := &model.Thread{Board: board, ThreadID: threadID}
	for _, n := range htmlutil.FindAll(doc, func(n *html.Node) bool {
		return n.Type == html.ElementNode && htmlutil.HasClass(n, "post")
	}) {
		p, err := a.postFromNode(board, threadID, n)
		if err != nil {
			a.logger.Warn("vichan: skip post", "board", board, "thread", threadID, "err", err)
			continue
		}
		if p.NativeID == threadID {
			th.Subject = p.Subject
			th.Sticky = htmlutil.HasClass(n, "sticky")
			th.Locked = htmlutil.HasClass(n, "locked")
			th.LastBump = p.Timestamp
		}
		th.Posts = append(th.Posts, p)
	}
	if len(th.Posts) == 0 {
		return nil, fmt.Errorf("vichan: thread %s/%s: no posts found", board, threadID)
	}
	return th, nil
}

// ParsePost implements Adapter (single post HTML fragment).
func (a *Adapter) ParsePost(raw []byte) (*model.Post, error) {
	doc, err := htmlutil.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	n := htmlutil.FindFirst(doc, func(n *html.Node) bool {
		return n.Type == html.ElementNode && htmlutil.HasClass(n, "post")
	})
	if n == nil {
		return nil, fmt.Errorf("vichan: no post container in fragment")
	}
	return a.postFromNode("", "", n)
}

func (a *Adapter) postFromNode(board, threadID string, n *html.Node) (*model.Post, error) {
	no := a.postNumber(n)
	if no == "" {
		return nil, fmt.Errorf("vichan: post without number")
	}

	p := &model.Post{
		NativeID:  no,
		ThreadID:  threadID,
		Timestamp: parseTimestamp(n),
	}

	if p.Timestamp == 0 {
		p.Timestamp = parser.ParseDisplayDate(htmlutil.Text(container(n)))
	}

	if node := htmlutil.FindFirst(n, func(x *html.Node) bool {
		return x.Type == html.ElementNode && htmlutil.HasAnyClass(x, "name", "postername", "poster_name")
	}); node != nil {
		p.Name = htmlutil.Text(node)
	}
	if node := htmlutil.FindFirst(n, func(x *html.Node) bool {
		return x.Type == html.ElementNode && htmlutil.HasAnyClass(x, "postertrip", "trip")
	}); node != nil {
		p.Tripcode = htmlutil.Text(node)
	}
	if node := htmlutil.FindFirst(n, func(x *html.Node) bool {
		return x.Type == html.ElementNode && htmlutil.HasClass(x, "capcode")
	}); node != nil {
		p.Capcode = htmlutil.Text(node)
	}
	if node := htmlutil.FindFirst(n, func(x *html.Node) bool {
		return x.Type == html.ElementNode && htmlutil.HasClass(x, "subject")
	}); node != nil {
		p.Subject = htmlutil.Text(node)
	}

	commentNode := htmlutil.FindFirst(n, func(x *html.Node) bool {
		return x.Type == html.ElementNode && htmlutil.HasAnyClass(x, "com", "postMessage", "comment", "message", "body")
	})
	if commentNode != nil {
		p.CommentRaw = htmlutil.InnerHTML(commentNode)
		parsed, err := parser.Sanitize(p.CommentRaw)
		if err != nil {
			parsed = p.CommentRaw
		}
		p.CommentHTML = parsed
		p.Quotes = parser.ExtractQuotesWith(p.CommentRaw, nil, nil)
	}
	p.Sage = parser.IsSage(p.Name)

	if node := htmlutil.FindFirst(n, func(x *html.Node) bool {
		return x.Type == html.ElementNode && htmlutil.HasAnyClass(x, "poster_id", "posterhash", "id")
	}); node != nil {
		p.PosterID = htmlutil.Text(node)
	}

	p.Files = a.files(n)
	if p.ParentID == "" {
		p.ParentID = ""
	}
	if p.NativeID != threadID && threadID != "" {
		p.ParentID = threadID
	}
	return p, nil
}

// files extracts the file block of a post. vichan renders:
//
//	File: <a href="src/<tim><ext>">name</a> (size, dims)
//	<a href="src/<tim><ext>"><img class="thumb" src="thumb/<tim>.."></a>
func (a *Adapter) files(n *html.Node) []*model.File {
	var fullNode, thumbNode *html.Node
	for _, x := range htmlutil.FindAll(n, func(x *html.Node) bool {
		return x.Type == html.ElementNode && htmlutil.IsTag(x, atom.A)
	}) {
		href := htmlutil.Attr(x, "href")
		if strings.Contains(href, "/src/") || strings.Contains(href, "/file/") || strings.Contains(href, "src=") {
			if fullNode == nil {
				fullNode = x
			}
		} else if strings.Contains(href, "/thumb/") {
			if thumbNode == nil {
				thumbNode = x
			}
		}
	}
	// Some themes only render a thumb anchor wrapping the image, or the thumb
	// is an <img> nested inside the full-size anchor with no separate /thumb/
	// link (class may be "thumb" or "post-image"). Catch both regardless of
	// whether loop 1 already found the full link.
	for _, x := range htmlutil.FindAll(n, func(x *html.Node) bool {
		if x.Type != html.ElementNode || !htmlutil.IsTag(x, atom.Img) {
			return false
		}
		if htmlutil.HasClass(x, "thumb") {
			return true
		}
		src := htmlutil.Attr(x, "src")
		return strings.Contains(src, "/thumb/") || strings.Contains(src, "/file/")
	}) {
		src := htmlutil.Attr(x, "src")
		if strings.Contains(src, "/thumb/") || strings.Contains(src, "/file/") {
			if thumbNode == nil {
				thumbNode = x
			}
		}
		if fullNode == nil {
			parent := x.Parent
			if parent != nil && strings.Contains(htmlutil.Attr(parent, "href"), "/src/") {
				fullNode = parent
			}
		}
	}
	if fullNode == nil && thumbNode == nil {
		return nil
	}

	fullURL := ""
	if fullNode != nil {
		fullURL = a.client.Resolve(htmlutil.Attr(fullNode, "href"))
		if fullURL == "" {
			fullURL = a.client.Resolve(htmlutil.Attr(fullNode, "src"))
		}
	}
	thumbURL := ""
	if thumbNode != nil {
		if htmlutil.IsTag(thumbNode, atom.Img) {
			thumbURL = a.client.Resolve(htmlutil.Attr(thumbNode, "src"))
		} else {
			thumbURL = a.client.Resolve(htmlutil.Attr(thumbNode, "href"))
		}
	}
	if fullURL == "" {
		// The thumb anchor is also the full link on some themes.
		if thumbNode != nil && !htmlutil.IsTag(thumbNode, atom.Img) {
			fullURL = thumbURL
		}
	}

	f := &model.File{
		FullURL:  fullURL,
		ThumbURL: thumbURL,
	}
	// Filename from the full URL path.
	if f.ServerFilename == "" && fullURL != "" {
		if i := strings.LastIndex(fullURL, "/"); i >= 0 {
			f.ServerFilename = fullURL[i+1:]
			if j := strings.LastIndex(f.ServerFilename, "?"); j >= 0 {
				f.ServerFilename = f.ServerFilename[:j]
			}
		}
		if j := strings.LastIndex(f.ServerFilename, "."); j >= 0 {
			f.Ext = f.ServerFilename[j:]
		}
	}
	// Original filename + size/dims from the file info text.
	info := htmlutil.Text(n)
	parent := fullNode
	if parent == nil {
		parent = thumbNode
	}
	if parent != nil {
		info = htmlutil.Text(parent.Parent)
	}
	if m := fileSizeRe.FindStringSubmatch(info); m != nil {
		f.SizeBytes = parser.ParseFileSize(m[1], m[2])
	}
	if m := dimsRe.FindStringSubmatch(info); m != nil {
		f.Width, _ = strconv.Atoi(m[1])
		f.Height, _ = strconv.Atoi(m[2])
	}
	f.Spoiler = htmlutil.FindFirst(n, func(x *html.Node) bool {
		return x.Type == html.ElementNode && htmlutil.HasClass(x, "spoiler")
	}) != nil
	return []*model.File{f}
}

// container returns the enclosing block whose text contains the displayed
// timestamp, used as the fallback date source.
func container(n *html.Node) *html.Node {
	for p := n.Parent; p != nil; p = p.Parent {
		if htmlutil.HasClass(p, "post") || htmlutil.IsTag(p, atom.Body) || htmlutil.IsTag(p, atom.Html) {
			return p
		}
	}
	return n
}

func parseTimestamp(n *html.Node) int64 {
	if v := htmlutil.Attr(n, "data-time"); v != "" {
		if ts, err := strconv.ParseInt(v, 10, 64); err == nil {
			return ts
		}
	}
	// Modern vichan themes render the epoch-less <time datetime="RFC3339">.
	if t := htmlutil.FindFirst(n, func(x *html.Node) bool {
		return x.Type == html.ElementNode && htmlutil.IsTag(x, atom.Time)
	}); t != nil {
		if v := htmlutil.Attr(t, "datetime"); v != "" {
			if ts, err := time.Parse(time.RFC3339, v); err == nil {
				return ts.Unix()
			}
		}
	}
	return 0
}

// postNumber resolves a post's number from (in order): data-no, id="op_<n>",
// id="reply_<n>", or the digit-only [No.] link.
func (a *Adapter) postNumber(n *html.Node) string {
	if no := htmlutil.Attr(n, "data-no"); no != "" {
		return no
	}
	if id := htmlutil.Attr(n, "id"); id != "" {
		for _, prefix := range []string{"op_", "reply_", "post_", "p"} {
			if strings.HasPrefix(id, prefix) {
				if num := strings.TrimPrefix(id, prefix); num != "" && isDigits(num) {
					return num
				}
			}
		}
	}
	if link := htmlutil.FindFirst(n, func(x *html.Node) bool {
		return x.Type == html.ElementNode && htmlutil.IsTag(x, atom.A) && isDigits(htmlutil.Text(x))
	}); link != nil {
		return htmlutil.Text(link)
	}
	return ""
}

func isDigits(s string) bool {
	return htmlutil.IsDigits(s)
}
