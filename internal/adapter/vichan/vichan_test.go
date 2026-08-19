package vichan

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

const vichanThreadPage = `<html><body>
<div class="post op" data-no="34224" data-time="1754903820">
  <span class="subject">Experiencias con chicas pickme</span>
  <span class="name">Waiyado</span>
  <div class="com">&iquest;Qu&eacute; es una pick-me?<br>Es una chica que busca aprobaci&oacute;n masculina.</div>
  <div class="file">
    <div class="fileinfo">File: <a href="/b/src/1777759026847.jpg">79.6 KB, 736x978</a></div>
    <a href="/b/src/1777759026847.jpg"><img class="thumb" src="/b/thumb/1777759026847.webp"></a>
  </div>
</div>
<div class="post reply" data-no="34713" data-time="1754932980">
  <span class="name">An&oacute;nimo</span>
  <div class="com">wai, si era como la de la pick...<br>&gt;&gt;34973<br>&gt;greentext here</div>
</div>
</body></html>`

const vichanBoardPage = `<html><body>
<div class="thread">
  <div class="post op" data-no="34224" data-time="1754903820">
    <span class="subject">Experiencias con chicas pickme</span>
    <div class="com">Omitted? no.</div>
  </div>
  <div class="threadstats">58 Replies / 33 Images</div>
</div>
<div class="thread">
  <div class="post op" data-no="35222" data-time="1754904000">
    <div class="com">second thread</div>
  </div>
  <span class="replies">12 replies and 3 images omitted.</span>
</div>
</body></html>`

