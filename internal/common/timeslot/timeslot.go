// Package timeslot computes free meeting slots from busy-interval data.
// It is shared by the protocol-specific findtimeslot actions so every
// protocol produces identical results from equivalent free/busy input.
package timeslot

import (
	"sort"
	"time"
)

// Interval is a half-open time interval [Start, End).
type Interval struct {
	Start time.Time
	End   time.Time
}

// Options controls the slot search.
type Options struct {
	Duration     time.Duration // slot length (required, > 0)
	Count        int           // maximum number of slots to return (required, > 0)
	WorkDayStart int           // working day start hour UTC (inclusive), 0-24
	WorkDayEnd   int           // working day end hour UTC (exclusive), 0-24
	WorkdaysOnly bool          // restrict slots to Monday-Friday
}

// grid is the granularity of candidate slot start times.
const grid = 30 * time.Minute

// FindFreeSlots returns up to opts.Count free slots of opts.Duration inside
// [windowStart, windowEnd), avoiding the given busy intervals. Candidate
// starts are aligned to a 30-minute grid. When opts.WorkDayStart/WorkDayEnd
// are set (start < end), slots must fall entirely within those hours (UTC);
// with opts.WorkdaysOnly, Saturdays and Sundays are skipped.
func FindFreeSlots(busy []Interval, windowStart, windowEnd time.Time, opts Options) []Interval {
	if opts.Duration <= 0 || opts.Count <= 0 || !windowEnd.After(windowStart) {
		return nil
	}

	merged := mergeIntervals(busy)

	var slots []Interval
	for candidate := roundUpToGrid(windowStart.UTC()); ; candidate = candidate.Add(grid) {
		slotEnd := candidate.Add(opts.Duration)
		if slotEnd.After(windowEnd) {
			break
		}
		if !withinWorkingHours(candidate, slotEnd, opts) {
			continue
		}
		if overlapsAny(merged, candidate, slotEnd) {
			continue
		}
		slots = append(slots, Interval{Start: candidate, End: slotEnd})
		if len(slots) >= opts.Count {
			break
		}
	}
	return slots
}

// AddWorkingDays adds the given number of working days (Monday-Friday) to t.
func AddWorkingDays(t time.Time, days int) time.Time {
	if days <= 0 {
		return t
	}
	result := t
	added := 0
	for added < days {
		result = result.Add(24 * time.Hour)
		weekday := result.Weekday()
		if weekday != time.Saturday && weekday != time.Sunday {
			added++
		}
	}
	return result
}

// roundUpToGrid rounds t up to the next grid boundary (no-op if aligned).
func roundUpToGrid(t time.Time) time.Time {
	rounded := t.Truncate(grid)
	if rounded.Before(t) {
		rounded = rounded.Add(grid)
	}
	return rounded
}

// mergeIntervals sorts the intervals and merges overlapping/adjacent ones,
// dropping empty or inverted entries.
func mergeIntervals(intervals []Interval) []Interval {
	valid := make([]Interval, 0, len(intervals))
	for _, iv := range intervals {
		if iv.End.After(iv.Start) {
			valid = append(valid, Interval{Start: iv.Start.UTC(), End: iv.End.UTC()})
		}
	}
	sort.Slice(valid, func(i, j int) bool { return valid[i].Start.Before(valid[j].Start) })

	var merged []Interval
	for _, iv := range valid {
		if len(merged) > 0 && !iv.Start.After(merged[len(merged)-1].End) {
			if iv.End.After(merged[len(merged)-1].End) {
				merged[len(merged)-1].End = iv.End
			}
			continue
		}
		merged = append(merged, iv)
	}
	return merged
}

// overlapsAny reports whether [start, end) overlaps any of the sorted,
// non-overlapping intervals.
func overlapsAny(sorted []Interval, start, end time.Time) bool {
	for _, iv := range sorted {
		if iv.Start.Before(end) && start.Before(iv.End) {
			return true
		}
		if !iv.Start.Before(end) {
			break // sorted: no later interval can overlap
		}
	}
	return false
}

// withinWorkingHours reports whether the slot [start, end) satisfies the
// working-hours and working-days constraints of opts. A slot must not cross
// the end of the working day.
func withinWorkingHours(start, end time.Time, opts Options) bool {
	if opts.WorkdaysOnly {
		weekday := start.Weekday()
		if weekday == time.Saturday || weekday == time.Sunday {
			return false
		}
	}
	if opts.WorkDayStart >= opts.WorkDayEnd {
		return true // no hour constraint configured
	}
	dayStart := time.Date(start.Year(), start.Month(), start.Day(), opts.WorkDayStart, 0, 0, 0, time.UTC)
	dayEnd := time.Date(start.Year(), start.Month(), start.Day(), opts.WorkDayEnd, 0, 0, 0, time.UTC)
	return !start.Before(dayStart) && !end.After(dayEnd)
}
