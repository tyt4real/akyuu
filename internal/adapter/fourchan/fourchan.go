// Package fourchan implements the Adapter interface for 4chan by parsing the
// server-rendered HTML served at boards.4chan.org. The classic 4chan engine
// exposes no JSON via ?json=1 (the API lives on a.4cdn.org and is out of scope
// of the provided samples), so HTML parsing is used.
//
// Field availability on this platform:
//   - tripcode:   yes (!!hash)
//   - poster_id:  yes (poster_hash, rendered as "ID:xxxx")
//   - capcode:    yes (## Mod / ## Admin)
//   - country:    not available in the HTML theme
//   - sage:       name == "sage"
//   - epoch:      yes, data-time attribute on the post element
//   - file hashes are not exposed in the HTML theme; blobs are keyed by the
//     sha256 we compute on download, so dedup across sites still works
package fourchan

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"akyuu/internal/adapter/htmlutil"
	"akyuu/internal/adapter/httpclient"
	"akyuu/internal/adapter/model"
	"akyuu/internal/parser"
)

// Adapter is the 4chan platform implementation.
type Adapter struct {
	client *httpclient.Client
	logger *slog.Logger
}

// New builds a 4chan adapter rooted at baseURL (typically
// https://boards.4chan.org).
func New(baseURL, userAgent string, transport http.RoundTripper, logger *slog.Logger) (*Adapter, error) {
	return &Adapter{
		client: httpclient.New(baseURL, userAgent, transport),
		logger: logger,
	}, nil
}

// PlatformName implements Adapter.
func (a *Adapter) PlatformName() string { return "fourchan" }

const fileHost = "https://i.4cdn.org"

var (
	replyCountRe = regexp.MustCompile(`(?i)(\d+)\s+replies?`)
	fileCountRe  = regexp.MustCompile(`(?i)(\d+)\s+(?:images?|files?)`)
	fileSizeRe   = regexp.MustCompile(`(?i)([\d.,]+)\s*(kb|mb|gb)`)
	dimsRe       = regexp.MustCompile(`(\d+)\s*[x×]\s*(\d+)`)
)

// FetchCatalog implements Adapter.
func (a *Adapter) FetchCatalog(ctx context.Context, board string) ([]model.ThreadSummary, error) {
	resp, err := a.client.Get(ctx, board+"/")
	if err != nil {
		return nil, fmt.Errorf("fourchan: catalog %s: %w", board, err)
	}
	defer resp.Body.Close()
	doc, err := htmlutil.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fourchan: parse catalog %s: %w", board, err)
	}

	var out []model.ThreadSummary
	for _, t := range htmlutil.FindAll(doc, func(n *html.Node) bool {
		return n.Type == html.ElementNode && htmlutil.HasClass(n, "thread")
	}) {
		opNode := htmlutil.FindFirst(t, func(n *html.Node) bool {
			return n.Type == html.ElementNode && htmlutil.HasClass(n, "op")
		})
		if opNode == nil {
			continue
		}
		op, err := a.postFromNode(board, "", opNode)
		if err != nil {
			a.logger.Warn("fourchan: skip thread card", "board", board, "err", err)
			continue
		}
		text := htmlutil.Text(t)
		s := model.ThreadSummary{
			ThreadID: op.NativeID,
			Subject:  op.Subject,
			Sticky: htmlutil.FindFirst(t, func(n *html.Node) bool {
				return n.Type == html.ElementNode && htmlutil.HasClass(n, "sticky")
			}) != nil,
			Locked: htmlutil.FindFirst(t, func(n *html.Node) bool {
				return n.Type == html.ElementNode && htmlutil.HasClass(n, "locked")
			}) != nil,
			Archived: strings.Contains(strings.ToLower(text), "archive"),
			LastBump: op.Timestamp,
		}
		// Reply/image counts come from the card's summary line, not the whole
		// card: OP bodies may contain the literal text "N replies".
		countText := ""
		if countEl := htmlutil.FindFirst(t, func(n *html.Node) bool {
			return n.Type == html.ElementNode &&
				htmlutil.HasAnyClass(n, "summary", "threadstats", "omitted", "replies")
		}); countEl != nil {
			countText = htmlutil.Text(countEl)
		}
		if countText == "" {
			countText = htmlutil.Text(t)
		}
		if m := replyCountRe.FindStringSubmatch(countText); m != nil {
			s.ReplyCount, _ = strconv.Atoi(m[1])
		}
		if m := fileCountRe.FindStringSubmatch(countText); m != nil {
			s.FileCount, _ = strconv.Atoi(m[1])
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("fourchan: catalog %s: no threads found", board)
	}
	return out, nil
}

