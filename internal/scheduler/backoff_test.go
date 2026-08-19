package scheduler

import (
	"encoding/json"
	"testing"
	"time"

	"akyuu/internal/adapter"
	"akyuu/internal/config"
	"akyuu/internal/store"
)

func TestBackoffDelay(t *testing.T) {
	// Monotonic growth for consecutive attempts.
	prev := time.Duration(0)
	for attempt := 1; attempt <= 5; attempt++ {
		d := backoffDelay(attempt, time.Minute, 30*time.Minute)
		if d < 0 {
			t.Fatalf("negative delay %v", d)
		}
		if d < prev {
			t.Errorf("attempt %d delay %v < previous %v", attempt, d, prev)
		}
		prev = d
	}
	// Attempt 1 uses the base (with jitter around it).
	base := backoffDelay(1, time.Second, time.Hour)
	if base < 750*time.Millisecond || base > 1250*time.Millisecond {
		t.Errorf("attempt 1 delay %v outside jitter range", base)
	}
	// The base delay is capped at max; jitter adds +/-25% of the capped value,
	// so the result stays within [max*0.75, max*1.25].
	big := backoffDelay(40, time.Minute, 30*time.Minute)
	if big < 30*time.Minute*3/4 || big > 30*time.Minute*5/4 {
		t.Errorf("delay %v outside capped jitter range", big)
	}
	// Bad base falls back to 1s.
	if d := backoffDelay(1, 0, time.Hour); d < 0 {
		t.Errorf("zero base produced %v", d)
	}
}

func TestCircuitCooldown(t *testing.T) {
	// base * 2^(failures-1), e capped at 5, bounded at 24h.
	cases := []struct {
		poll time.Duration
		fail int
		want time.Duration
	}{
		{time.Minute, 1, time.Minute},
		{time.Minute, 5, 16 * time.Minute},
		{time.Minute, 6, 32 * time.Minute},
		{5 * time.Second, 5, 16 * time.Minute}, // poll clamped to 1m base
		{time.Hour, 40, 24 * time.Hour},        // 2^5*1h exceeds the 24h cap
	}
	for _, c := range cases {
		if got := circuitCooldown(c.poll, c.fail); got != c.want {
			t.Errorf("circuitCooldown(%v,%d) = %v, want %v", c.poll, c.fail, got, c.want)
		}
	}
}

func TestCountThreadStats(t *testing.T) {
	th := &adapter.Thread{Posts: []*adapter.Post{
		{NativeID: "1", ThreadID: "1", Files: []*adapter.File{{}}},
		{NativeID: "2", ThreadID: "1", ParentID: "1", Files: []*adapter.File{{}, {}}},
	}}
	replies, files := countThreadStats(th)
	if replies != 1 || files != 3 {
		t.Errorf("stats = (%d,%d), want (1,3)", replies, files)
	}
	replies, files = countThreadStats(&adapter.Thread{})
	if replies != 0 || files != 0 {
		t.Errorf("empty stats = (%d,%d)", replies, files)
	}
}

func TestBoardCfg(t *testing.T) {
	sc := &config.SiteConfig{Boards: []config.BoardConfig{{Code: "a", NSFW: true}, {Code: "b"}}}
	if got := boardCfg(sc, "a"); !got.NSFW {
		t.Error("board a should be nsfw")
	}
	if got := boardCfg(sc, "missing"); got.Code != "" {
		t.Errorf("missing board = %+v", got)
	}
}

func TestJobPayloadHelpers(t *testing.T) {
	noPayload := &store.Job{}
	if f, th := jobPayloadFlags(noPayload); f || th {
		t.Errorf("nil payload flags = (%v,%v)", f, th)
	}
	with := &store.Job{Payload: mustJSON(t, map[string]any{"download_full": true, "download_thumb": false})}
	f, th := jobPayloadFlags(with)
	if !f || th {
		t.Errorf("payload flags = (%v,%v), want (true,false)", f, th)
	}
	if v, ok := jobPayloadInt(with, "limit"); ok || v != 0 {
		t.Errorf("jobPayloadInt missing key = (%d,%v), want (0,false)", v, ok)
	}
	withLimit := &store.Job{Payload: mustJSON(t, map[string]any{"limit": 25})}
	if v, ok := jobPayloadInt(withLimit, "limit"); !ok || v != 25 {
		t.Errorf("jobPayloadInt limit = (%d,%v)", v, ok)
	}
	bad := &store.Job{Payload: []byte("not json")}
	if f, th := jobPayloadFlags(bad); f || th {
		t.Errorf("bad payload flags = (%v,%v)", f, th)
	}
	if v, ok := jobPayloadInt(bad, "x"); ok || v != 0 {
		t.Errorf("bad payload int = (%d,%v)", v, ok)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
