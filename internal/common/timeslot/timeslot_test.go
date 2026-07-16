//go:build !integration

package timeslot

import (
	"testing"
	"time"
)

// mustTime parses an RFC3339 timestamp or fails the test.
func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("bad test time %q: %v", s, err)
	}
	return parsed
}

func TestFindFreeSlots_Scenarios(t *testing.T) {
	// Monday 2026-07-20 is the anchor working day.
	tests := []struct {
		name        string
		busy        []Interval
		windowStart string
		windowEnd   string
		opts        Options
		want        []string // "start|end" RFC3339 pairs
	}{
		{
			name:        "empty busy returns first grid slots",
			windowStart: "2026-07-20T08:00:00Z",
			windowEnd:   "2026-07-20T17:00:00Z",
			opts:        Options{Duration: 30 * time.Minute, Count: 3, WorkDayStart: 8, WorkDayEnd: 17, WorkdaysOnly: true},
			want: []string{
				"2026-07-20T08:00:00Z|2026-07-20T08:30:00Z",
				"2026-07-20T08:30:00Z|2026-07-20T09:00:00Z",
				"2026-07-20T09:00:00Z|2026-07-20T09:30:00Z",
			},
		},
		{
			name: "busy block pushes slots past it",
			busy: []Interval{
				{Start: mustTime(t, "2026-07-20T08:00:00Z"), End: mustTime(t, "2026-07-20T09:15:00Z")},
			},
			windowStart: "2026-07-20T08:00:00Z",
			windowEnd:   "2026-07-20T17:00:00Z",
			opts:        Options{Duration: 30 * time.Minute, Count: 2, WorkDayStart: 8, WorkDayEnd: 17, WorkdaysOnly: true},
			want: []string{
				"2026-07-20T09:30:00Z|2026-07-20T10:00:00Z",
				"2026-07-20T10:00:00Z|2026-07-20T10:30:00Z",
			},
		},
		{
			name: "overlapping busy intervals are merged",
			busy: []Interval{
				{Start: mustTime(t, "2026-07-20T08:00:00Z"), End: mustTime(t, "2026-07-20T10:00:00Z")},
				{Start: mustTime(t, "2026-07-20T09:00:00Z"), End: mustTime(t, "2026-07-20T11:00:00Z")},
			},
			windowStart: "2026-07-20T08:00:00Z",
			windowEnd:   "2026-07-20T17:00:00Z",
			opts:        Options{Duration: 60 * time.Minute, Count: 1, WorkDayStart: 8, WorkDayEnd: 17, WorkdaysOnly: true},
			want:        []string{"2026-07-20T11:00:00Z|2026-07-20T12:00:00Z"},
		},
		{
			name:        "slot must not cross end of working day",
			windowStart: "2026-07-20T16:00:00Z",
			windowEnd:   "2026-07-21T09:30:00Z",
			opts:        Options{Duration: 90 * time.Minute, Count: 1, WorkDayStart: 8, WorkDayEnd: 17, WorkdaysOnly: true},
			// 16:00+90m = 17:30 > 17:00, so first fit is next day 08:00.
			want: []string{"2026-07-21T08:00:00Z|2026-07-21T09:30:00Z"},
		},
		{
			name:        "weekend is skipped",
			windowStart: "2026-07-18T08:00:00Z", // Saturday
			windowEnd:   "2026-07-20T12:00:00Z",
			opts:        Options{Duration: 30 * time.Minute, Count: 1, WorkDayStart: 8, WorkDayEnd: 17, WorkdaysOnly: true},
			want:        []string{"2026-07-20T08:00:00Z|2026-07-20T08:30:00Z"}, // Monday
		},
		{
			name:        "unaligned window start rounds up to grid",
			windowStart: "2026-07-20T08:10:00Z",
			windowEnd:   "2026-07-20T17:00:00Z",
			opts:        Options{Duration: 30 * time.Minute, Count: 1, WorkDayStart: 8, WorkDayEnd: 17, WorkdaysOnly: true},
			want:        []string{"2026-07-20T08:30:00Z|2026-07-20T09:00:00Z"},
		},
		{
			name: "fully busy window returns no slots",
			busy: []Interval{
				{Start: mustTime(t, "2026-07-20T00:00:00Z"), End: mustTime(t, "2026-07-21T00:00:00Z")},
			},
			windowStart: "2026-07-20T08:00:00Z",
			windowEnd:   "2026-07-20T17:00:00Z",
			opts:        Options{Duration: 30 * time.Minute, Count: 3, WorkDayStart: 8, WorkDayEnd: 17, WorkdaysOnly: true},
			want:        nil,
		},
		{
			name:        "no working-hours constraint allows night slots",
			windowStart: "2026-07-20T01:00:00Z",
			windowEnd:   "2026-07-20T02:00:00Z",
			opts:        Options{Duration: 30 * time.Minute, Count: 1},
			want:        []string{"2026-07-20T01:00:00Z|2026-07-20T01:30:00Z"},
		},
		{
			name:        "invalid options return nil",
			windowStart: "2026-07-20T08:00:00Z",
			windowEnd:   "2026-07-20T17:00:00Z",
			opts:        Options{Duration: 0, Count: 3},
			want:        nil,
		},
		{
			name:        "inverted window returns nil",
			windowStart: "2026-07-20T17:00:00Z",
			windowEnd:   "2026-07-20T08:00:00Z",
			opts:        Options{Duration: 30 * time.Minute, Count: 1},
			want:        nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindFreeSlots(tt.busy, mustTime(t, tt.windowStart), mustTime(t, tt.windowEnd), tt.opts)
			if len(got) != len(tt.want) {
				t.Fatalf("FindFreeSlots() returned %d slots, want %d: %v", len(got), len(tt.want), got)
			}
			for i, slot := range got {
				gotStr := slot.Start.Format(time.RFC3339) + "|" + slot.End.Format(time.RFC3339)
				if gotStr != tt.want[i] {
					t.Errorf("slot %d = %s, want %s", i, gotStr, tt.want[i])
				}
			}
		})
	}
}

func TestAddWorkingDays(t *testing.T) {
	tests := []struct {
		name  string
		start string
		days  int
		want  string
	}{
		{"monday plus one is tuesday", "2026-07-20T09:00:00Z", 1, "2026-07-21T09:00:00Z"},
		{"friday plus one skips weekend", "2026-07-17T09:00:00Z", 1, "2026-07-20T09:00:00Z"},
		{"monday plus five is next monday", "2026-07-20T09:00:00Z", 5, "2026-07-27T09:00:00Z"},
		{"zero days is identity", "2026-07-20T09:00:00Z", 0, "2026-07-20T09:00:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AddWorkingDays(mustTime(t, tt.start), tt.days)
			if !got.Equal(mustTime(t, tt.want)) {
				t.Errorf("AddWorkingDays() = %s, want %s", got.Format(time.RFC3339), tt.want)
			}
		})
	}
}
