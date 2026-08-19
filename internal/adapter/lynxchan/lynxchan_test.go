package lynxchan

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

const lynxThreadJSON = `{
  "op": {
    "no": 49657, "resto": 0, "sub": "Fuck The Web (FTW) movement",
    "name": "John Rain", "time": 1785930835, "last_mod": 1785930835,
    "filename": "IMG_1195.GIF", "ext": ".gif", "w": 2048, "h": 1152,
    "fsize": 71065, "tim": "1786896565672-0", "md5": "abc", "board": "homu",
    "com": "If you are looking to help start a new image board community..."
  },
  "posts": [
    {
      "no": 49557, "resto": 49657, "name": "Acid Burn", "time": 1785929456,
      "com": "[&gt;&gt;49556] for logo use krita guess"
    }
  ]
}`

func newLynx(t *testing.T, catalog string) (*Adapter, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.RequestURI() {
		case "/homu/?json=1":
			w.Write([]byte(catalog))
		case "/homu/res/49657.json":
			w.Write([]byte(lynxThreadJSON))
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

func TestFetchThreadLynx(t *testing.T) {
	a, ts := newLynx(t, `{"threads":[]}`)
	defer ts.Close()

	th, err := a.FetchThread(context.Background(), "homu", "49657")
	if err != nil {
		t.Fatal(err)
	}
	if len(th.Posts) != 2 {
		t.Fatalf("want 2 posts, got %d", len(th.Posts))
	}
	op := th.Posts[0]
	if op.NativeID != "49657" || op.Subject != "Fuck The Web (FTW) movement" {
		t.Errorf("op = %+v", op)
	}
	if op.Name != "John Rain" {
		t.Errorf("name = %q", op.Name)
	}
	if op.Timestamp != 1785930835 {
		t.Errorf("timestamp = %d", op.Timestamp)
	}
	if len(op.Files) != 1 {
		t.Fatalf("want 1 file, got %d", len(op.Files))
	}
	f := op.Files[0]
	if f.OriginalFilename != "IMG_1195.GIF" {
		t.Errorf("original = %q", f.OriginalFilename)
	}
	if f.FullURL != ts.URL+"/homu/src/1786896565672-0.gif" {
		t.Errorf("full = %s", f.FullURL)
	}
	if f.ThumbURL != ts.URL+"/homu/thumb/1786896565672-0.png" {
		t.Errorf("thumb = %s", f.ThumbURL)
	}
	if f.MD5 != "abc" {
		t.Errorf("md5 = %q", f.MD5)
	}
	if f.Width != 2048 || f.Height != 1152 {
		t.Errorf("dims = %dx%d", f.Width, f.Height)
	}

	reply := th.Posts[1]
	if reply.ParentID != "49657" {
		t.Errorf("reply parent = %q", reply.ParentID)
	}
	if len(reply.Quotes) != 1 || reply.Quotes[0].PostID != "49556" {
		t.Errorf("quotes = %+v", reply.Quotes)
	}
}

func TestFetchCatalogLynxArray(t *testing.T) {
	catalog := `{"threads": [
		{"posts": [{"no": 49657, "resto": 0, "sub": "FTW movement", "time": 1785930835, "last_mod": 1785930835, "tim": "1786896565672-0", "ext": ".gif"}], "post_count": 120, "file_count": 60, "sticky": true}
	]}`
	a, ts := newLynx(t, catalog)
	defer ts.Close()

	sums, err := a.FetchCatalog(context.Background(), "homu")
	if err != nil {
		t.Fatal(err)
	}
	if len(sums) != 1 {
		t.Fatalf("want 1 summary, got %d", len(sums))
	}
	s := sums[0]
	if s.ThreadID != "49657" || s.Subject != "FTW movement" || !s.Sticky {
		t.Errorf("summary = %+v", s)
	}
	if s.ReplyCount != 120 || s.FileCount != 60 {
		t.Errorf("counts = %+v", s)
	}
}

func TestFetchCatalogLynxMap(t *testing.T) {
	catalog := `{"threads": {
		"49657": {"op": {"no": 49657, "resto": 0, "sub": "map shape", "time": 1785930835, "last_mod": 1785930835}, "post_count": 3, "file_count": 1}
	}}`
	a, ts := newLynx(t, catalog)
	defer ts.Close()

	sums, err := a.FetchCatalog(context.Background(), "homu")
	if err != nil {
		t.Fatal(err)
	}
	if len(sums) != 1 || sums[0].ThreadID != "49657" || sums[0].Subject != "map shape" {
		t.Errorf("sums = %+v", sums)
	}
}
