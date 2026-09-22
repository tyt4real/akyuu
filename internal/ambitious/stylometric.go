package ambitious

import (
	"context"
	"log/slog"
	"math"
	"regexp"
	"sort"
	"strings"
)

type StylometricWorker struct {
	store        StoreAPI
	logger       *slog.Logger
	minPosts     int
	nGramMin     int
	nGramMax     int
	topFeatures  int
	simThreshold float64
}

type authorProfile struct {
	PosterID  string
	ThreadID  int64
	PostIDs   []int64
	Features  map[string]float64 // n-gram -> TF-IDF
	PostCount int
}

func NewStylometricWorker(st StoreAPI, logger *slog.Logger, minPosts, nGramMin, nGramMax, topFeatures int, simThreshold float64) *StylometricWorker {
	if minPosts <= 0 {
		minPosts = 5
	}
	if nGramMin <= 0 {
		nGramMin = 3
	}
	if nGramMax <= 0 {
		nGramMax = 5
	}
	if topFeatures <= 0 {
		topFeatures = 200
	}
	if simThreshold <= 0 {
		simThreshold = 0.7
	}
	return &StylometricWorker{
		store:        st,
		logger:       logger,
		minPosts:     minPosts,
		nGramMin:     nGramMin,
		nGramMax:     nGramMax,
		topFeatures:  topFeatures,
		simThreshold: simThreshold,
	}
}

func (w *StylometricWorker) RunOnce(ctx context.Context) error {
	w.logger.Debug("stylometric: analyzing threads")

	// Find threads with enough posts for analysis
	rows, err := w.store.Query(ctx, `
		SELECT t.id, t.board_id, COUNT(p.id) as post_count
		FROM threads t
		JOIN posts p ON p.thread_id = t.id
		WHERE p.poster_id IS NOT NULL AND p.poster_id != ''
		  AND p.comment_parsed IS NOT NULL AND p.comment_parsed != ''
		GROUP BY t.id, t.board_id
		HAVING COUNT(p.id) >= $1
		ORDER BY post_count DESC
		LIMIT 50`, w.minPosts)
	if err != nil {
		return err
	}
	defer rows.Close()

	type threadInfo struct {
		ThreadID  int64
		BoardID   int64
		PostCount int
	}

	var threads []threadInfo
	for rows.Next() {
		var t threadInfo
		if err := rows.Scan(&t.ThreadID, &t.BoardID, &t.PostCount); err != nil {
			continue
		}
		threads = append(threads, t)
	}

	for _, t := range threads {
		if err := w.analyzeThread(ctx, t.ThreadID); err != nil {
			w.logger.Error("stylometric: analyze thread", "thread", t.ThreadID, "err", err)
		}
	}

	return nil
}

