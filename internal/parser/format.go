package parser

import (
	"strconv"
	"strings"
)

// ParseFileSize converts the human file sizes vichan/4chan render ("630 KB",
// "636.38 KB", "50.58KB", "252.3 KB", "3.1 MB") into bytes.
func ParseFileSize(num, unit string) int64 {
	f, err := strconv.ParseFloat(strings.ReplaceAll(num, ",", ""), 64)
	if err != nil {
		return 0
	}
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "kb":
		return int64(f * 1024)
	case "mb":
		return int64(f * 1024 * 1024)
	case "gb":
		return int64(f * 1024 * 1024 * 1024)
	}
	return int64(f)
}