// FetchThread implements Adapter.
func (a *Adapter) FetchThread(ctx context.Context, board, threadID string) (*model.Thread, error) {
	resp, err := a.client.Get(ctx, board+"/thread/"+threadID)
	if err != nil {
		return nil, fmt.Errorf("fourchan: thread %s/%s: %w", board, threadID, err)
	}
	defer resp.Body.Close()
	doc, err := htmlutil.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fourchan: parse thread %s/%s: %w", board, threadID, err)
	}

	th := &model.Thread{Board: board, ThreadID: threadID}
	for _, n := range htmlutil.FindAll(doc, func(n *html.Node) bool {
		return n.Type == html.ElementNode && htmlutil.HasClass(n, "post")
	}) {
		p, err := a.postFromNode(board, threadID, n)
		if err != nil {
			a.logger.Warn("fourchan: skip post", "board", board, "thread", threadID, "err", err)
			continue
		}
		if p.NativeID == threadID {
			th.Subject = p.Subject
			th.Sticky = htmlutil.HasClass(n, "sticky")
			th.Locked = htmlutil.HasClass(n, "locked")
			th.Archived = htmlutil.HasClass(n, "archived")
			th.LastBump = p.Timestamp
		}
		th.Posts = append(th.Posts, p)
	}
	if len(th.Posts) == 0 {
		return nil, fmt.Errorf("fourchan: thread %s/%s: no posts found", board, threadID)
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
		return nil, fmt.Errorf("fourchan: no post container in fragment")
	}
	return a.postFromNode("", "", n)
}

