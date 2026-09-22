package apiserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"akyuu/internal/embedder"
	"akyuu/internal/store"
)

// Searcher is the store slice the search handler needs.
type Searcher interface {
	SearchEmbeddings(ctx context.Context, query []float32, modelVersion string, opts store.SearchOpts) ([]store.SearchResult, error)
}

// HealthChecker reports database liveness.
type HealthChecker interface {
	Ping(ctx context.Context) error
}

// Server is the HTTP handler set for the search API.
type Server struct {
	embed  embedder.Embedder
	search Searcher
	health HealthChecker
	logger *slog.Logger
	now    func() time.Time
	store  *store.Store
}

// New builds a Server. logger may be nil.
func New(e embedder.Embedder, s Searcher, h HealthChecker, st *store.Store, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{embed: e, search: s, health: h, store: st, logger: logger, now: time.Now}
}

// Routes returns the API mux.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /search", s.handleSearch)
	mux.HandleFunc("GET /posts", s.handlePosts)
	mux.HandleFunc("GET /boards", s.handleBoards)
	mux.HandleFunc("GET /threads", s.handleThreads)
	return mux
}

type healthResponse struct {
	Status string `json:"status"`
}

// handleHealth reports database liveness.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.health.Ping(ctx); err != nil {
		s.logger.Error("health: db ping failed", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, healthResponse{Status: "degraded"})
		return
	}
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

// SearchRequest is the accepted body of POST /search.
type SearchRequest struct {
	Query         string     `json:"query"`
	Site          string     `json:"site,omitempty"`
	Board         string     `json:"board,omitempty"`
	ThreadID      *int64     `json:"thread_id,omitempty"`
	DateFrom      *time.Time `json:"date_from,omitempty"`
	DateTo        *time.Time `json:"date_to,omitempty"`
	HasAttachment *bool      `json:"has_attachment,omitempty"`
	Limit         int        `json:"limit,omitempty"`
}

// SearchResult mirrors store.SearchResult plus model metadata.
type SearchResult struct {
	PostID         int64      `json:"post_id"`
	Score          float32    `json:"score"`
	Site           string     `json:"site"`
	Board          string     `json:"board"`
	ThreadID       int64      `json:"thread_id"`
	ThreadNativeID string     `json:"thread_native_id"`
	Timestamp      *time.Time `json:"timestamp,omitempty"`
	Excerpt        string     `json:"excerpt"`
}

// SearchResponse wraps the hits and the model that produced them.
type SearchResponse struct {
	Query   string         `json:"query"`
	Model   string         `json:"model"`
	Count   int            `json:"count"`
	Results []SearchResult `json:"results"`
}

// PostResponse is the response for a single post via GET /posts.
type PostResponse struct {
	ID             int64      `json:"id"`
	ThreadID       int64      `json:"thread_id"`
	NativeID       string     `json:"native_id"`
	Timestamp      *time.Time `json:"timestamp,omitempty"`
	AuthorName     string     `json:"author_name,omitempty"`
	Tripcode       string     `json:"tripcode,omitempty"`
	Capcode        string     `json:"capcode,omitempty"`
	PosterID       string     `json:"poster_id,omitempty"`
	Country        string     `json:"country,omitempty"`
	Flag           string     `json:"flag,omitempty"`
	Sage           bool       `json:"sage"`
	CommentRaw     string     `json:"comment_raw,omitempty"`
	CommentHTML    string     `json:"comment_html,omitempty"`
	OriginalBoard  string     `json:"original_board,omitempty"`
	Website        string     `json:"website,omitempty"`
	OriginalThread string     `json:"original_thread,omitempty"`
	OriginalLink   string     `json:"original_link,omitempty"`
	Subject        string     `json:"subject,omitempty"`
}

// PostsResponse wraps the hit list for GET /posts.
type PostsResponse struct {
	Count int             `json:"count"`
	Posts []*PostResponse `json:"posts"`
}

