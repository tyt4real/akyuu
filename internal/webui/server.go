package webui

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"akyuu/internal/store"
	"log/slog"
)

type Server struct {
	store  *store.Store
	logger *slog.Logger
	tmpl   *template.Template
}

func NewServer(st *store.Store, logger *slog.Logger) (*Server, error) {
	// Parse templates
	tmpl := template.New("").Funcs(template.FuncMap{
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
		"truncate": func(s string, n int) string {
			if len(s) <= n {
				return s
			}
			return s[:n] + "..."
		},
		"sub": func(a, b int) int { return a - b },
		"add": func(a, b int) int { return a + b },
		"now": func() time.Time { return time.Now() },
		"humanSize": func(bytes int64) string {
			const unit = 1024
			if bytes < unit {
				return fmt.Sprintf("%d B", bytes)
			}
			div, exp := int64(unit), 0
			for n := bytes / unit; n >= unit; n /= unit {
				div *= unit
				exp++
			}
			return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
		},
		"formatTime": func(t interface{}) string {
			switch v := t.(type) {
			case time.Time:
				return v.Format("01/02/06(Mon)15:04")
			case *time.Time:
				if v != nil {
					return v.Format("01/02/06(Mon)15:04")
				}
			case int64:
				if v > 0 {
					return time.Unix(v, 0).Format("01/02/06(Mon)15:04")
				}
			}
			return ""
		},
		"dict": func(values ...interface{}) map[string]interface{} {
			m := make(map[string]interface{})
			for i := 0; i < len(values); i += 2 {
				if i+1 < len(values) {
					key, ok := values[i].(string)
					if ok {
						m[key] = values[i+1]
					}
				}
			}
			return m
		},
	})
	var err error
	tmpl, err = tmpl.ParseGlob("internal/webui/templates/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	return &Server{
		logger: nil,
		tmpl:   tmpl,
	}, nil
}

func (s *Server) SetStore(st *store.Store) {
	s.store = st
}

func (s *Server) SetLogger(logger *slog.Logger) {
	s.logger = logger
}

func (s *Server) HandleIndex(c *gin.Context) {
	ctx := c.Request.Context()

	sites, err := s.store.ListSites(ctx)
	if err != nil {
		s.logger.Error("failed to list sites", "err", err)
		c.HTML(http.StatusInternalServerError, "error.tmpl", gin.H{"error": "Failed to load sites"})
		return
	}

	type SiteWithBoards struct {
		Site   *store.Site
		Boards []*store.Board
	}
	var sitesWithBoards []SiteWithBoards
	for _, site := range sites {
		boards, err := s.store.ListBoards(ctx, site.ID)
		if err != nil {
			s.logger.Error("failed to list boards", "site", site.Name, "err", err)
			continue
		}
		sitesWithBoards = append(sitesWithBoards, SiteWithBoards{
			Site:   site,
			Boards: boards,
		})
	}

	c.HTML(http.StatusOK, "index.tmpl", gin.H{
		"Title":       "Akyuu - Image Board Archive",
		"Sites":       sitesWithBoards,
		"CurrentYear": 2024,
	})
}

type SiteWithBoards struct {
	Site   *store.Site
	Boards []*store.Board
}

func (s *Server) HandleSite(c *gin.Context) {
	ctx := c.Request.Context()
	siteName := c.Param("site")

	site, err := s.store.GetSite(ctx, siteName)
	if err != nil {
		s.logger.Error("failed to get site", "site", siteName, "err", err)
		c.HTML(http.StatusNotFound, "error.tmpl", gin.H{"error": "Site not found"})
		return
	}

	boards, err := s.store.ListBoards(ctx, site.ID)
	if err != nil {
		s.logger.Error("failed to list boards", "site", siteName, "err", err)
		c.HTML(http.StatusInternalServerError, "error.tmpl", gin.H{"error": "Failed to load boards"})
		return
	}

	c.HTML(http.StatusOK, "site.tmpl", gin.H{
		"Title": site.Name,
		"Site":  site,
		"Boards": boards,
	})
}

