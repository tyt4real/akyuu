// Package desuarchive implements the Adapter interface for the FoolFuuka
// archive of 4chan hosted at desuarchive.org. It is a pull-only source of
// historical 4chan threads: boards auto-discover from the /archives listing
// and thread history is crawled page by page through the /index endpoint.
//
// Data model (FoolFuuka /_/api/chan/ JSON, thread-style post object):
//   - post number:   yes (num)
//   - tripcode:      yes (trip_processed)
//   - poster_id:     yes (poster_hash_processed)
//   - capcode:       yes
//   - country:       yes (poster_country)
//   - sage:          yes (email == "sage")
//   - timestamp:     yes (unix seconds)
//   - content hash:  yes (media_hash, base64 md5) so blobs dedup against the
//     same file pulled from live 4chan before any bytes are downloaded
//   - ghost posts:   subnum > 0 are skipped
//   - deleted posts: kept (deleted flag) with any media omitted
package desuarchive

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"akyuu/internal/adapter/httpclient"
	"akyuu/internal/adapter/model"
	"akyuu/internal/parser"
)

// Adapter is the desuarchive platform implementation.
type Adapter struct {
	client *httpclient.Client
	logger *slog.Logger
}

// New builds a desuarchive adapter rooted at baseURL (typically
// https://desuarchive.org).
func New(baseURL, userAgent string, transport http.RoundTripper, logger *slog.Logger) (*Adapter, error) {
	return &Adapter{
		client: httpclient.New(baseURL, userAgent, transport),
		logger: logger,
	}, nil
}

// PlatformName implements Adapter.
func (a *Adapter) PlatformName() string { return "desuarchive" }

// --- FoolFuuka /_/api/chan/ wire types ---

// chanThread is the thread endpoint response: { "<threadnum>": {op, posts} }.
type chanThread struct {
	Op    chanPost            `json:"op"`
	Posts map[string]chanPost `json:"posts"`
}

// chanIndexEntry is one key of the index endpoint response.
type chanIndexEntry struct {
	Op chanPost `json:"op"`
}

// chanPost is a single post in FoolFuuka's thread-style JSON.
type chanPost struct {
	Num            string     `json:"num"`
	ThreadNum      string     `json:"thread_num"`
	Subnum         string     `json:"subnum"`
	Timestamp      int64      `json:"timestamp"`
	Name           string     `json:"name"`
	NameProc       string     `json:"name_processed"`
	Trip           string     `json:"trip"`
	TripProc       string     `json:"trip_processed"`
	Capcode        string     `json:"capcode"`
	PosterHash     string     `json:"poster_hash"`
	PosterHashProc string     `json:"poster_hash_processed"`
	Country        string     `json:"poster_country"`
	Email          string     `json:"email"`
	Title          string     `json:"title"`
	TitleProc      string     `json:"title_processed"`
	Comment        string     `json:"comment"`
	CommentProc    string     `json:"comment_processed"`
	Sticky         flexInt    `json:"sticky"`
	Locked         flexInt    `json:"locked"`
	Deleted        flexInt    `json:"deleted"`
	NReplies       int        `json:"nreplies"`
	NImages        int        `json:"nimages"`
	Media          *chanMedia `json:"media"`
}

// chanMedia is a post's attachment. FoolFuuka encodes sizes as strings and
// hashes as base64.
type chanMedia struct {
	Media         string  `json:"media"`
	MediaOrig     string  `json:"media_orig"`
	MediaFilename string  `json:"media_filename"`
	MediaLink     string  `json:"media_link"`
	ThumbLink     string  `json:"thumb_link"`
	MediaHash     string  `json:"media_hash"`
	MediaSize     flexInt `json:"media_size"`
	MediaW        flexInt `json:"media_w"`
	MediaH        flexInt `json:"media_h"`
	MediaStatus   string  `json:"media_status"`
	Spoiler       string  `json:"spoiler"`
}

// flexInt unmarshals integer fields that the API sometimes emits as strings.
type flexInt int64

// UnmarshalJSON implements json.Unmarshaler.
func (f *flexInt) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		n, perr := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if perr != nil {
			return perr
		}
		*f = flexInt(n)
		return nil
	}
	var n int64
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = flexInt(n)
	return nil
}

