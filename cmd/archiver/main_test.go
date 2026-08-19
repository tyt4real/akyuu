package main

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"akyuu/internal/config"
	"akyuu/internal/store"
)

func newTestStore(t *testing.T, dsn string) *store.Store {
	t.Helper()
	ctx := context.Background()
	st, err := store.New(ctx, dsn, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := st.ResetAll(ctx); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestNewLoggerLevels(t *testing.T) {
	cases := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"garbage", slog.LevelInfo}, // unknown falls back to info
		{"", slog.LevelInfo},
	}
	ctx := context.Background()
	for _, c := range cases {
		lg := newLogger(c.in)
		h := lg.Handler()
		if !h.Enabled(ctx, c.want) {
			t.Errorf("newLogger(%q): %v not enabled", c.in, c.want)
		}
		// Anything quieter than the configured level must be disabled.
		for l := slog.LevelDebug; l < c.want; l++ {
			if h.Enabled(ctx, l) {
				t.Errorf("newLogger(%q): level %v enabled below %v", c.in, l, c.want)
			}
		}
	}
}

func TestSyncSites(t *testing.T) {
	dsn := os.Getenv("AKYUU_TEST_DSN")
	if dsn == "" {
		t.Skip("AKYUU_TEST_DSN not set")
	}
	ctx := context.Background()
	st := newTestStore(t, dsn)
	defer st.Close()
	logger := slog.New(slog.DiscardHandler)

	// Two sites; one board leaves worksafe unset (defaults true), one sets it
	// explicitly, one is nsfw.
	sites := []*config.SiteConfig{
		{
			Name: "s1", BaseURL: "https://s1", Platform: "lynxchan", APIAvailable: false,
			Boards: []config.BoardConfig{
				{Code: "a", Title: "Anon"},
				{Code: "b", Title: "B", Worksafe: false},            // neither -> worksafe defaults true
				{Code: "c", Title: "C", NSFW: true},                 // nsfw, worksafe stays false
				{Code: "d", Title: "D", NSFW: true, Worksafe: true}, // nsfw but worksafe allowed
			},
		},
		{Name: "s2", BaseURL: "https://s2", Platform: "vichan", APIAvailable: true, Boards: nil},
	}
	syncSites(ctx, st, sites, logger)

	if site, _ := st.GetSite(ctx, "s1"); site == nil || site.BaseURL != "https://s1" {
		t.Fatalf("site s1 = %+v", site)
	}
	if site, _ := st.GetSite(ctx, "s2"); site == nil || !site.APIAvailable || site.Platform != "vichan" {
		t.Fatalf("site s2 = %+v", site)
	}
	site1, _ := st.GetSite(ctx, "s1")
	boards, err := st.ListBoards(ctx, site1.ID)
	if err != nil {
		t.Fatal(err)
	}
	byCode := map[string]*store.Board{}
	for _, b := range boards {
		byCode[b.Code] = b
	}
	if len(byCode) != 4 {
		t.Fatalf("boards = %+v", byCode)
	}
	if byCode["a"].Worksafe != true || byCode["a"].NSFW != false {
		t.Errorf("board a worksafe=%v nsfw=%v", byCode["a"].Worksafe, byCode["a"].NSFW)
	}
	if byCode["b"].Worksafe != true {
		t.Errorf("board b worksafe=%v, want defaulted true", byCode["b"].Worksafe)
	}
	if byCode["c"].Worksafe != false || byCode["c"].NSFW != true {
		t.Errorf("board c worksafe=%v nsfw=%v", byCode["c"].Worksafe, byCode["c"].NSFW)
	}
	if byCode["d"].Worksafe != true || byCode["d"].NSFW != true {
		t.Errorf("board d worksafe=%v nsfw=%v", byCode["d"].Worksafe, byCode["d"].NSFW)
	}

	// Boards removed from config are left in place, never deleted.
	syncSites(ctx, st, []*config.SiteConfig{
		{Name: "s1", BaseURL: "https://s1", Platform: "lynxchan", Boards: []config.BoardConfig{{Code: "a", Title: "Anon"}}},
	}, logger)
	boards, _ = st.ListBoards(ctx, site1.ID)
	if len(boards) != 4 {
		t.Errorf("boards after shrinking config = %d, want 4 (no deletions)", len(boards))
	}
}