func (s *Server) HandleBoard(c *gin.Context) {
	ctx := c.Request.Context()
	siteName := c.Param("site")
	boardCode := c.Param("board")

	board, err := s.store.GetBoard(ctx, siteName, boardCode)
	if err != nil {
		s.logger.Error("failed to get board", "site", siteName, "board", boardCode, "err", err)
		c.HTML(http.StatusNotFound, "error.tmpl", gin.H{"error": "Board not found"})
		return
	}

	// Get site for context
	site, _ := s.store.GetSite(ctx, siteName)

	// Get threads for this board
	threads, err := s.store.ListActiveThreads(ctx, board.ID)
	if err != nil {
		s.logger.Error("failed to list threads", "board", boardCode, "err", err)
		c.HTML(http.StatusInternalServerError, "error.tmpl", gin.H{"error": "Failed to load threads"})
		return
	}

	// Get posts for each thread (first 3 replies)
	type ThreadWithReplies struct {
		*store.Thread
		Replies []*store.Post
	}

	var threadsWithReplies []ThreadWithReplies
	for _, thread := range threads {
		posts, err := s.store.GetThreadPosts(ctx, thread.ID)
		if err != nil {
			s.logger.Error("failed to get thread posts", "thread", thread.ID, "err", err)
			continue
		}
		// Limit to first 3 replies for preview
		replies := posts
		if len(posts) > 3 {
			replies = posts[:3]
		}
		threadsWithReplies = append(threadsWithReplies, ThreadWithReplies{
			Thread:  thread,
			Replies: replies,
		})
	}

	c.HTML(http.StatusOK, "board.tmpl", gin.H{
		"Title":  "/" + board.Code + "/ - " + board.Title,
		"Board":  board,
		"Site":   site,
		"Threads": threadsWithReplies,
		"Page":   1,
	})
}

type ThreadWithReplies struct {
	*store.Thread
	Replies []*store.Post
}

func (s *Server) HandleThread(c *gin.Context) {
	ctx := c.Request.Context()
	siteName := c.Param("site")
	boardCode := c.Param("board")
	threadIDStr := c.Param("thread")

	threadID, err := strconv.ParseInt(threadIDStr, 10, 64)
	if err != nil {
		c.HTML(http.StatusBadRequest, "error.tmpl", gin.H{"error": "Invalid thread ID"})
		return
	}

	board, err := s.store.GetBoard(ctx, siteName, boardCode)
	if err != nil {
		s.logger.Error("failed to get board", "site", siteName, "err", err)
		c.HTML(http.StatusNotFound, "error.tmpl", gin.H{"error": "Board not found"})
		return
	}

	// Get site for context
	_, _ = s.store.GetSite(ctx, siteName)

	_, err = s.store.GetThreadByID(ctx, threadID)
	if err != nil {
		s.logger.Error("failed to get thread", "err", err)
		c.HTML(http.StatusNotFound, "error.tmpl", gin.H{"error": "Thread not found"})
		return
	}

	// Get posts for this thread
	posts, err := s.store.GetThreadPosts(ctx, threadID)
	if err != nil {
		s.logger.Error("failed to get thread posts", "thread", threadID, "err", err)
		c.HTML(http.StatusInternalServerError, "error.tmpl", gin.H{"error": "Failed to load posts"})
		return
	}

	// First post is OP, rest are replies
	var op *store.Post
	var replies []*store.Post
	if len(posts) > 0 {
		op = posts[0]
		replies = posts[1:]
	}

	c.HTML(http.StatusOK, "thread.tmpl", gin.H{
		"Title":   "Thread",
		"Thread":  op,
		"Replies": replies,
		"Board":   board,
		"Site":    map[string]string{"Name": siteName},
		"CSRFToken": "todo", // TODO: implement CSRF
	})
}

func (s *Server) HandleSearch(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.HTML(http.StatusOK, "search.tmpl", gin.H{
			"Title":  "Search",
			"Query":  "",
			"Results": []interface{}{},
		})
		return
	}

	// TODO: Implement search via API
	c.HTML(http.StatusOK, "search.tmpl", gin.H{
		"Title":  "Search Results",
		"Query":  query,
		"Results": []interface{}{},
	})
}

func (s *Server) HandleOverboard(c *gin.Context) {
	ctx := c.Request.Context()

	// Get recent posts across all boards
	posts, err := s.store.ListPosts(ctx, "", "", "", nil)
	if err != nil {
		s.logger.Error("failed to list posts", "err", err)
		c.HTML(http.StatusInternalServerError, "error.tmpl", gin.H{"error": "Failed to load posts"})
		return
	}

	// Limit to recent 50 posts
	if len(posts) > 50 {
		posts = posts[:50]
	}

	c.HTML(http.StatusOK, "overboard.tmpl", gin.H{
		"Title": "Overboard",
		"Posts": posts,
	})
}