// chanBoardsResponse is the /boards and /archives response envelope.
type chanBoardsResponse struct {
	Archives map[string]chanBoard `json:"archives"`
	Boards   map[string]chanBoard `json:"boards"`
}

type chanBoard struct {
	Shortname string `json:"shortname"`
	Name      string `json:"name"`
	IsNSFW    bool   `json:"is_nsfw"`
}

// ListBoards implements adapter.BoardLister. The /archives endpoint lists the
// boards that mirror 4chan (the /boards endpoint only has the site's own
// internal boards).
func (a *Adapter) ListBoards(ctx context.Context) ([]model.BoardInfo, error) {
	raw, err := a.client.GetBytes(ctx, "/_/api/chan/archives/")
	if err != nil {
		return nil, fmt.Errorf("desuarchive: list boards: %w", err)
	}
	var resp chanBoardsResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("desuarchive: parse boards: %w", err)
	}
	// Map iteration order is nondeterministic; sort by shortname for stability.
	codes := make([]string, 0, len(resp.Archives))
	for code := range resp.Archives {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	out := make([]model.BoardInfo, 0, len(codes))
	for _, code := range codes {
		b := resp.Archives[code]
		out = append(out, model.BoardInfo{Code: b.Shortname, Title: b.Name, NSFW: b.IsNSFW})
	}
	return out, nil
}

// FetchCatalog implements Adapter (first index page).
func (a *Adapter) FetchCatalog(ctx context.Context, board string) ([]model.ThreadSummary, error) {
	return a.FetchCatalogPage(ctx, board, 1)
}

// FetchCatalogPage implements adapter.PaginatedCataloger: one page of the
// board's thread history, newest first.
func (a *Adapter) FetchCatalogPage(ctx context.Context, board string, page int) ([]model.ThreadSummary, error) {
	raw, err := a.client.GetBytes(ctx,
		fmt.Sprintf("/_/api/chan/index/?board=%s&page=%d", board, page))
	if err != nil {
		return nil, fmt.Errorf("desuarchive: index %s page %d: %w", board, page, err)
	}
	var entries map[string]chanIndexEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("desuarchive: parse index %s page %d: %w", board, page, err)
	}
	out := make([]model.ThreadSummary, 0, len(entries))
	// Map iteration order is nondeterministic; sort by thread number for
	// stable ordering.
	nums := make([]string, 0, len(entries))
	for n := range entries {
		nums = append(nums, n)
	}
	sort.Strings(nums)
	for _, n := range nums {
		op := entries[n].Op
		out = append(out, model.ThreadSummary{
			ThreadID:   op.Num,
			Subject:    firstNonEmpty(op.TitleProc, op.Title),
			ReplyCount: op.NReplies,
			FileCount:  op.NImages,
			Sticky:     op.Sticky == 1,
			Locked:     op.Locked == 1,
			Archived:   true, // every listed thread is an archived thread
			LastBump:   op.Timestamp,
		})
	}
	return out, nil
}

