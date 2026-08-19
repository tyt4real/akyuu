package downloader

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
)

// SafetyChecker decides whether a file's content hash is on a blocklist.
// A file that matches must never be written to storage.
type SafetyChecker interface {
	// Check returns true when the given sha256 hex hash is flagged. The
	// downloader discards the content and does not persist it on a match.
	Check(ctx context.Context, sha256Hex string) (bool, error)
}

// BlocklistChecker implements SafetyChecker from two optional sources: a local
// list of known-bad hashes and a remote PhotoDNA-style API. At least one
// source is expected; with neither configured the checker always returns
// "not flagged" and logs a warning at startup.
type BlocklistChecker struct {
	mu       sync.RWMutex
	hashes   map[string]struct{}
	listPath string
	apiURL   string
	apiToken string
	client   *http.Client
	logger   *slog.Logger
}

// NewBlocklistChecker builds a checker. cfg carries hash_list_path and the
// optional api_url / api_token. If neither is set, the checker logs a warning:
// the safety gate still runs on every file, it just matches nothing until a
// source is configured.
func NewBlocklistChecker(listPath, apiURL, apiToken string, logger *slog.Logger) (*BlocklistChecker, error) {
	if logger == nil {
		logger = slog.Default()
	}
	c := &BlocklistChecker{
		hashes:   map[string]struct{}{},
		listPath: listPath,
		apiURL:   apiURL,
		apiToken: apiToken,
		client:   &http.Client{Timeout: httpTimeout},
		logger:   logger,
	}
	if listPath != "" {
		if err := c.loadList(); err != nil {
			return nil, err
		}
		logger.Info("csam: loaded hash list", "path", listPath, "count", len(c.hashes))
	}
	if apiURL != "" && apiToken == "" {
		logger.Warn("csam: api_url set without api_token; remote checks will be skipped")
	}
	if listPath == "" && apiURL == "" {
		logger.Warn("csam: no hash source configured (hash_list_path or api_url); " +
			"the safety check still runs but matches nothing")
	}
	return c, nil
}

func (c *BlocklistChecker) loadList() error {
	f, err := os.Open(c.listPath)
	if err != nil {
		return fmt.Errorf("downloader: open csam hash list %s: %w", c.listPath, err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 256*1024)
	for sc.Scan() {
		h := strings.ToLower(strings.TrimSpace(sc.Text()))
		if h == "" || strings.HasPrefix(h, "#") {
			continue
		}
		c.hashes[h] = struct{}{}
	}
	return sc.Err()
}

// Check implements SafetyChecker. The local list is authoritative; the API is
// only consulted when the local list has no match and is configured.
func (c *BlocklistChecker) Check(ctx context.Context, sha256Hex string) (bool, error) {
	h := strings.ToLower(strings.TrimSpace(sha256Hex))
	if h == "" {
		return false, nil
	}
	c.mu.RLock()
	_, hit := c.hashes[h]
	c.mu.RUnlock()
	if hit {
		return true, nil
	}
	if c.apiURL != "" && c.apiToken != "" {
		return c.checkAPI(ctx, h)
	}
	return false, nil
}

// checkAPI POSTs the hash to a PhotoDNA-style endpoint. The wire contract here
// is deliberately minimal: {"hash": "..."} -> {"match": true|false}. Real
// PhotoDNA integrations would swap this method's request shape for the
// provider's SDK.
func (c *BlocklistChecker) checkAPI(ctx context.Context, hash string) (bool, error) {
	body, _ := json.Marshal(map[string]string{"hash": hash})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL, strings.NewReader(string(body)))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	resp, err := c.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("downloader: csam api: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("downloader: csam api returned %s", resp.Status)
	}
	var verdict struct {
		Match bool `json:"match"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&verdict); err != nil {
		return false, fmt.Errorf("downloader: csam api decode: %w", err)
	}
	return verdict.Match, nil
}

// AlwaysAllow is a SafetyChecker for tests that never flags content.
type AlwaysAllow struct{}

// Check implements SafetyChecker.
func (AlwaysAllow) Check(context.Context, string) (bool, error) { return false, nil }
