// Package model holds the normalized data types shared by every platform
// adapter and the store. It is a leaf package (no dependencies) so that
// platform implementations can import it without creating an import cycle.
package model

// ThreadSummary is the per-thread card data available from a board (catalog)
// listing page. Listing pages are never ground truth for reply/file counts;
// FetchThread is.
type ThreadSummary struct {
	ThreadID   string
	Subject    string
	ReplyCount int
	FileCount  int
	Sticky     bool
	Locked     bool
	Archived   bool
	LastBump   int64 // unix seconds; 0 if the platform does not expose it
}

// Thread is a fully fetched thread: the OP plus all replies.
type Thread struct {
	Board    string
	ThreadID string
	Subject  string
	Sticky   bool
	Locked   bool
	Archived bool
	LastBump int64 // unix seconds
	Posts    []*Post
}

// Post is a single normalized post. Fields a given platform does not provide
// are left zero-valued. Per-platform availability is documented in each
// adapter package (e.g. poster_id exists on 4chan but not on vichan).
type Post struct {
	NativeID    string // platform post number, as text
	ThreadID    string // the thread this post belongs to (the OP's number)
	ParentID    string // resto; "0" / empty for an OP post
	Name        string
	Tripcode    string
	Capcode     string
	PosterID    string
	Country     string
	Flag        string
	Timestamp   int64 // unix seconds; 0 if the platform exposes no timestamp
	Sage        bool
	CommentRaw  string // verbatim comment markup as served by the platform
	CommentHTML string // sanitized, display-safe HTML
	Quotes      []QuoteRef
	Files       []*File
	Subject     string
	OriginalBoard   string // original board name where the post came from
	Website         string // website/source it came from
	OriginalThread  string // original thread number from the source
	OriginalLink    string // original link to the attachment
}

// QuoteRef is a >>12345 / >>>/board/123 reference found in a comment.
// Board is empty for same-board references. A reference to a post we have
// never seen is stored verbatim and resolved only at query time.
type QuoteRef struct {
	Board  string
	PostID string
}

// File is a single attachment on a post. FileHash is the content-addressable
// key into blobs; it may be empty until the file has been downloaded or a
// platform-provided hash has been mapped to a blob.
type File struct {
	OriginalFilename string
	ServerFilename   string // platform-side stored name, e.g. <tim><ext>
	Ext              string // with leading dot, e.g. ".jpg"
	SizeBytes        int64
	Width            int // 0 for non-image mime types
	Height           int
	FullURL          string
	ThumbURL         string
	MD5              string // platform-provided content hash, if any
	SHA1             string // platform-provided content hash, if any
	Spoiler          bool
}

// BoardInfo is a board as reported by a platform's board listing. It is used
// for auto-discovery of archive boards (sites with auto_boards: true).
type BoardInfo struct {
	Code  string
	Title string
	NSFW  bool
}
