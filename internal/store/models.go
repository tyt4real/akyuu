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
	ID               int64
	ThreadID         int64
	NativeID         string
	Timestamp        *time.Time
	CommentParsed    string
	PendingEmbedding bool
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
