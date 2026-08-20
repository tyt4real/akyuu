// Package apiserver exposes the archived corpus over HTTP: a health endpoint
// and a semantic-search endpoint that embeds a free-text query in-process and
// runs a pgvector similarity search.
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
}

// New builds a Server. logger may be nil.
func New(e embedder.Embedder, s Searcher, h HealthChecker, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{embed: e, search: s, health: h, logger: logger, now: time.Now}
}

// Routes returns the API mux.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /search", s.handleSearch)
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Response already committed; nothing left to do but log.
		slog.Error("api: encode response", "err", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