func (w *StylometricWorker) analyzeThread(ctx context.Context, threadID int64) error {
	// Get all posts in thread grouped by poster_id
	rows, err := w.store.Query(ctx, `
		SELECT p.id, p.poster_id, p.comment_parsed
		FROM posts p
		WHERE p.thread_id = $1
		  AND p.poster_id IS NOT NULL AND p.poster_id != ''
		  AND p.comment_parsed IS NOT NULL AND p.comment_parsed != ''
		ORDER BY p.poster_id, p.id`, threadID)
	if err != nil {
		return err
	}
	defer rows.Close()

	// Group posts by poster
	posters := make(map[string]*authorProfile)
	for rows.Next() {
		var postID int64
		var posterID, comment string
		if err := rows.Scan(&postID, &posterID, &comment); err != nil {
			continue
		}

		prof, ok := posters[posterID]
		if !ok {
			prof = &authorProfile{PosterID: posterID, ThreadID: threadID}
			posters[posterID] = prof
		}
		prof.PostIDs = append(prof.PostIDs, postID)
		prof.PostCount++
	}

	// Need at least 2 posters with enough posts each
	validPosters := make([]*authorProfile, 0)
	for _, prof := range posters {
		if prof.PostCount >= w.minPosts {
			validPosters = append(validPosters, prof)
		}
	}

	if len(validPosters) < 2 {
		return nil // Not enough posters with sufficient posts
	}

	// Build corpus: combine all posts for TF-IDF
	allPosts := make([]string, 0)
	postToPoster := make(map[int]string)

	for _, prof := range validPosters {
		for _, postID := range prof.PostIDs {
			// Fetch post content
			var text string
			w.store.QueryRow(ctx, `SELECT comment_parsed FROM posts WHERE id = $1`, postID).Scan(&text)
			if text != "" {
				postToPoster[len(allPosts)] = prof.PosterID
				allPosts = append(allPosts, text)
			}
		}
	}

	if len(allPosts) == 0 {
		return nil
	}

	// Extract n-grams from all posts
	docFreq := make(map[string]int)
	postNgrams := make([]map[string]int, len(allPosts))

	for i, post := range allPosts {
		ngrams := w.extractNGrams(post)
		postNgrams[i] = ngrams
		for ngram := range ngrams {
			docFreq[ngram]++
		}
	}

	// Compute TF-IDF for each post
	totalDocs := float64(len(allPosts))
	postTFIDF := make([]map[string]float64, len(allPosts))

	for i, ngrams := range postNgrams {
		tfidf := make(map[string]float64)
		total := 0
		for _, count := range ngrams {
			total += count
		}
		for ngram, count := range ngrams {
			tf := float64(count) / float64(total)
			idf := math.Log(totalDocs / float64(docFreq[ngram]))
			tfidf[ngram] = tf * idf
		}
		postTFIDF[i] = tfidf
	}

	// Aggregate TF-IDF per poster
	for _, prof := range validPosters {
		prof.Features = make(map[string]float64)
		postCount := 0
		for i, posterID := range postToPoster {
			if posterID == prof.PosterID {
				for ngram, score := range postTFIDF[i] {
					prof.Features[ngram] += score
				}
				postCount++
			}
		}
		// Average
		for ngram := range prof.Features {
			prof.Features[ngram] /= float64(postCount)
		}
	}

	// Keep top features per poster
	for _, prof := range validPosters {
		type kv struct {
			ngram string
			score float64
		}
		var items []kv
		for ngram, score := range prof.Features {
			items = append(items, kv{ngram, score})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].score > items[j].score })
		if len(items) > w.topFeatures {
			items = items[:w.topFeatures]
		}
		prof.Features = make(map[string]float64)
		for _, item := range items {
			prof.Features[item.ngram] = item.score
		}
	}

	// Compute pairwise similarities
	type pair struct {
		PosterA, PosterB string
		Similarity       float64
	}

	var pairs []pair
	for i := 0; i < len(validPosters); i++ {
		for j := i + 1; j < len(validPosters); j++ {
			sim := w.cosineSimilarity(validPosters[i].Features, validPosters[j].Features)
			if sim >= w.simThreshold {
				pairs = append(pairs, pair{
					PosterA:    validPosters[i].PosterID,
					PosterB:    validPosters[j].PosterID,
					Similarity: sim,
				})
			}
		}
	}

	// Store clusters (simple: each high-similarity pair forms a cluster)
	// In production, use proper clustering (HDBSCAN, etc.)
	for _, p := range pairs {
		// Check if cluster exists
		var clusterID int64
		err := w.store.QueryRow(ctx, `
			SELECT id FROM style_clusters 
			WHERE thread_id = $1 AND (poster_a = $2 AND poster_b = $3 OR poster_a = $3 AND poster_b = $2)
			LIMIT 1`, threadID, p.PosterA, p.PosterB).Scan(&clusterID)

		if err != nil {
			// Create new cluster
			err = w.store.QueryRow(ctx, `
				INSERT INTO style_clusters (thread_id, poster_a, poster_b, similarity, method, post_count_a, post_count_b)
				VALUES ($1, $2, $3, $4, 'tfidf_ngram', $5, $6)
				RETURNING id`,
				threadID, p.PosterA, p.PosterB, p.Similarity,
				posters[p.PosterA].PostCount, posters[p.PosterB].PostCount).Scan(&clusterID)
			if err != nil {
				w.logger.Error("stylometric: create cluster", "err", err)
				continue
			}

			// Link posts to cluster
			for _, postID := range posters[p.PosterA].PostIDs {
				w.store.Exec(ctx, `
					INSERT INTO style_cluster_posts (cluster_id, post_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
					clusterID, postID)
			}
			for _, postID := range posters[p.PosterB].PostIDs {
				w.store.Exec(ctx, `
					INSERT INTO style_cluster_posts (cluster_id, post_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
					clusterID, postID)
			}
		}
	}

	return nil
}

func (w *StylometricWorker) extractNGrams(text string) map[string]int {
	// Clean text
	text = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(text, " ")
	text = strings.ToLower(text)
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)

	ngrams := make(map[string]int)

	// Character n-grams
	for n := w.nGramMin; n <= w.nGramMax; n++ {
		for i := 0; i <= len(text)-n; i++ {
			ngram := text[i : i+n]
			// Skip n-grams that are mostly whitespace/punctuation
			if w.isValidNGram(ngram) {
				ngrams[ngram]++
			}
		}
	}

	// Word n-grams (1-3 words)
	words := strings.Fields(text)
	for n := 1; n <= 3; n++ {
		for i := 0; i <= len(words)-n; i++ {
			ngram := strings.Join(words[i:i+n], " ")
			if len(ngram) >= 3 {
				ngrams[ngram]++
			}
		}
	}

	return ngrams
}

func (w *StylometricWorker) isValidNGram(s string) bool {
	// Must have at least one letter/digit
	hasAlnum := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			hasAlnum = true
			break
		}
	}
	return hasAlnum
}

func (w *StylometricWorker) cosineSimilarity(a, b map[string]float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for k, va := range a {
		vb, ok := b[k]
		if ok {
			dot += va * vb
		}
		normA += va * va
	}
	for _, vb := range b {
		normB += vb * vb
	}

	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