func newVichan(t *testing.T, page string) (*Adapter, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/b/":
			w.Write([]byte(vichanBoardPage))
		case "/b/res/34224.html":
			w.Write([]byte(page))
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

// Modern wired-7 theme: no data-no/data-time, id="op_<n>", comment in .body,
// file rendered as fileinfo + a src/ anchor wrapping an img.
const vichanModernThreadPage = `<html><body>
<div class="post op" id="op_3215">
  <div class="files"><div class="file"><p class="fileinfo">618.75 KB, 972x648</p>
    <a href="/a/src/1772777229515.jpg" target="_blank"><img class="post-image" src="/a/thumb/1772777229515.webp"></a>
  </div></div>
  <p class="intro"><span class="subject">Jap&oacute;n antes de plaga</span>&nbsp;<span class="name">Waiyado</span>&nbsp;
    <time datetime="2026-03-06T06:07:09Z">06/03/26 (Fri) 06:07</time>
    <a class="post_link" href="/a/res/3215.html#3215">No.</a><a class="post_no" href="/a/res/3215.html#q3215">3215</a></p>
  <div class="body">Encontr&eacute; una USB que fue m&iacute;a all&aacute; por 2009.<br>No hay nada realmente interesante.</div>
</div>
<div class="post reply" id="reply_3435">
  <p class="intro"><span class="name">An&oacute;nimo</span>&nbsp;
    <time datetime="2026-03-06T07:12:00Z">06/03/26 (Fri) 07:12</time>
    <a class="post_no" href="/a/res/3215.html#q3435">3435</a></p>
  <div class="body">&gt;&gt;3215 &lt;&lt;no te creo</div>
</div>
</body></html>`

func TestFetchThreadVichanModern(t *testing.T) {
	a, ts := newVichan(t, vichanModernThreadPage)
	defer ts.Close()

	th, err := a.FetchThread(context.Background(), "b", "34224")
	if err != nil {
		t.Fatal(err)
	}
	if len(th.Posts) != 2 {
		t.Fatalf("want 2 posts, got %d", len(th.Posts))
	}
	op := th.Posts[0]
	if op.NativeID != "3215" || op.Subject != "Japón antes de plaga" {
		t.Errorf("op = %+v", op)
	}
	if op.Name != "Waiyado" {
		t.Errorf("name = %q", op.Name)
	}
	if op.Timestamp == 0 {
		t.Error("expected timestamp from time[datetime]")
	}
	if len(op.Files) != 1 {
		t.Fatalf("want 1 file, got %d", len(op.Files))
	}
	f := op.Files[0]
	if f.FullURL != ts.URL+"/a/src/1772777229515.jpg" {
		t.Errorf("full = %s", f.FullURL)
	}
	if f.ThumbURL != ts.URL+"/a/thumb/1772777229515.webp" {
		t.Errorf("thumb = %s", f.ThumbURL)
	}
	if f.Width != 972 || f.Height != 648 {
		t.Errorf("dims = %dx%d", f.Width, f.Height)
	}
	if f.SizeBytes != int64(618.75*1024) {
		t.Errorf("size = %d", f.SizeBytes)
	}

	reply := th.Posts[1]
	if reply.NativeID != "3435" {
		t.Errorf("reply id = %q", reply.NativeID)
	}
	if len(reply.Quotes) != 1 || reply.Quotes[0].PostID != "3215" {
		t.Errorf("quotes = %+v", reply.Quotes)
	}
}

func TestFetchThreadVichan(t *testing.T) {
	a, ts := newVichan(t, vichanThreadPage)
	defer ts.Close()

	th, err := a.FetchThread(context.Background(), "b", "34224")
	if err != nil {
		t.Fatal(err)
	}
	if len(th.Posts) != 2 {
		t.Fatalf("want 2 posts, got %d", len(th.Posts))
	}
	op := th.Posts[0]
	if op.NativeID != "34224" {
		t.Errorf("op id = %s", op.NativeID)
	}
	if op.Subject != "Experiencias con chicas pickme" {
		t.Errorf("subject = %q", op.Subject)
	}
	if op.Name != "Waiyado" {
		t.Errorf("name = %q", op.Name)
	}
	if op.Timestamp != 1754903820 {
		t.Errorf("timestamp = %d", op.Timestamp)
	}
	if len(op.Files) != 1 {
		t.Fatalf("want 1 file on OP, got %d", len(op.Files))
	}
	f := op.Files[0]
	if f.FullURL != ts.URL+"/b/src/1777759026847.jpg" {
		t.Errorf("full = %s", f.FullURL)
	}
	if f.ThumbURL != ts.URL+"/b/thumb/1777759026847.webp" {
		t.Errorf("thumb = %s", f.ThumbURL)
	}
	if f.Width != 736 || f.Height != 978 {
		t.Errorf("dims = %dx%d", f.Width, f.Height)
	}
	if f.SizeBytes != 81510 {
		t.Errorf("size = %d", f.SizeBytes)
	}

	reply := th.Posts[1]
	if reply.ParentID != "34224" {
		t.Errorf("reply parent = %q", reply.ParentID)
	}
	if len(reply.Quotes) != 1 || reply.Quotes[0].PostID != "34973" {
		t.Errorf("quotes = %+v", reply.Quotes)
	}
}

func TestFetchCatalogVichan(t *testing.T) {
	a, ts := newVichan(t, vichanThreadPage)
	defer ts.Close()

	sums, err := a.FetchCatalog(context.Background(), "b")
	if err != nil {
		t.Fatal(err)
	}
	if len(sums) != 2 {
		t.Fatalf("want 2 summaries, got %d", len(sums))
	}
	if sums[0].ThreadID != "34224" || sums[0].ReplyCount != 58 || sums[0].FileCount != 33 {
		t.Errorf("summary0 = %+v", sums[0])
	}
	if sums[0].Subject != "Experiencias con chicas pickme" {
		t.Errorf("summary0 subject = %q", sums[0].Subject)
	}
	if sums[1].ThreadID != "35222" || sums[1].ReplyCount != 12 || sums[1].FileCount != 3 {
		t.Errorf("summary1 = %+v", sums[1])
	}
}

func TestParsePostVichan(t *testing.T) {
	a, ts := newVichan(t, vichanThreadPage)
	defer ts.Close()

	p, err := a.ParsePost([]byte(`<div class="post" data-no="1" data-time="1"><span class="name">x</span><div class="com">hi</div></div>`))
	if err != nil {
		t.Fatal(err)
	}
	if p.NativeID != "1" || p.Name != "x" || p.CommentHTML == "" {
		t.Errorf("post = %+v", p)
	}
}
