package scheduler

import (
	"testing"

	"akyuu/internal/config"
)

func TestDownloadPolicy(t *testing.T) {
	on := true
	board := &config.BoardConfig{}
	site := &config.SiteConfig{DownloadFull: &on, DownloadThumb: &on}

	cases := []struct {
		name     string
		textOnly bool
		wantFull bool
		wantThum bool
	}{
		{"normal mode honors overrides", false, true, true},
		{"text_only forces everything off", true, false, false},
	}
	for _, c := range cases {
		s := &Scheduler{cfg: &config.Config{}}
		s.cfg.Scheduler.TextOnly = c.textOnly
		full, thumb := s.downloadPolicy(site, board)
		if full != c.wantFull || thumb != c.wantThum {
			t.Errorf("%s: downloadPolicy = (%v,%v), want (%v,%v)",
				c.name, full, thumb, c.wantFull, c.wantThum)
		}
	}
}
