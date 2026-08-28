package store

import "time"

// Site mirrors the sites table plus health columns.
type Site struct {
	ID                  int64
	Name                string
	BaseURL             string
	Platform            string
	APIAvailable        bool
	LastSuccessfulPoll  *time.Time
	ConsecutiveFailures int
	LastError           string
	CircuitOpen         bool
	CircuitUntil        *time.Time
}

// Board mirrors the boards table.
type Board struct {
	ID          int64
	SiteID      int64
	Code        string
	Title       string
	NSFW        bool
	Worksafe    bool
	ArchivePage int
}

// Thread mirrors the threads table.
type Thread struct {
	ID           int64
	BoardID      int64
	NativeID     string
	Subject      string
	Sticky       bool
	Locked       bool
	Archived     bool
	Status       string
	LastBumpTime *time.Time
	ReplyCount   int
	FileCount    int
	MissingCount int
	LastSeenAt   *time.Time
}

// Post mirrors a post row (the embed worker's selection target).
type Post struct {
	ID                int64
	ThreadID          int64
	NativeID          string
	Timestamp         *time.Time
	AuthorName        string
	Tripcode          string
	Capcode           string
	PosterID          string
	CommentParsed     string
	Sage              bool
	Country           string
	Flag              string
	PendingEmbedding  bool
	OriginalBoard     string    // original board name where the post came from
	Website           string    // website/source it came from
	OriginalThread    string    // original thread number from the source
	OriginalLink      string    // original link to the attachment
}

// Job mirrors the jobs table.
type Job struct {
	ID          int64
	SiteID      int64
	BoardID     *int64
	ThreadID    *int64
	Kind        string
	Status      string
	Attempts    int
	MaxAttempts int
	RunAfter    time.Time
	LastError   string
	Payload     []byte
}

// PendingDownload is a file row that still needs its full/thumb content.
type PendingDownload struct {
	FileID        int64
	PostID        int64
	FullURL       string
	ThumbURL      string
	DownloadFull  bool
	DownloadThumb bool
	PlatformMD5   string
	PlatformSHA1  string
	FileHash      string
}

// Tag is a lightweight user-defined or auto-generated tag.
type Tag struct {
	ID        int64
	Name      string // unique via application logic
	CreatedAt time.Time
}

// TagAttachment links a tag to a post attachment.
type TagAttachment struct {
	TagID   int64
	PostID  int64
}

// TagThread links a tag to a thread.
type TagThread struct {
	TagID   int64
	ThreadID int64
}
