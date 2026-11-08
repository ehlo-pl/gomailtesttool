// Package report aggregates stored DMARC records into period summaries and
// renders them as text, Markdown, CSV, JSON or self-contained HTML.
package report

import (
	"fmt"
	"strings"
	"time"
)

// Period is a bucketing granularity.
type Period string

// Supported periods.
const (
	Day     Period = "day"
	Week    Period = "week"
	Month   Period = "month"
	Quarter Period = "quarter"
	Year    Period = "year"
	Total   Period = "total"
)

// ParsePeriod validates a period name.
func ParsePeriod(s string) (Period, error) {
	switch p := Period(strings.ToLower(strings.TrimSpace(s))); p {
	case Day, Week, Month, Quarter, Year, Total:
		return p, nil
	}
	return "", fmt.Errorf("invalid period %q (day, week, month, quarter, year, total)", s)
}

// Bucket returns the start of the period containing t (in loc), the start of
// the next period and a human label. For Total it returns zero times.
func Bucket(t time.Time, p Period, loc *time.Location) (start, next time.Time, label string) {
	t = t.In(loc)
	y, m, d := t.Date()
	switch p {
	case Day:
		start = time.Date(y, m, d, 0, 0, 0, 0, loc)
		return start, start.AddDate(0, 0, 1), start.Format("2006-01-02")
	case Week:
		// ISO 8601: weeks start on Monday.
		offset := (int(t.Weekday()) + 6) % 7
		start = time.Date(y, m, d-offset, 0, 0, 0, 0, loc)
		iy, iw := start.ISOWeek()
		return start, start.AddDate(0, 0, 7), fmt.Sprintf("%d-W%02d", iy, iw)
	case Month:
		start = time.Date(y, m, 1, 0, 0, 0, 0, loc)
		return start, start.AddDate(0, 1, 0), start.Format("2006-01")
	case Quarter:
		q := (int(m)-1)/3 + 1
		start = time.Date(y, time.Month((q-1)*3+1), 1, 0, 0, 0, 0, loc)
		return start, start.AddDate(0, 3, 0), fmt.Sprintf("%d-Q%d", y, q)
	case Year:
		start = time.Date(y, 1, 1, 0, 0, 0, 0, loc)
		return start, start.AddDate(1, 0, 0), fmt.Sprintf("%d", y)
	}
	return time.Time{}, time.Time{}, "total"
}

// ParseSince parses an absolute date (2006-01-02 or RFC 3339) or a relative
// duration back from now ("30d", "12h", "2w").
func ParseSince(s string, now time.Time, loc *time.Location) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	if d, ok, err := ParseRelative(s); ok {
		if err != nil {
			return time.Time{}, err
		}
		return now.Add(-d), nil
	}
	return ParseDate(s, loc)
}

// ParseDate parses 2006-01-02 (midnight in loc) or RFC 3339.
func ParseDate(s string, loc *time.Location) (time.Time, error) {
	if t, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("invalid date %q (use YYYY-MM-DD, RFC 3339 or a relative value like 30d)", s)
}

// ParseRelative parses "<n>d", "<n>w", "<n>h", "<n>m" (minutes) or any
// time.ParseDuration value. ok is false if s does not look relative.
func ParseRelative(s string) (d time.Duration, ok bool, err error) {
	if s == "" || s[0] < '0' || s[0] > '9' || strings.Count(s, "-") > 0 {
		return 0, false, nil
	}
	unit := s[len(s)-1]
	var n int
	if unit == 'd' || unit == 'w' {
		if _, err := fmt.Sscanf(s[:len(s)-1], "%d", &n); err != nil || n < 0 {
			return 0, true, fmt.Errorf("invalid relative duration %q", s)
		}
		if unit == 'd' {
			return time.Duration(n) * 24 * time.Hour, true, nil
		}
		return time.Duration(n) * 7 * 24 * time.Hour, true, nil
	}
	d, err = time.ParseDuration(s)
	if err != nil {
		return 0, true, fmt.Errorf("invalid relative duration %q", s)
	}
	return d, true, nil
}
