package adapter

import (
	"log/slog"
	"testing"
)

func TestNewDispatchesByPlatform(t *testing.T) {
	cases := []struct {
		platform Platform
		name     string
	}{
		{PlatformLynxChan, "lynxchan"},
		{PlatformVichan, "vichan"},
		{PlatformFourChan, "fourchan"},
	}
	for _, c := range cases {
		a, err := New(c.platform, "https://example.org", "", nil, slog.New(slog.DiscardHandler))
		if err != nil {
			t.Fatalf("New(%q): %v", c.platform, err)
		}
		if got := a.PlatformName(); got != c.name {
			t.Errorf("PlatformName() = %q, want %q", got, c.name)
		}
		if a == nil {
			t.Fatalf("New(%q) returned nil adapter", c.platform)
		}
	}
}

func TestNewUnknownPlatform(t *testing.T) {
	a, err := New(Platform("dokuwiki"), "https://example.org", "", nil, nil)
	if err == nil {
		t.Fatal("expected error for unknown platform")
	}
	if a != nil {
		t.Fatal("expected nil adapter on error")
	}
}

func TestPlatformNames(t *testing.T) {
	if PlatformLynxChan != "lynxchan" || PlatformVichan != "vichan" || PlatformFourChan != "fourchan" {
		t.Error("platform constants drifted from config keys")
	}
}
