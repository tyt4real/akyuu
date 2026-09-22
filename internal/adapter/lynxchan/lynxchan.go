// Package lynxchan implements the Adapter interface for LynxChan and its forks
// (lainchan, leftypol, alogs.space, bandada, homurachan, ...) using the native
// ?json=1 / .json feed.
//
// Field availability on this platform:
//   - tripcode:  yes (name!ident@host)
//   - poster_id: yes, via poster_id/poster_hash when enabled by the board
//   - country:   yes, via country/flag fields when the board shows flags
//   - sage:      name == "sage"
//   - epoch:     yes (time / last_mod)
//   - thumb URL: constructed at <board>/thumb/<tim>.png (LynxChan re-encodes
//     every thumbnail to png); some forks expose file/thumb URLs in the JSON
//     `files` array, which are preferred when present.
package lynxchan

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"akyuu/internal/adapter/httpclient"
	"akyuu/internal/adapter/model"
	"akyuu/internal/adapter/vichan"
	"akyuu/internal/parser"
)

// Adapter is a LynxChan platform implementation. LynxChan serves a JSON feed
// via ?json=1 when the instance enables it; some instances ship with it
// disabled and render the same theme as vichan forks. html is used as a
// fallback for those instances.
type Adapter struct {
	client *httpclient.Client
	logger *slog.Logger
	html   *vichan.Adapter
}

// New builds a LynxChan adapter rooted at baseURL.
func New(baseURL, userAgent string, transport http.RoundTripper, logger *slog.Logger) (*Adapter, error) {
	html, err := vichan.New(baseURL, userAgent, transport, logger)
	if err != nil {
		return nil, err
	}
	return &Adapter{
		client: httpclient.New(baseURL, userAgent, transport),
		logger: logger,
		html:   html,
	}, nil
}

// PlatformName implements Adapter.
func (a *Adapter) PlatformName() string { return "lynxchan" }

// CatalogURL implements Adapter.
func (a *Adapter) CatalogURL(board string) string {
	return a.client.BaseURL() + "/" + board + "/?json=1"
}

// lynxPost is one post from the LynxChan JSON feed. Numeric fields are kept as
// raw messages because forks alternate between numbers and strings.
type lynxPost struct {
	No         json.RawMessage `json:"no"`
	Resto      json.RawMessage `json:"resto"`
	Sub        string          `json:"sub"`
	Com        string          `json:"com"`
	Name       string          `json:"name"`
	Trip       string          `json:"trip"`
	Capcode    string          `json:"capcode"`
	PosterID   string          `json:"poster_id"`
	PosterHash string          `json:"poster_hash"`
	Time       json.RawMessage `json:"time"`
	LastMod    json.RawMessage `json:"last_mod"`
	Country    string          `json:"country"`
	Flag       string          `json:"flag"`
	Filename   string          `json:"filename"`
	Ext        string          `json:"ext"`
	W          int             `json:"w"`
	H          int             `json:"h"`
	Fsize      int64           `json:"fsize"`
	Size       int64           `json:"size"`
	Tim        json.RawMessage `json:"tim"`
	MD5        string          `json:"md5"`
	SHA1       string          `json:"sha1"`
	FileHash   string          `json:"filehash"`
	Spoiler    bool            `json:"spoiler"`
	ComNum     int             `json:"com_num"`
	Board      string          `json:"board"`
	Files      []lynxFile      `json:"files"`
	Sticky     bool            `json:"sticky"`
	Locked     bool            `json:"locked"`
	Bumplocked bool            `json:"bumplocked"`
	Archived   bool            `json:"archived"`
	Endless    bool            `json:"endless"`
}

