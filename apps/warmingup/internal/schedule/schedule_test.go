//go:build !integration

package schedule

import (
	"math/rand/v2"
	"testing"
	"time"
)

func defaults(t *testing.T) Params {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	days := map[time.Weekday]bool{time.Monday: true, time.Tuesday: true, time.Wednesday: true, time.Thursday: true, time.Friday: true}
	return Params{
		Location: loc, Business: &Hours{Start: 8 * 60, End: 17 * 60, Days: days},
		Holidays: map[string]bool{"2026-12-25": true},
		RampDays: 30, StartPerDay: 1, BusinessPerHour: 3, OffMin: 1, OffMax: 2, MinGap: 10 * time.Minute,
	}
}

func TestTotal(t *testing.T) {
	p := defaults(t)
	for day, want := range map[int]int{1: 1, 8: 2, 15: 5, 22: 11, 30: 29, 31: 29, 100: 29} {
		if got := p.Total(day); got != want {
			t.Errorf("Total(%d) = %d, want %d", day, got, want)
		}
	}
	p.Linear = true
	if got := p.Total(15); got != 15 {
		t.Errorf("linear Total(15) = %d, want 15", got)
	}
	p.MaxPerDay = 10
	if got := p.Total(30); got != 10 {
		t.Errorf("capped Total(30) = %d, want 10", got)
	}
}

func TestQuota(t *testing.T) {
	p := defaults(t)
	rng := rand.New(rand.NewPCG(1, 2))
	monday := time.Date(2026, 12, 7, 12, 0, 0, 0, p.Location)
	saturday := monday.AddDate(0, 0, 5)
	christmas := time.Date(2026, 12, 25, 12, 0, 0, 0, p.Location)

	if b, o := p.Quota(1, monday, rng); b != 1 || o != 0 {
		t.Errorf("day 1 = %d+%d, want 1+0", b, o)
	}
	for range 50 {
		b, o := p.Quota(30, monday, rng)
		if b > 27 || o < 1 || o > 2 || b+o < 28 || b+o > 29 {
			t.Fatalf("peak = %d+%d, want about 27 + 1..2", b, o)
		}
		for _, date := range []time.Time{saturday, christmas} {
			if b, o := p.Quota(30, date, rng); b != 0 || o < 1 || o > 2 {
				t.Fatalf("non-business day = %d+%d, want 0 + 1..2", b, o)
			}
		}
	}
}

func TestSlots(t *testing.T) {
	p := defaults(t)
	// The DST changes of 2026 are 29 March and 25 October.
	for _, date := range []string{"2026-12-07", "2026-03-30", "2026-10-26", "2026-03-29", "2026-10-25"} {
		day, _ := time.ParseInLocation("2006-01-02", date, p.Location)
		for seed := range uint64(50) {
			rng := rand.New(rand.NewPCG(seed, 7))
			business, other := p.Quota(30, day, rng)
			slots := p.Slots(day, business, other, rng)
			if len(slots) != business+other {
				t.Fatalf("%s: %d slots, want %d", date, len(slots), business+other)
			}
			perHour := map[int]int{}
			inBusiness := 0
			for i, s := range slots {
				if s.Format("2006-01-02") != date {
					t.Fatalf("%s: slot %s on another day", date, s)
				}
				if p.IsBusinessDay(day) && s.Hour() >= 8 && s.Hour() < 17 {
					inBusiness++
					perHour[s.Hour()]++
				}
				// Spring-forward nights can squeeze wall-clock gaps.
				if i > 0 && date != "2026-03-29" && s.Sub(slots[i-1]) < p.MinGap {
					t.Fatalf("%s: gap %s between %s and %s", date, s.Sub(slots[i-1]), slots[i-1], s)
				}
			}
			if inBusiness != business {
				t.Fatalf("%s: %d slots in business hours, want %d (%v)", date, inBusiness, business, slots)
			}
			for h, n := range perHour {
				if n > 3 {
					t.Fatalf("%s: %d messages in hour %d", date, n, h)
				}
			}
		}
	}
}

func TestFlatRate(t *testing.T) {
	p := defaults(t)
	p.Business, p.PerDay = nil, 48
	if err := p.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	rng := rand.New(rand.NewPCG(3, 4))
	day := time.Date(2026, 12, 7, 0, 0, 0, 0, p.Location)
	b, o := p.Quota(30, day, rng)
	if b != 0 || o != 48 || len(p.Slots(day, b, o, rng)) != 48 {
		t.Errorf("flat quota = %d+%d", b, o)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Params)
	}{
		{"gap too long for the hourly rate", func(p *Params) { p.BusinessPerHour = 12 }},
		{"off range inverted", func(p *Params) { p.OffMin, p.OffMax = 3, 1 }},
		{"start above peak", func(p *Params) { p.StartPerDay = 40 }},
		{"flat without per_day", func(p *Params) { p.Business = nil }},
		{"no ramp days", func(p *Params) { p.RampDays = 0 }},
	}
	if err := defaults(t).Validate(); err != nil {
		t.Fatalf("defaults: %v", err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := defaults(t)
			tt.change(&p)
			if p.Validate() == nil {
				t.Error("Validate() = nil, want error")
			}
		})
	}
}
