package desuarchive

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"akyuu/internal/adapter/httpclient"
)

// newTestAdapter spins up an httptest server serving the given handler and
// builds an Adapter rooted at it.
func newTestAdapter(t *testing.T, handler http.HandlerFunc) *Adapter {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	a, err := New(srv.URL, "akyuu-test/0.1", nil, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// minimal OP object in FoolFuuka chan-post shape.
func chanOp(num string) map[string]any {
	return map[string]any{
		"num": num, "thread_num": num, "subnum": "0", "op": "1",
		"timestamp": 1787078099, "capcode": "N", "name": "Anonymous",
		"name_processed": "Anonymous", "title": nil, "comment": "OP text",
		"comment_processed": "OP text", "sticky": 0, "locked": 0, "deleted": 0,
		"nreplies": 3, "nimages": 1,
	}
}

const (
	md5B64    = "pR5sPKiWputPDMqi+h/Gzg=="
	md5Hex    = "a51e6c3ca896a6eb4f0ccaa2fa1fc6ce"
	mediaLink = "https://desu-usergeneratedcontent.xyz/a/image/1787/07/1787078099747.png"
	thumbLink = "https://desu-usergeneratedcontent.xyz/a/thumb/1787/07/1787078099747s.jpg"
)

func withMedia(p map[string]any) map[string]any {
	p["media"] = map[string]any{
		"media_id": "87388960", "spoiler": "0",
		"media": "1787078099747.png", "media_orig": "1787078099747.png",
		"media_filename": "image(1).png",
		"media_w":        "1098", "media_h": "536", "media_size": "390699",
		"media_hash": md5B64, "media_status": "normal",
		"media_link": mediaLink, "thumb_link": thumbLink,
	}
	return p
}

func TestListBoards(t *testing.T) {
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_/api/chan/archives/" {
			t.Errorf("path = %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"site": map[string]any{"name": "Desuarchive"},
			"archives": map[string]any{
				"1": map[string]any{"shortname": "a", "name": "Anime & Manga", "is_nsfw": false},
				"2": map[string]any{"shortname": "pol", "name": "Politically Incorrect", "is_nsfw": true},
			},
			"Articles": map[string]any{},
		})
	})
	boards, err := a.ListBoards(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(boards) != 2 {
		t.Fatalf("boards = %+v", boards)
	}
	if boards[0].Code != "a" || boards[0].Title != "Anime & Manga" || boards[0].NSFW {
		t.Errorf("boards[0] = %+v", boards[0])
	}
	if boards[1].Code != "pol" || !boards[1].NSFW {
		t.Errorf("boards[1] = %+v", boards[1])
	}
}

func TestFetchCatalogPage(t *testing.T) {
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/_/api/chan/index/"; got != want {
			t.Errorf("path = %s, want %s", got, want)
		}
		if got, want := r.URL.Query().Get("board"), "a"; got != want {
			t.Errorf("board = %q", got)
		}
		if got, want := r.URL.Query().Get("page"), "4"; got != want {
			t.Errorf("page = %q", got)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"290240851": map[string]any{
				"omitted": 0, "images_omitted": 0,
				"op": withMedia(chanOp("290240851")),
			},
			"290240714": map[string]any{
				"omitted": 2, "images_omitted": 1,
				"op": chanOp("290240714"),
			},
		})
	})
	summaries, err := a.FetchCatalogPage(context.Background(), "a", 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 2 {
		t.Fatalf("summaries = %+v", summaries)
	}
	s := summaries[0]
	if s.ThreadID != "290240714" || !s.Archived || s.ReplyCount != 3 || s.FileCount != 1 ||
		s.LastBump != 1787078099 {
		t.Errorf("summary = %+v", s)
	}
	if summaries[1].ThreadID != "290240851" {
		t.Errorf("ordering = %+v", summaries)
	}
}

func TestFetchCatalogDelegatesToPageOne(t *testing.T) {
	var gotPage string
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		gotPage = r.URL.Query().Get("page")
		json.NewEncoder(w).Encode(map[string]any{
			"1": map[string]any{"op": chanOp("1")},
		})
	})
	summaries, err := a.FetchCatalog(context.Background(), "a")
	if err != nil {
		t.Fatal(err)
	}
	if gotPage != "1" || len(summaries) != 1 {
		t.Errorf("page = %q summaries = %+v", gotPage, summaries)
	}
}

