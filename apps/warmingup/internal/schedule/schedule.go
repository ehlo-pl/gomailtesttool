// Package schedule turns a ramp day into the number of messages of one
// calendar day and their send times. It has no state and does no I/O.
package schedule

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"time"
)

// Hours are the business hours, in minutes after local midnight.
type Hours struct {
	Start, End int
	Days       map[time.Weekday]bool
}

// Params describe how fast one domain sends.
type Params struct {
	Location *time.Location
	Business *Hours          // nil: one flat rate over the whole day
	Holidays map[string]bool // "2006-01-02"; treated as non-business days

	RampDays    int
	StartPerDay int
	Linear      bool

	// With business hours:
	BusinessPerHour int // peak per business hour
	OffMin, OffMax  int // peak per day outside business hours

	PerDay    int // peak without business hours
	MinGap    time.Duration
	MaxPerDay int // 0 = no extra cap
}

// Validate rejects combinations that cannot be scheduled.
func (p Params) Validate() error {
	switch {
	case p.RampDays < 1:
		return errors.New("ramp_days must be at least 1")
	case p.StartPerDay < 1:
		return errors.New("start_per_day must be at least 1")
	case p.MinGap < 0:
		return errors.New("min_gap must not be negative")
	case p.MaxPerDay < 0:
		return errors.New("max_per_day must not be negative")
	}
	if p.Business == nil {
		if p.PerDay < 1 {
			return errors.New("per_day is required when business_hours is \"none\"")
		}
		if time.Duration(p.Peak())*p.MinGap > 24*time.Hour {
			return fmt.Errorf("%d messages with min_gap %s do not fit into a day", p.Peak(), p.MinGap)
		}
	} else {
		if p.BusinessPerHour < 1 {
			return errors.New("business_per_hour must be at least 1")
		}
		if p.OffMin < 0 || p.OffMax < p.OffMin {
			return errors.New("off_hours_per_day must be [min, max] with 0 <= min <= max")
		}
		if time.Duration(p.BusinessPerHour)*p.MinGap > time.Hour {
			return fmt.Errorf("%d messages per hour with min_gap %s do not fit into an hour", p.BusinessPerHour, p.MinGap)
		}
		off := 24*time.Hour - time.Duration(p.Business.End-p.Business.Start)*time.Minute
		if p.OffMax > 0 && time.Duration(p.OffMax)*p.MinGap > off-p.MinGap {
			return fmt.Errorf("%d off-hours messages with min_gap %s do not fit outside business hours", p.OffMax, p.MinGap)
		}
	}
	if p.StartPerDay > p.Peak() {
		return fmt.Errorf("start_per_day %d exceeds the peak of %d per day", p.StartPerDay, p.Peak())
	}
	return nil
}

// businessCap is the most messages business hours of one day can take.
func (p Params) businessCap() int {
	if p.Business == nil {
		return 0
	}
	return (p.Business.End - p.Business.Start) / 60 * p.BusinessPerHour
}

// Peak is the daily total at the end of the ramp.
func (p Params) Peak() int {
	peak := p.PerDay
	if p.Business != nil {
		peak = p.businessCap() + p.OffMax
	}
	if p.MaxPerDay > 0 {
		peak = min(peak, p.MaxPerDay)
	}
	return peak
}

// Total is the daily total on ramp day (1-based) along the curve.
func (p Params) Total(day int) int {
	day = max(day, 1)
	peak := p.Peak()
	if day >= p.RampDays || p.RampDays == 1 {
		return peak
	}
	f := float64(day-1) / float64(p.RampDays-1)
	start := float64(p.StartPerDay)
	var v float64
	if p.Linear {
		v = start + (float64(peak)-start)*f
	} else {
		v = start * math.Pow(float64(peak)/start, f)
	}
	return min(max(int(math.Round(v)), 1), peak)
}

// IsBusinessDay reports whether date is a working day. Without business hours
// every day counts the same.
func (p Params) IsBusinessDay(date time.Time) bool {
	if p.Business == nil {
		return false
	}
	date = date.In(p.Location)
	return p.Business.Days[date.Weekday()] && !p.Holidays[date.Format("2006-01-02")]
}

// Quota splits the total of a ramp day into business-hours and other
// messages. Without business hours everything is "other".
func (p Params) Quota(day int, date time.Time, rng *rand.Rand) (business, other int) {
	total := p.Total(day)
	if p.Business == nil {
		return 0, total
	}
	off := p.OffMin
	if p.OffMax > p.OffMin {
		off += rng.IntN(p.OffMax - p.OffMin + 1)
	}
	if !p.IsBusinessDay(date) {
		return 0, min(total, off)
	}
	// Off-hours grow with the ramp; very small totals stay in business hours.
	other = int(math.Round(float64(off) * float64(total) / float64(p.Peak())))
	other = min(other, total)
	business = min(total-other, p.businessCap())
	return business, other
}

type span struct{ from, to time.Duration } // offsets from local midnight

// spread places n times into spans: the spans are treated as one line cut
// into n equal segments, one random point each, leaving gap at the end of
// every segment so neighbors are at least gap apart.
func spread(spans []span, n int, gap time.Duration, rng *rand.Rand) []time.Duration {
	var total time.Duration
	for _, s := range spans {
		total += s.to - s.from
	}
	if n <= 0 || total <= 0 {
		return nil
	}
	seg := total / time.Duration(n)
	out := make([]time.Duration, 0, n)
	for i := range n {
		pos := seg * time.Duration(i)
		if room := seg - gap; room > 0 {
			pos += time.Duration(rng.Int64N(int64(room)))
		}
		for _, s := range spans {
			if l := s.to - s.from; pos < l {
				out = append(out, s.from+pos)
				break
			} else {
				pos -= l
			}
		}
	}
	return out
}

// Slots returns the send times of one day, sorted. At most BusinessPerHour
// messages fall into any business hour.
func (p Params) Slots(date time.Time, business, other int, rng *rand.Rand) []time.Time {
	date = date.In(p.Location)
	var offsets []time.Duration
	day := 24 * time.Hour

	spans := []span{{0, day - p.MinGap}}
	if p.Business != nil && p.IsBusinessDay(date) {
		start := time.Duration(p.Business.Start) * time.Minute
		end := time.Duration(p.Business.End) * time.Minute
		spans = nil
		if start > p.MinGap {
			spans = append(spans, span{0, start - p.MinGap})
		}
		if day-p.MinGap > end {
			spans = append(spans, span{end, day - p.MinGap})
		}

		hours := (p.Business.End - p.Business.Start) / 60
		business = min(business, hours*p.BusinessPerHour)
		perHour := make([]int, hours)
		for i := range perHour {
			perHour[i] = business / hours
		}
		for _, i := range rng.Perm(hours)[:business%hours] {
			perHour[i]++
		}
		for h, n := range perHour {
			from := start + time.Duration(h)*time.Hour
			offsets = append(offsets, spread([]span{{from, from + time.Hour}}, n, p.MinGap, rng)...)
		}
	}
	offsets = append(offsets, spread(spans, other, p.MinGap, rng)...)

	out := make([]time.Time, 0, len(offsets))
	for _, o := range offsets {
		// Wall-clock arithmetic, so business hours stay put across DST changes.
		out = append(out, time.Date(date.Year(), date.Month(), date.Day(), 0, 0, int(o/time.Second), 0, p.Location))
	}
	slices.SortFunc(out, time.Time.Compare)
	return out
}
