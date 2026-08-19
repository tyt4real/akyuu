package parser

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// weekdayNames maps weekday abbreviations (English and the Spanish used by
// vichan forks like 75chan/wired-7) to time.Weekday. Used to disambiguate
// MM/DD vs DD/MM by validating against the actual day of week.
var weekdayNames = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday,
	"wed": time.Wednesday, "thu": time.Thursday, "fri": time.Friday,
	"sat": time.Saturday,
	"dom": time.Sunday, "lun": time.Monday, "mar": time.Tuesday,
	"mié": time.Wednesday, "mie": time.Wednesday, "jue": time.Thursday,
	"vie": time.Friday, "sáb": time.Saturday, "sab": time.Saturday,
}

// dateWithDayRe matches vichan-style "MM/DD/YY (Day) HH:MM[:SS]" timestamps.
// The weekday is decisive for telling MM/DD from DD/MM. The day name allows
// accented characters (Spanish boards: Sáb, Mié).
var dateWithDayRe = regexp.MustCompile(`(?i)^\s*(\d{1,2})/(\d{1,2})/(\d{2})\s*\(([A-Za-zÁÉÍÓÚÑÜáéíóúñü]{2,10})\)\s*(\d{1,2}):(\d{2})(?::(\d{2}))?`)

// dateLongRe matches "YYYY/MM/DD (Day) HH:MM:SS" (75chan).
var dateLongRe = regexp.MustCompile(`(?i)^\s*(\d{4})/(\d{1,2})/(\d{1,2})\s*\(([A-Za-zÁÉÍÓÚÑÜáéíóúñü]{2,10})\)?\s*(\d{1,2}):(\d{2})(?::(\d{2}))?`)

// dateDashRe matches "DD-MM-YY HH:MM:SS" (leftypol).
var dateDashRe = regexp.MustCompile(`^\s*(\d{1,2})-(\d{1,2})-(\d{2})\s+(\d{1,2}):(\d{2}):(\d{2})`)

// ParseDisplayDate converts the human-readable timestamps that vichan-family
// boards render into a unix epoch. It returns 0 when the string is not
// recognized. When a weekday is present it is used to decide MM/DD vs DD/MM;
// otherwise MM/DD is preferred (the 4chan convention).
func ParseDisplayDate(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	if m := dateLongRe.FindStringSubmatch(s); m != nil {
		if t := mkDate(2000+atoi(m[3]), atoi(m[2]), atoi(m[1]), m[4], atoi(m[5]), atoi(m[6]), atoi(m[7])); t > 0 {
			return t
		}
		return mkDateNoWeekday(atoi(m[1]), atoi(m[2]), atoi(m[3]), atoi(m[5]), atoi(m[6]), atoi(m[7]))
	}
	if m := dateDashRe.FindStringSubmatch(s); m != nil {
		return mkDateNoWeekday(atoi(m[1]), atoi(m[2]), atoi(m[3]), atoi(m[4]), atoi(m[5]), atoi(m[6]))
	}
	if m := dateWithDayRe.FindStringSubmatch(s); m != nil {
		hh, mm, ss := atoi(m[5]), atoi(m[6]), atoi(m[7])
		// Try MM/DD first, then DD/MM; weekday decides.
		if t := mkDate(2000+atoi(m[3]), atoi(m[1]), atoi(m[2]), m[4], hh, mm, ss); t > 0 {
			return t
		}
		if t := mkDate(2000+atoi(m[3]), atoi(m[2]), atoi(m[1]), m[4], hh, mm, ss); t > 0 {
			return t
		}
		// No weekday match: prefer the 4chan MM/DD reading.
		return mkDateNoWeekday(2000+atoi(m[3]), atoi(m[1]), atoi(m[2]), hh, mm, ss)
	}
	return 0
}

// mkDate builds a timestamp and validates the weekday when a day name is given.
func mkDate(year, month, day int, dayName string, hh, mm, ss int) int64 {
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return 0
	}
	t := time.Date(year, time.Month(month), day, hh, mm, ss, 0, time.Local)
	if name, ok := weekdayNames[strings.ToLower(dayName)]; ok && t.Weekday() != name {
		return 0
	}
	return t.Unix()
}

func mkDateNoWeekday(year, month, day, hh, mm, ss int) int64 {
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return 0
	}
	return time.Date(year, time.Month(month), day, hh, mm, ss, 0, time.Local).Unix()
}

func atoi(s string) int {
	if s == "" {
		return 0
	}
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}
