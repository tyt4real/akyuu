package fourchan

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Current 4chan HTML: post number lives in id="p<num>", epoch in
// data-utc on .dateTime, and the thumb is an anchor wrapping an <img> with
// data-md5. Derived from the live boards.4chan.org markup.
const fourchanThreadPage = `<html><body>
<div class="thread" id="t105076684">
  <div class="postContainer">
    <div id="p105076684" class="post op">
      <span class="subject">cel ai</span>
      <span class="name">Anonymous</span>
      <span class="dateTime postNum" data-utc="1755400133"><time datetime="2025-04-25T16:24:10-04:00">04/25/25(Fri)16:24:10</time> <a href="#p105076684">No.</a><a href="javascript:quote('105076684');">105076684</a></span>
      <div class="file">
        <div class="fileText">File: <a href="//i.4cdn.org/g/1745612650141704.png" target="_blank">sticky btfo.png</a> (294 KB, 535x420)</div>
        <a class="fileThumb" href="//i.4cdn.org/g/1745612650141704.png" target="_blank"><img src="//i.4cdn.org/g/1745612650141704s.jpg" data-md5="zuZHMJMYYp5WY7vM397nWQ=="></a>
      </div>
      <div class="postMessage">Just a little more shine</div>
    </div>
  </div>
</div>
<div class="postContainer">
  <div id="p105076685" class="post reply">
    <span class="name">Anonymous</span>
    <span class="postertrip">!!h+inaLvwW9m</span>
    <span class="dateTime" data-utc="1755406705"><time datetime="2025-04-25T16:24:26-04:00">04/25/25(Fri)16:24:26</time></span>
    <div class="file">
      <div class="fileText">File: <a href="//i.4cdn.org/g/1745612666142055.jpg" target="_blank">IMG_1845.jpg</a> (68 KB, 796x1024)</div>
      <a class="fileThumb" href="//i.4cdn.org/g/1745612666142055.jpg" target="_blank"><img src="//i.4cdn.org/g/1745612666142055s.jpg" data-md5="AAAAAAAAAAAAAAAAAAAAAA=="></a>
    </div>
    <div class="postMessage">&gt;&gt;105076684</div>
  </div>
</div>
</body></html>`

// Older 4chan markup (and still served by some mirrors): data-no / data-time,
// img.fileThumb with no wrapper anchor.
const fourchanLegacyThreadPage = `<html><body>
<div class="postContainer">
  <div class="post op" id="p952683023" data-no="952683023" data-time="1755400133">
    <span class="name">Anonymous</span>
    <div class="file">
      <div class="fileText">File: <a href="//i.4cdn.org/b/1786892933109351.jpg">1772300396251763.jpg</a> (630 KB, 1792x2400)</div>
      <img class="fileThumb" src="//i.4cdn.org/b/1786892933109351s.jpg">
    </div>
    <div class="postMessage">old theme</div>
  </div>
</div>
</body></html>`

const fourchanBoardPage = `<html><body>
<div class="thread" id="t105076684">
  <div class="postContainer">
    <div id="p105076684" class="post op">
      <span class="subject">cel ai</span>
      <span class="dateTime" data-utc="1755400133"><time datetime="2025-04-25T16:24:10-04:00">04/25/25(Fri)16:24:10</time></span>
      <div class="postMessage">Just a little more shine</div>
    </div>
  </div>
  <div class="postContainer">
    <div id="p105076685" class="post reply">
      <span class="dateTime" data-utc="1755406705"></span>
      <div class="postMessage">&gt;&gt;105076684</div>
    </div>
  </div>
</div>
</body></html>`

func newFourchan(t *testing.T, threadPage string) (*Adapter, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/g/":
			w.Write([]byte(fourchanBoardPage))
		case "/g/thread/105076684":
			w.Write([]byte(threadPage))
		default:
			http.NotFound(w, r)
		}
	}))
	a, err := New(ts.URL, "test", http.DefaultTransport, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return a, ts
}

func TestFetchThreadFourchan(t *testing.T) {
	a, ts := newFourchan(t, fourchanThreadPage)
	defer ts.Close()

	th, err := a.FetchThread(context.Background(), "g", "105076684")
	if err != nil {
		t.Fatal(err)
	}
	if len(th.Posts) != 2 {
		t.Fatalf("want 2 posts, got %d", len(th.Posts))
	}
	op := th.Posts[0]
	if op.NativeID != "105076684" || op.Subject != "cel ai" {
		t.Errorf("op = %+v", op)
	}
	if op.Timestamp != 1755400133 {
		t.Errorf("op timestamp = %d", op.Timestamp)
	}
	if len(op.Files) != 1 {
		t.Fatalf("want 1 file, got %d", len(op.Files))
	}
	f := op.Files[0]
	if f.FullURL != "https://i.4cdn.org/g/1745612650141704.png" {
		t.Errorf("full = %s", f.FullURL)
	}
	if f.ThumbURL != "https://i.4cdn.org/g/1745612650141704s.jpg" {
		t.Errorf("thumb = %s", f.ThumbURL)
	}
	if f.MD5 != "cee647309318629e5663bbccdfdee759" {
		t.Errorf("md5 = %q", f.MD5)
	}
	if f.Width != 535 || f.Height != 420 {
		t.Errorf("dims = %dx%d", f.Width, f.Height)
	}
	if f.SizeBytes != 294*1024 {
		t.Errorf("size = %d", f.SizeBytes)
	}
	if f.OriginalFilename != "sticky btfo.png" {
		t.Errorf("original filename = %q", f.OriginalFilename)
	}

	reply := th.Posts[1]
	if reply.Tripcode != "!!h+inaLvwW9m" {
		t.Errorf("reply tripcode = %q", reply.Tripcode)
	}
	if len(reply.Quotes) != 1 || reply.Quotes[0].PostID != "105076684" {
		t.Errorf("quotes = %+v", reply.Quotes)
	}
}

func TestFetchThreadFourchanLegacy(t *testing.T) {
	a, ts := newFourchan(t, fourchanLegacyThreadPage)
	defer ts.Close()

	th, err := a.FetchThread(context.Background(), "g", "105076684")
	if err != nil {
		t.Fatal(err)
	}
	if len(th.Posts) != 1 {
		t.Fatalf("want 1 post, got %d", len(th.Posts))
	}
	op := th.Posts[0]
	if op.NativeID != "952683023" || op.Timestamp != 1755400133 {
		t.Errorf("legacy post = %+v", op)
	}
	f := op.Files[0]
	if f.FullURL != "https://i.4cdn.org/b/1786892933109351.jpg" || f.ThumbURL != "https://i.4cdn.org/b/1786892933109351s.jpg" {
		t.Errorf("legacy files = %+v", f)
	}
}

func TestFetchCatalogFourchan(t *testing.T) {
	a, ts := newFourchan(t, fourchanThreadPage)
	defer ts.Close()

	sums, err := a.FetchCatalog(context.Background(), "g")
	if err != nil {
		t.Fatal(err)
	}
	if len(sums) != 1 {
		t.Fatalf("want 1 summary, got %d", len(sums))
	}
	if sums[0].ThreadID != "105076684" || sums[0].Subject != "cel ai" {
		t.Errorf("summary = %+v", sums[0])
	}
}