// lynxFile is an entry of the multi-file posts' `files` array. When present it
// carries concrete URLs, avoiding the need to reconstruct them.
type lynxFile struct {
	OriginalName string `json:"original_name"`
	Filename     string `json:"filename"`
	Ext          string `json:"ext"`
	MD5          string `json:"md5"`
	SHA1         string `json:"sha1"`
	Size         int64  `json:"size"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Thumb        string `json:"thumb"`
	File         string `json:"file"`
}

// lynxThread is a thread object as returned by the board catalog feed.
type lynxThread struct {
	Op        json.RawMessage `json:"op"`
	Posts     []lynxPost      `json:"posts"`
	LastBump  int64           `json:"last_bump"`
	PostCount int             `json:"post_count"`
	FileCount int             `json:"file_count"`
	Sticky    bool            `json:"sticky"`
	Locked    bool            `json:"locked"`
	Archived  bool            `json:"archived"`
}

// FetchCatalog implements Adapter.
func (a *Adapter) FetchCatalog(ctx context.Context, board string) ([]model.ThreadSummary, error) {
	data, err := a.client.GetBytes(ctx, board+"/?json=1")
	if err != nil {
		return nil, fmt.Errorf("lynxchan: catalog %s: %w", board, err)
	}
	var resp struct {
		Threads json.RawMessage `json:"threads"`
		Posts   json.RawMessage `json:"posts"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		// The instance has the JSON feed disabled (serves HTML instead);
		// fall back to the shared vichan-style HTML parser.
		a.logger.Debug("lynxchan: json feed unavailable, falling back to HTML", "board", board)
		return a.html.FetchCatalog(ctx, board)
	}

	seen := map[string]bool{}
	var out []model.ThreadSummary
	add := func(s model.ThreadSummary) {
		if s.ThreadID == "" || seen[s.ThreadID] {
			return
		}
		seen[s.ThreadID] = true
		out = append(out, s)
	}

	// Shape A: "threads" as an array of thread objects, each with a posts list.
	if len(resp.Threads) > 0 {
		var arr []lynxThread
		if err := json.Unmarshal(resp.Threads, &arr); err == nil {
			for _, t := range arr {
				if len(t.Posts) > 0 {
					add(threadSummary(board, t.Posts[0], t))
				}
			}
		} else {
			// Shape B: "threads" as a map of id -> thread object.
			var m map[string]lynxThread
			if err := json.Unmarshal(resp.Threads, &m); err == nil {
				for _, t := range m {
					if len(t.Op) > 0 {
						op, err := parsePost(t.Op)
						if err == nil {
							add(threadSummaryFromPost(board, op, t))
						}
					}
				}
			}
		}
	}

	// Shape C: flat "posts" array; ops are the resto==0 entries.
	if len(resp.Posts) > 0 {
		var posts []lynxPost
		if err := json.Unmarshal(resp.Posts, &posts); err == nil {
			for _, p := range posts {
				if isOP(p) {
					var t lynxThread
					t.LastBump = jsonInt(p.LastMod)
					t.PostCount = p.ComNum
					add(threadSummaryFromPost(board, &p, t))
				}
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("lynxchan: catalog %s: no threads in response", board)
	}
	return out, nil
}

// FetchThread implements Adapter.
func (a *Adapter) FetchThread(ctx context.Context, board, threadID string) (*model.Thread, error) {
	data, err := a.client.GetBytes(ctx, board+"/res/"+threadID+".json")
	if err != nil {
		return nil, fmt.Errorf("lynxchan: thread %s/%s: %w", board, threadID, err)
	}
	var resp struct {
		Posts []lynxPost      `json:"posts"`
		Op    json.RawMessage `json:"op"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		// JSON feed disabled: delegate to the HTML fallback.
		a.logger.Debug("lynxchan: json feed unavailable, falling back to HTML", "board", board, "thread", threadID)
		return a.html.FetchThread(ctx, board, threadID)
	}

	var posts []lynxPost
	if len(resp.Op) > 0 {
		if op, err := parsePost(resp.Op); err == nil {
			posts = append(posts, *op)
		}
	}
	posts = append(posts, resp.Posts...)
	if len(posts) == 0 {
		return nil, fmt.Errorf("lynxchan: thread %s/%s: no posts in response", board, threadID)
	}

	var op *lynxPost
	for i := range posts {
		if isOP(posts[i]) {
			op = &posts[i]
			break
		}
	}
	if op == nil {
		op = &posts[0]
	}

	th := &model.Thread{
		Board:    board,
		ThreadID: threadID,
		Subject:  op.Sub,
		Sticky:   op.Sticky,
		Locked:   op.Locked || op.Bumplocked,
		Archived: op.Archived,
		LastBump: jsonInt(op.LastMod),
	}
	for i := range posts {
		p, err := a.parse(ctx, board, threadID, &posts[i])
		if err != nil {
			a.logger.Warn("lynxchan: skipping unparsable post", "board", board, "err", err)
			continue
		}
		th.Posts = append(th.Posts, p)
	}
	return th, nil
}

// ParsePost implements Adapter (single raw post JSON, or an HTML post
// fragment when the JSON feed is disabled).
func (a *Adapter) ParsePost(raw []byte) (*model.Post, error) {
	p, err := parsePost(raw)
	if err != nil {
		return a.html.ParsePost(raw)
	}
	return a.parse(context.Background(), p.Board, textOf(p.Resto), p)
}

func (a *Adapter) parse(_ context.Context, board, threadID string, raw *lynxPost) (*model.Post, error) {
	out := &model.Post{
		NativeID:   textOf(raw.No),
		ThreadID:   threadID,
		ParentID:   textOf(raw.Resto),
		Name:       raw.Name,
		Tripcode:   raw.Trip,
		Capcode:    raw.Capcode,
		PosterID:   firstNonEmpty(raw.PosterID, raw.PosterHash),
		Country:    raw.Country,
		Flag:       raw.Flag,
		Timestamp:  jsonInt(raw.Time),
		Subject:    raw.Sub,
		CommentRaw: raw.Com,
	}
	if out.ParentID == "" || out.ParentID == "0" {
		out.ParentID = ""
	}
	out.Sage = parser.IsSage(out.Name)
	out.Quotes = parser.ExtractQuotesWith(raw.Com, nil, nil)
	parsed, err := parser.Sanitize(raw.Com)
	if err != nil {
		parsed = raw.Com
	}
	out.CommentHTML = parsed
	out.Files = a.files(board, raw)
	return out, nil
}

func (a *Adapter) files(board string, p *lynxPost) []*model.File {
	if len(p.Files) > 0 {
		var out []*model.File
		for _, f := range p.Files {
			full := f.File
			if full == "" {
				full = board + "/src/" + f.Filename + f.Ext
			}
			thumb := f.Thumb
			if thumb == "" {
				thumb = board + "/thumb/" + f.Filename + ".png"
			}
			out = append(out, &model.File{
				OriginalFilename: f.OriginalName,
				ServerFilename:   f.Filename + f.Ext,
				Ext:              f.Ext,
				SizeBytes:        f.Size,
				Width:            pos(f.Width),
				Height:           pos(f.Height),
				FullURL:          a.client.Resolve(full),
				ThumbURL:         a.client.Resolve(thumb),
				MD5:              f.MD5,
				SHA1:             f.SHA1,
			})
		}
		return out
	}

	server := textOf(p.Tim)
	if server == "" {
		return nil
	}
	full := board + "/src/" + server + p.Ext
	thumb := board + "/thumb/" + server + ".png"
	size := p.Fsize
	if size == 0 {
		size = p.Size
	}
	return []*model.File{{
		OriginalFilename: p.Filename,
		ServerFilename:   server + p.Ext,
		Ext:              p.Ext,
		SizeBytes:        size,
		Width:            pos(p.W),
		Height:           pos(p.H),
		FullURL:          a.client.Resolve(full),
		ThumbURL:         a.client.Resolve(thumb),
		MD5:              firstNonEmpty(p.MD5, ""),
		SHA1:             firstNonEmpty(p.SHA1, p.FileHash),
		Spoiler:          p.Spoiler,
	}}
}

// parsePost decodes a raw single-post JSON payload.
func parsePost(raw json.RawMessage) (*lynxPost, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, fmt.Errorf("lynxchan: null post")
	}
	var p lynxPost
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("lynxchan: decode post: %w", err)
	}
	return &p, nil
}

func isOP(p lynxPost) bool {
	r := textOf(p.Resto)
	return r == "" || r == "0"
}

func threadSummary(board string, op lynxPost, t lynxThread) model.ThreadSummary {
	return model.ThreadSummary{
		ThreadID:   textOf(op.No),
		Subject:    op.Sub,
		ReplyCount: t.PostCount,
		FileCount:  t.FileCount,
		Sticky:     t.Sticky || op.Sticky,
		Locked:     t.Locked || op.Locked || op.Bumplocked,
		Archived:   t.Archived || op.Archived,
		LastBump:   maxInt64(t.LastBump, jsonInt(op.LastMod)),
	}
}

func threadSummaryFromPost(board string, op *lynxPost, t lynxThread) model.ThreadSummary {
	return threadSummary(board, *op, t)
}

func textOf(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return fmt.Sprintf("%d", n)
	}
	return string(bytes.Trim(raw, `"`))
}

func jsonInt(raw json.RawMessage) int64 {
	if len(raw) == 0 {
		return 0
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return int64(f)
	}
	return 0
}

func pos(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