// handleSearch embeds the query text and runs the similarity search.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(req.Query) == 0 {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}
	text, err := embedder.CleanText(req.Query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to clean query")
		return
	}
	if text == "" {
		writeError(w, http.StatusBadRequest, "query has no searchable text")
		return
	}

	vecs, err := s.embed.EmbedBatch(r.Context(), []string{text})
	if err != nil {
		s.logger.Error("search: embed query", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to embed query")
		return
	}

	res, err := s.search.SearchEmbeddings(r.Context(), vecs[0], s.embed.ModelVersion(), store.SearchOpts{
		Site:          req.Site,
		Board:         req.Board,
		ThreadID:      req.ThreadID,
		DateFrom:      req.DateFrom,
		DateTo:        req.DateTo,
		HasAttachment: req.HasAttachment,
		Limit:         req.Limit,
	})
	if err != nil {
		s.logger.Error("search: query store", "err", err)
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}

	out := SearchResponse{
		Query:   req.Query,
		Model:   s.embed.ModelVersion(),
		Count:   len(res),
		Results: make([]SearchResult, 0, len(res)),
	}
	for _, hit := range res {
		out.Results = append(out.Results, SearchResult{
			PostID:         hit.PostID,
			Score:          hit.Score,
			Site:           hit.Site,
			Board:          hit.Board,
			ThreadID:       hit.ThreadID,
			ThreadNativeID: hit.ThreadNativeID,
			Timestamp:      hit.Timestamp,
			Excerpt:        hit.Excerpt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handlePosts lists posts with optional filtering by site, board, thread native ID, and attachment status.
func (s *Server) handlePosts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	query := r.URL.Query()
	site := query.Get("site")
	board := query.Get("board")
	nativeID := query.Get("native_id")
	hasAttach := query.Get("has_attachment")

	hasAttachment := false
	if hasAttach == "1" {
		hasAttachment = true
	}

	posts, err := s.store.ListPosts(ctx, site, board, nativeID, &hasAttachment)
	if err != nil {
		s.logger.Error("api: list posts", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list posts")
		return
	}

	var postsResp []*PostResponse
	for _, p := range posts {
		postsResp = append(postsResp, &PostResponse{
			ID:             p.ID,
			ThreadID:       p.ThreadID,
			NativeID:       p.NativeID,
			Timestamp:      p.Timestamp,
			AuthorName:     p.AuthorName,
			Tripcode:       p.Tripcode,
			Capcode:        p.Capcode,
			PosterID:       p.PosterID,
			Country:        p.Country,
			Flag:           p.Flag,
			Sage:           p.PendingEmbedding,
			CommentRaw:     p.CommentParsed,
			CommentHTML:    "",
			OriginalBoard:  p.OriginalBoard,
			Website:        p.Website,
			OriginalThread: p.OriginalThread,
			OriginalLink:   p.OriginalLink,
			Subject:        "",
		})
	}

	writeJSON(w, http.StatusOK, PostsResponse{
		Count: len(postsResp),
		Posts: postsResp,
	})
}

// handleBoards lists boards for a site.
func (s *Server) handleBoards(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	sites, err := s.store.ListSites(ctx)
	if err != nil {
		s.logger.Error("api: list boards", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list boards")
		return
	}

	type boardResponse struct {
		ID          int64  `json:"id"`
		SiteID      int64  `json:"site_id"`
		Code        string `json:"code"`
		Title       string `json:"title,omitempty"`
		NSFW        bool   `json:"nsfw,omitempty"`
		Worksafe    bool   `json:"worksafe,omitempty"`
		ArchivePage int    `json:"archive_page,omitempty"`
	}

	var resp []boardResponse
	for _, st := range sites {
		resp = append(resp, boardResponse{
			ID:          st.ID,
			SiteID:      st.ID,
			Code:        st.Name,
			Title:       st.BaseURL,
			NSFW:        false,
			Worksafe:    true,
			ArchivePage: 0,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleThreads lists threads for a board.
func (s *Server) handleThreads(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	threads, err := s.store.ListActiveThreads(ctx, 0)
	if err != nil {
		s.logger.Error("api: list threads", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list threads")
		return
	}

	type threadResponse struct {
		ID         int64      `json:"id"`
		BoardID    int64      `json:"board_id"`
		NativeID   string     `json:"native_id"`
		Subject    string     `json:"subject,omitempty"`
		Sticky     bool       `json:"sticky,omitempty"`
		Locked     bool       `json:"locked,omitempty"`
		Archived   bool       `json:"archived,omitempty"`
		Status     string     `json:"status,omitempty"`
		ReplyCount int        `json:"reply_count,omitempty"`
		FileCount  int        `json:"file_count,omitempty"`
		LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	}

	var resp []threadResponse
	for _, t := range threads {
		resp = append(resp, threadResponse{
			ID:         t.ID,
			BoardID:    t.BoardID,
			NativeID:   t.NativeID,
			Subject:    t.Subject,
			Sticky:     t.Sticky,
			Locked:     t.Locked,
			Archived:   t.Archived,
			Status:     t.Status,
			ReplyCount: t.ReplyCount,
			FileCount:  t.FileCount,
			LastSeenAt: t.LastSeenAt,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("api: encode response", "err", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