func TestFetchThread(t *testing.T) {
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"290240714": map[string]any{
				"op": withMedia(chanOp("290240714")),
				"posts": map[string]any{
					"290240744": map[string]any{
						"num": "290240744", "thread_num": "290240714", "subnum": "0",
						"timestamp": 1787078199, "name": "Anonymous", "name_processed": "Anonymous",
						"comment":           ">>290240714\nNadia is the worse part of her own show.",
						"comment_processed": "Nadia is the worse part of her own show.",
						"poster_hash":       "fUSBgQ2y", "poster_hash_processed": "fUSBgQ2y",
						"poster_country": "US", "email": "sage", "capcode": "N",
						"sticky": 0, "locked": 0, "deleted": 0,
						"media": map[string]any{
							"media": "1787078199999.png", "media_orig": "1787078199999.png",
							"media_filename": "x.png", "media_link": mediaLink,
							"thumb_link": thumbLink, "media_hash": md5B64,
							"media_size": "100", "media_w": "10", "media_h": "20",
							"media_status": "normal", "spoiler": "1",
						},
					},
					"290240777": map[string]any{
						"num": "290240777", "thread_num": "290240714", "subnum": "1",
						"timestamp": 0, "name": "ghost", "comment": "ghost post",
					},
				},
			},
		})
	})
	th, err := a.FetchThread(context.Background(), "a", "290240714")
	if err != nil {
		t.Fatal(err)
	}
	if th.ThreadID != "290240714" || !th.Archived || len(th.Posts) != 2 {
		t.Fatalf("thread = %+v", th)
	}
	op := th.Posts[0]
	if op.NativeID != "290240714" || op.ThreadID != "290240714" || op.ParentID != "" {
		t.Errorf("op = %+v", op)
	}
	if len(op.Files) != 1 {
		t.Fatalf("op files = %+v", op.Files)
	}
	f := op.Files[0]
	if f.FullURL != mediaLink || f.ThumbURL != thumbLink || f.MD5 != md5Hex ||
		f.Ext != ".png" || f.OriginalFilename != "image(1).png" ||
		f.Width != 1098 || f.Height != 536 || f.SizeBytes != 390699 {
		t.Errorf("file = %+v", f)
	}
	reply := th.Posts[1]
	if reply.NativeID != "290240744" || reply.ParentID != "290240714" ||
		!reply.Sage || reply.Country != "US" || reply.PosterID != "fUSBgQ2y" {
		t.Errorf("reply = %+v", reply)
	}
	if len(reply.Quotes) != 1 || reply.Quotes[0].PostID != "290240714" {
		t.Errorf("reply quotes = %+v", reply.Quotes)
	}
	if reply.Files[0].Spoiler != true {
		t.Errorf("reply file spoiler not set")
	}
}

func TestFetchThreadNotFound(t *testing.T) {
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"error":"Thread not found."}`))
	})
	_, err := a.FetchThread(context.Background(), "a", "999999999")
	if !errors.Is(err, httpclient.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestFetchThreadMissingFromResponse(t *testing.T) {
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"1234":{"op":{"num":"1234"}}}`))
	})
	_, err := a.FetchThread(context.Background(), "a", "999")
	if !errors.Is(err, httpclient.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestParsePost(t *testing.T) {
	a := &Adapter{}
	raw, _ := json.Marshal(withMedia(chanOp("777")))
	p, err := a.ParsePost(raw)
	if err != nil {
		t.Fatal(err)
	}
	if p.NativeID != "777" || p.ThreadID != "777" || p.CommentHTML != "OP text" {
		t.Errorf("post = %+v", p)
	}
}

func TestMediaStatusFiltersUnavailableFiles(t *testing.T) {
	a := &Adapter{}
	for _, status := range []string{"", "banned", "not-available"} {
		op := chanOp("1")
		op["media"] = map[string]any{
			"media": "1.png", "media_link": mediaLink, "thumb_link": thumbLink,
			"media_hash": md5B64, "media_status": status,
		}
		raw, _ := json.Marshal(op)
		p, err := a.ParsePost(raw)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Files) != 0 {
			t.Errorf("status %q: files = %+v, want none", status, p.Files)
		}
	}
}

func TestFetchCatalogPageHTTPError(t *testing.T) {
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	if _, err := a.FetchCatalogPage(context.Background(), "a", 1); err == nil {
		t.Fatal("expected error")
	}
}
