package downloader

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"akyuu/internal/adapter"
	"akyuu/internal/store"
)

// testStore opens a store against AKYUU_TEST_DSN, skipping the test when unset.
func testStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := os.Getenv("AKYUU_TEST_DSN")
	if dsn == "" {
		t.Skip("set AKYUU_TEST_DSN to run downloader integration tests")
	}
	st, err := store.New(context.Background(), dsn, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := st.ResetAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	return st
}

// seedThread creates a site/board/thread with one post carrying the given
// files and returns the thread id plus the file rows pending download.
func seedThread(t *testing.T, st *store.Store, files []*adapter.File) (threadID int64, pendings []*store.PendingDownload) {
	t.Helper()
	ctx := context.Background()
	siteID, err := st.UpsertSite(ctx, "dl-site", "https://dl.example", "vichan", true)
	if err != nil {
		t.Fatal(err)
	}
	boardID, err := st.UpsertBoard(ctx, siteID, "b", "Board", false, true)
	if err != nil {
		t.Fatal(err)
	}
	threadID, _, err = st.UpsertThread(ctx, boardID, "100", "t", false, false, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	posts := []*adapter.Post{{NativeID: "100", ThreadID: "100", CommentRaw: "x", Files: files}}
	if err := st.UpsertThreadPosts(ctx, threadID, posts); err != nil {
		t.Fatal(err)
	}
	pendings, err = st.ListPendingDownloads(ctx, threadID)
	if err != nil {
		t.Fatal(err)
	}
	return threadID, pendings
}

func newDownloader(t *testing.T, st *store.Store, root string, checker SafetyChecker) *Downloader {
	t.Helper()
	return New(st, nil, root, checker, slog.New(slog.DiscardHandler))
}

// pngData returns a real, decodable 1x1 PNG (image.Decode must accept it for
// thumbnail generation).
func pngData(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDownloadFullStoresBlob(t *testing.T) {
	st := testStore(t)
	root := filepath.Join(t.TempDir(), "storage")
	data := pngData(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer srv.Close()

	_, pendings := seedThread(t, st, []*adapter.File{{
		OriginalFilename: "img.png", ServerFilename: "img.png", Ext: ".png", SizeBytes: int64(len(data)),
		FullURL: srv.URL + "/src/img.png", ThumbURL: srv.URL + "/thumb/img.png",
	}})
	if len(pendings) != 1 {
		t.Fatalf("pending = %d", len(pendings))
	}

	d := newDownloader(t, st, root, AlwaysAllow{})
	if err := d.DownloadFull(context.Background(), pendings[0]); err != nil {
		t.Fatal(err)
	}

	// Blob exists with the computed hash and mime; file is flagged downloaded.
	n, err := st.BlobCount(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("blob count = %d err=%v", n, err)
	}
	hash := hashOf(t, data)
	blob, err := st.GetBlobByHash(context.Background(), hash)
	if err != nil {
		t.Fatal(err)
	}
	if blob.MimeType != "image/png" || blob.SizeBytes != int64(len(data)) {
		t.Errorf("blob = %+v", blob)
	}
	if blob.StoragePath == "" {
		t.Error("blob has no storage path")
	}
	// The file is on disk at the content-addressed path.
	if _, err := os.Stat(filepath.Join(root, blob.StoragePath)); err != nil {
		t.Errorf("file not on disk: %v", err)
	}
}

// TestDownloadFullDedup verifies identical content across two posts yields one
// blob with ref_count 2.
func TestDownloadFullDedup(t *testing.T) {
	st := testStore(t)
	root := filepath.Join(t.TempDir(), "storage")
	data := pngData(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer srv.Close()

	ctx := context.Background()
	siteID, _ := st.UpsertSite(ctx, "dl-site", "https://dl.example", "vichan", true)
	boardID, _ := st.UpsertBoard(ctx, siteID, "b", "B", false, true)
	threadID, _, _ := st.UpsertThread(ctx, boardID, "100", "t", false, false, false, 0)
	mkFile := func() *adapter.File {
		return &adapter.File{OriginalFilename: "img.png", Ext: ".png", SizeBytes: int64(len(data)),
			FullURL: srv.URL + "/src/a.png", ThumbURL: ""}
	}
	st.UpsertThreadPosts(ctx, threadID, []*adapter.Post{
		{NativeID: "100", ThreadID: "100", Files: []*adapter.File{mkFile()}},
		{NativeID: "101", ThreadID: "100", ParentID: "100", Files: []*adapter.File{mkFile()}},
	})
	pendings, err := st.ListPendingDownloads(ctx, threadID)
	if err != nil || len(pendings) != 2 {
		t.Fatalf("pending=%d err=%v", len(pendings), err)
	}

	d := newDownloader(t, st, root, AlwaysAllow{})
	for _, pd := range pendings {
		if err := d.DownloadFull(ctx, pd); err != nil {
			t.Fatal(err)
		}
	}

	n, _ := st.BlobCount(ctx)
	if n != 1 {
		t.Fatalf("blob count = %d, want 1 (dedup)", n)
	}
	hash := hashOf(t, data)
	blob, err := st.GetBlobByHash(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	if blob.RefCount != 2 {
		t.Errorf("ref_count = %d, want 2", blob.RefCount)
	}
}

// TestDownloadFullBlocked verifies a CSAM hash match discards content.
func TestDownloadFullBlocked(t *testing.T) {
	st := testStore(t)
	root := filepath.Join(t.TempDir(), "storage")
	data := pngData(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer srv.Close()

	_, pendings := seedThread(t, st, []*adapter.File{{
		OriginalFilename: "img.png", Ext: ".png", SizeBytes: int64(len(data)),
		FullURL: srv.URL + "/src/img.png", ThumbURL: srv.URL + "/thumb/img.png",
	}})
	pd := pendings[0]

	// A checker that flags the exact hash of data.
	hash := hashOf(t, data)
	list := filepath.Join(t.TempDir(), "hashes.txt")
	if err := os.WriteFile(list, []byte(hash+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	checker, err := NewBlocklistChecker(list, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}

	d := newDownloader(t, st, root, checker)
	err = d.DownloadFull(context.Background(), pd)
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("err = %v, want ErrBlocked", err)
	}
	if n, _ := st.BlobCount(context.Background()); n != 0 {
		t.Errorf("blob count = %d, want 0", n)
	}
	if got := fileCountOnDisk(t, root); got != 0 {
		t.Errorf("files on disk = %d, want 0", got)
	}
}

func TestDownloadThumbRemote(t *testing.T) {
	st := testStore(t)
	root := filepath.Join(t.TempDir(), "storage")
	data := pngData(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer srv.Close()

	_, pendings := seedThread(t, st, []*adapter.File{{
		OriginalFilename: "img.png", Ext: ".png", SizeBytes: int64(len(data)),
		FullURL: srv.URL + "/src/img.png", ThumbURL: srv.URL + "/thumb/img.png",
	}})
	pd := pendings[0]

	d := newDownloader(t, st, root, AlwaysAllow{})
	if err := d.DownloadThumb(context.Background(), pd); err != nil {
		t.Fatal(err)
	}
	thumbHash := hashOf(t, data)
	if _, err := os.Stat(filepath.Join(root, "thumb", thumbHash[0:2], thumbHash[2:4], thumbHash+".jpg")); err != nil {
		t.Errorf("thumb not on disk: %v", err)
	}
	// Second call is idempotent and stores nothing new.
	if err := d.DownloadThumb(context.Background(), pd); err != nil {
		t.Fatal(err)
	}
}

// TestDownloadThumb404GeneratesFromFull verifies thumb fallback to local
// generation when the remote thumb 404s and the full file is stored. The
// fallback needs the file row to already reference the stored blob, which is
// the case on a re-poll after the full content has been downloaded.
func TestDownloadThumb404GeneratesFromFull(t *testing.T) {
	st := testStore(t)
	root := filepath.Join(t.TempDir(), "storage")
	data := pngData(t)
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path == "/src/img.png" {
			w.Write(data)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	threadID, pendings := seedThread(t, st, []*adapter.File{{
		OriginalFilename: "img.png", Ext: ".png", SizeBytes: int64(len(data)),
		FullURL: srv.URL + "/src/img.png", ThumbURL: srv.URL + "/thumb/img.png",
	}})
	pd := pendings[0]

	d := newDownloader(t, st, root, AlwaysAllow{})
	if err := d.DownloadFull(context.Background(), pd); err != nil {
		t.Fatal(err)
	}
	// Re-fetch pending so the file row carries the stored blob hash.
	pendings, err := st.ListPendingDownloads(context.Background(), threadID)
	if err != nil || len(pendings) != 1 {
		t.Fatalf("pending=%d err=%v", len(pendings), err)
	}
	if pendings[0].FileHash == "" {
		t.Fatal("file row does not reference the stored blob")
	}
	if err := d.DownloadThumb(context.Background(), pendings[0]); err != nil {
		t.Fatal(err)
	}
	if hits != 2 { // one /src request, one /thumb request that 404s
		t.Errorf("requests = %d, want 2", hits)
	}
	left, err := st.ListPendingDownloads(context.Background(), threadID)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Errorf("still pending after local thumb generation: %+v", left)
	}
}

func TestDownloadEmptyURLs(t *testing.T) {
	st := testStore(t)
	root := filepath.Join(t.TempDir(), "storage")
	d := newDownloader(t, st, root, AlwaysAllow{})
	if err := d.DownloadFull(context.Background(), &store.PendingDownload{}); err != nil {
		t.Errorf("DownloadFull empty: %v", err)
	}
	if err := d.DownloadThumb(context.Background(), &store.PendingDownload{}); err != nil {
		t.Errorf("DownloadThumb empty: %v", err)
	}
}

func fileCountOnDisk(t *testing.T, root string) int {
	t.Helper()
	n := 0
	err := filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			n++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}