func (a *Adapter) postFromNode(board, threadID string, n *html.Node) (*model.Post, error) {
	no := a.postNumber(n)
	if no == "" {
		return nil, fmt.Errorf("fourchan: post without number")
	}

	p := &model.Post{
		NativeID:  no,
		ThreadID:  threadID,
		Timestamp: parseTimestamp(n),
	}

	if node := htmlutil.FindFirst(n, func(x *html.Node) bool {
		return x.Type == html.ElementNode && htmlutil.HasAnyClass(x, "name", "poster_name")
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
	if node := htmlutil.FindFirst(n, func(x *html.Node) bool {
		return x.Type == html.ElementNode && htmlutil.HasAnyClass(x, "poster_hash", "poster_id", "id")
	}); node != nil {
		p.PosterID = strings.TrimPrefix(htmlutil.Text(node), "ID:")
	}

	commentNode := htmlutil.FindFirst(n, func(x *html.Node) bool {
		return x.Type == html.ElementNode && htmlutil.HasAnyClass(x, "com", "postMessage", "comment", "message")
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

	p.Files = a.files(n)
	if p.NativeID != threadID && threadID != "" {
		p.ParentID = threadID
	}
	return p, nil
}

func (a *Adapter) files(n *html.Node) []*model.File {
	fileText := htmlutil.FindFirst(n, func(x *html.Node) bool {
		return x.Type == html.ElementNode && htmlutil.HasClass(x, "fileText")
	})
	// Current 4chan HTML wraps the thumb in an anchor with the full link:
	// <a class="fileThumb" href="FULL"><img src="THUMB" data-md5="base64"></a>.
	// Older markup used <img class="fileThumb" src="...">.
	thumbEl := htmlutil.FindFirst(n, func(x *html.Node) bool {
		return x.Type == html.ElementNode && htmlutil.HasClass(x, "fileThumb")
	})
	if fileText == nil && thumbEl == nil {
		return nil
	}

	f := &model.File{}
	if fileText != nil {
		if link := htmlutil.FindFirst(fileText, func(x *html.Node) bool {
			return x.Type == html.ElementNode && htmlutil.IsTag(x, atom.A) && htmlutil.Attr(x, "href") != ""
		}); link != nil {
			f.FullURL = a.resolveFile(htmlutil.Attr(link, "href"))
		}
		info := htmlutil.Text(fileText)
		if m := fileSizeRe.FindStringSubmatch(info); m != nil {
			f.SizeBytes = parser.ParseFileSize(m[1], m[2])
		}
		if m := dimsRe.FindStringSubmatch(info); m != nil {
			f.Width, _ = strconv.Atoi(m[1])
			f.Height, _ = strconv.Atoi(m[2])
		}
		// Original filename is the anchor's text on 4chan.
		if link := htmlutil.FindFirst(fileText, func(x *html.Node) bool {
			return x.Type == html.ElementNode && htmlutil.IsTag(x, atom.A)
		}); link != nil {
			f.OriginalFilename = htmlutil.Text(link)
		}
	}
	if thumbEl != nil {
		if htmlutil.IsTag(thumbEl, atom.A) {
			// Anchor wraps the <img>. The thumb URL and md5 live on the img.
			if f.FullURL == "" {
				f.FullURL = a.resolveFile(htmlutil.Attr(thumbEl, "href"))
			}
			if img := htmlutil.FindFirst(thumbEl, func(x *html.Node) bool {
				return x.Type == html.ElementNode && htmlutil.IsTag(x, atom.Img)
			}); img != nil {
				f.ThumbURL = a.resolveFile(htmlutil.Attr(img, "src"))
				if md5 := htmlutil.Attr(img, "data-md5"); md5 != "" {
					if raw, err := base64.StdEncoding.DecodeString(md5); err == nil {
						f.MD5 = hex.EncodeToString(raw)
					}
				}
			}
		} else {
			f.ThumbURL = a.resolveFile(htmlutil.Attr(thumbEl, "src"))
			if f.FullURL == "" {
				if p := thumbEl.Parent; p != nil {
					f.FullURL = a.resolveFile(htmlutil.Attr(p, "href"))
				}
			}
		}
	}
	if f.ServerFilename == "" && f.FullURL != "" {
		if i := strings.LastIndex(f.FullURL, "/"); i >= 0 {
			f.ServerFilename = f.FullURL[i+1:]
		}
		if j := strings.LastIndex(f.ServerFilename, "."); j >= 0 {
			f.Ext = f.ServerFilename[j:]
		}
	}
	if f.FullURL == "" && f.ThumbURL != "" {
		// Some themes render only a thumb; derive the full URL from it.
		f.FullURL = strings.TrimSuffix(f.ThumbURL, "s.jpg") + strings.TrimSuffix(f.Ext, "")
	}
	f.Spoiler = htmlutil.FindFirst(n, func(x *html.Node) bool {
		return x.Type == html.ElementNode && htmlutil.HasClass(x, "spoiler")
	}) != nil
	return []*model.File{f}
}

// resolveFile turns a 4chan file URL (absolute, "//i.4cdn.org/...", or
// "/<board>/...") into an absolute URL on the i.4cdn.org file host.
func (a *Adapter) resolveFile(u string) string {
	u = strings.TrimSpace(u)
	switch {
	case strings.HasPrefix(u, "http://"), strings.HasPrefix(u, "https://"):
		return u
	case strings.HasPrefix(u, "//"):
		return "https:" + u
	case strings.HasPrefix(u, "/"):
		return fileHost + u
	}
	return fileHost + "/" + u
}

func parseTimestamp(n *html.Node) int64 {
	if v := htmlutil.Attr(n, "data-time"); v != "" {
		if ts, err := strconv.ParseInt(v, 10, 64); err == nil {
			return ts
		}
	}
	// Current 4chan HTML renders the epoch on the dateTime element:
	// <span class="dateTime" data-utc="1745612650">.
	if dt := htmlutil.FindFirst(n, func(x *html.Node) bool {
		return x.Type == html.ElementNode && htmlutil.HasClass(x, "dateTime")
	}); dt != nil {
		if v := htmlutil.Attr(dt, "data-utc"); v != "" {
			if ts, err := strconv.ParseInt(v, 10, 64); err == nil {
				return ts
			}
		}
	}
	return 0
}

// postNumber resolves a post's number from (in order): data-no, id="p<num>",
// the digit-only permalink/reply link, or the javascript:quote('num') link.
func (a *Adapter) postNumber(n *html.Node) string {
	if no := htmlutil.Attr(n, "data-no"); no != "" {
		return no
	}
	if id := htmlutil.Attr(n, "id"); strings.HasPrefix(id, "p") {
		if num := strings.TrimPrefix(id, "p"); num != "" && htmlutil.IsDigits(num) {
			return num
		}
	}
	if link := htmlutil.FindFirst(n, func(x *html.Node) bool {
		if x.Type != html.ElementNode || !htmlutil.IsTag(x, atom.A) {
			return false
		}
		href := htmlutil.Attr(x, "href")
		if strings.HasPrefix(href, "javascript:quote(") {
			return htmlutil.IsDigits(strings.TrimSuffix(strings.TrimPrefix(href, "javascript:quote('"), "');"))
		}
		return (strings.HasPrefix(href, "#p") || strings.HasPrefix(href, "#q") ||
			strings.Contains(href, "thread/")) && htmlutil.IsDigits(htmlutil.Text(x))
	}); link != nil {
		href := htmlutil.Attr(link, "href")
		if strings.HasPrefix(href, "javascript:quote(") {
			return strings.TrimSuffix(strings.TrimPrefix(href, "javascript:quote('"), "');")
		}
		return htmlutil.Text(link)
	}
	return ""
}
