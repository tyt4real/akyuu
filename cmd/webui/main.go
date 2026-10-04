// Command webui runs the web UI server for browsing image board content.
package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"akyuu/internal/config"
	"akyuu/internal/store"
	"akyuu/internal/webui"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

func main() {
	configPath := flag.String("config", "config/config.yaml", "path to the global config file")
	addr := flag.String("listen", ":8082", "HTTP listen address")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	logger := newLogger(cfg.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st, err := store.New(ctx, cfg.Database.DSN, logger)
	if err != nil {
		logger.Error("connect database", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	if err := st.Migrate(ctx); err != nil {
		logger.Error("migrate", "err", err)
		os.Exit(1)
	}

	// Create the web UI server
	webServer, err := webui.NewServer(st, logger)
	if err != nil {
		logger.Error("create web server", "err", err)
		os.Exit(1)
	}

	// Set up Gin router with rate limiting
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gin.Logger())

	// Rate limiting middleware - 100 requests per minute per IP
	router.Use(rateLimitMiddleware(time.Minute, 100))

	// Static files
	router.Static("/static", "./internal/webui/static")

	// Load HTML templates
	router.LoadHTMLGlob("internal/webui/templates/*")

	// Public routes
	public := router.Group("/")
	public.GET("/", webServer.HandleIndex)
	public.GET("/site/:site", webServer.HandleSite)
	public.GET("/site/:site/board/:board", webServer.HandleBoard)
	public.GET("/site/:site/board/:board/thread/:thread", webServer.HandleThread)
	public.GET("/search", webServer.HandleSearch)
	public.GET("/overboard", webServer.HandleOverboard)
	public.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// API routes with stricter rate limiting
	api := router.Group("/api")
	api.Use(rateLimitMiddleware(10*time.Minute, 30)) // 30 requests per 10 minutes
	api.GET("/search", func(c *gin.Context) {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
	})

	// Create HTTP server
	server := &http.Server{
		Addr:    *addr,
		Handler: router,
	}

	// Start server
	go func() {
		logger.Info("starting web UI server", "addr", *addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	logger.Info("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown", "err", err)
		os.Exit(1)
	}
	logger.Info("server stopped")
}

func newLogger(level string) *slog.Logger {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: l}))
}

// rateLimitMiddleware creates a rate limiting middleware using golang.org/x/time/rate
func rateLimitMiddleware(limitPeriod time.Duration, limitCount int) gin.HandlerFunc {
	if limitPeriod == 0 {
		limitPeriod = time.Minute
	}
	if limitCount == 0 {
		limitCount = 100
	}

	limiter := rate.NewLimiter(rate.Every(limitPeriod/time.Duration(limitCount)), limitCount)

	return func(c *gin.Context) {
		if !limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":    "rate limit exceeded",
				"retry_in": int64(time.Minute.Seconds()),
			})
			return
		}
		c.Next()
	}
}