// FetchThread implements Adapter.
func (a *Adapter) FetchThread(ctx context.Context, board, threadID string) (*model.Thread, error) {
	raw, err := a.client.GetBytes(ctx,
		fmt.Sprintf("/_/api/chan/thread/?board=%s&num=%s", board, threadID))
	if err != nil {
		return nil, fmt.Errorf("desuarchive: thread %s/%s: %w", board, threadID, err)
	}
	var rawResp map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rawResp); err != nil {
		return nil, fmt.Errorf("desuarchive: parse thread %s/%s: %w", board, threadID, err)
	}
	// The API reports "Thread not found." as a 200 with an error string, which
	// the scheduler must treat like a 404 so the thread is archived.
	if errMsg, ok := rawResp["error"]; ok {
		var s string
		if json.Unmarshal(errMsg, &s) == nil {
			return nil, httpclient.ErrNotFound
		}
		return nil, fmt.Errorf("desuarchive: thread %s/%s: %s", board, threadID, string(errMsg))
	}
	body, ok := rawResp[threadID]
	if !ok {
		return nil, httpclient.ErrNotFound
	}
	var ct chanThread
	if err := json.Unmarshal(body, &ct); err != nil {
		return nil, fmt.Errorf("desuarchive: parse thread %s/%s: %w", board, threadID, err)
	}

	th := &model.Thread{Board: board, ThreadID: threadID}
	if p, err := a.postFromChan(ct.Op); err == nil {
		th.Subject = p.Subject
		th.Sticky = ct.Op.Sticky == 1
		th.Locked = ct.Op.Locked == 1
		th.Archived = true
		th.LastBump = p.Timestamp
		th.Posts = append(th.Posts, p)
	} else {
		a.logger.Warn("desuarchive: skip OP", "thread", threadID, "err", err)
	}
	for _, rp := range ct.Posts {
		if rp.Subnum != "" && rp.Subnum != "0" {
			continue // ghost post
		}
		if rp.Num == threadID {
			continue // op already added
		}
		p, err := a.postFromChan(rp)
		if err != nil {
			a.logger.Warn("desuarchive: skip reply", "thread", threadID, "err", err)
			continue
		}
		th.Posts = append(th.Posts, p)
	}
	if len(th.Posts) == 0 {
		return nil, fmt.Errorf("desuarchive: thread %s/%s: no posts", board, threadID)
	}
	return th, nil
}

// ParsePost implements Adapter (single post JSON object).
func (a *Adapter) ParsePost(raw []byte) (*model.Post, error) {
	var p chanPost
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	return a.postFromChan(p)
}

func (a *Adapter) postFromChan(cp chanPost) (*model.Post, error) {
	if cp.Num == "" {
		return nil, fmt.Errorf("desuarchive: post without number")
	}
	p := &model.Post{
		NativeID:  cp.Num,
		ThreadID:  cp.ThreadNum,
		Timestamp: cp.Timestamp,
		Name:      firstNonEmpty(cp.NameProc, cp.Name),
		Tripcode:  firstNonEmpty(cp.TripProc, cp.Trip),
		Capcode:   cp.Capcode,
		PosterID:  firstNonEmpty(cp.PosterHashProc, cp.PosterHash),
		Country:   cp.Country,
		Subject:   firstNonEmpty(cp.TitleProc, cp.Title),
	}
	if cp.Num != cp.ThreadNum && cp.ThreadNum != "" {
		p.ParentID = cp.ThreadNum
	}
	p.Sage = parser.IsSage(cp.Email) || parser.IsSage(cp.Name)
	if cp.Comment != "" {
		p.CommentRaw = cp.Comment
	}
	if cp.CommentProc != "" {
		if cleaned, err := parser.Sanitize(cp.CommentProc); err == nil {
			p.CommentHTML = cleaned
		} else {
			p.CommentHTML = cp.CommentProc
		}
	}
	p.Quotes = parser.ExtractQuotesWith(cp.Comment, nil, nil)
	if cp.Media != nil {
		if f := fileFromMedia(cp.Media); f != nil {
			p.Files = []*model.File{f}
		}
	}
	return p, nil
}

func fileFromMedia(m *chanMedia) *model.File {
	// Deleted/banned/unavailable media has no fetchable URL (FoolFuuka renders
	// a placeholder). Skip it: the downloader cannot fetch anything anyway.
	if m == nil || m.MediaLink == "" || m.MediaStatus == "" {
		return nil
	}
	switch m.MediaStatus {
	case "normal", "available":
	default:
		return nil
	}
	f := &model.File{
		FullURL:          m.MediaLink,
		ThumbURL:         m.ThumbLink,
		OriginalFilename: m.MediaFilename,
		ServerFilename:   firstNonEmpty(m.MediaOrig, m.Media),
		SizeBytes:        int64(m.MediaSize),
		Width:            int(m.MediaW),
		Height:           int(m.MediaH),
		Spoiler:          m.Spoiler == "1",
	}
	base := firstNonEmpty(m.MediaOrig, m.Media)
	if i := strings.LastIndex(base, "."); i >= 0 {
		f.Ext = base[i:]
	}
	if m.MediaHash != "" {
		if raw, err := base64.StdEncoding.DecodeString(m.MediaHash); err == nil {
			f.MD5 = hex.EncodeToString(raw)
		}
	}
	return f
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
