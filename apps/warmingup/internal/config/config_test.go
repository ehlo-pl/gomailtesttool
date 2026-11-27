//go:build !integration

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const global = `
endpoints:
  site-mta: { protocol: smtp, host: mail.example.com, port: 587, username: u, password: "${PW}" }
  bounce:   { protocol: imap, host: imap.example.com, imaps: true, username: bounce@example.com, password: p }
  gmail1:   { protocol: imap, host: imap.gmail.com, imaps: true, username: a@gmail.com, password: p, junk: "[Gmail]/Spam" }
  fm1:      { protocol: jmap, host: api.fastmail.com, accesstoken: t, address: b@fastmail.com, engage: false }
`

const minimal = `
domain: Example.com
sender: { endpoint: site-mta, from: "News <news@example.com>", ehloname: mail.example.com }
bounce: { endpoint: bounce }
targets: [gmail1, fm1]
campaign: { start: 2026-12-01 08:00 }
`

func load(t *testing.T, domains ...string) (*Config, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "domains.d"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "warmingup.yaml")
	if err := os.WriteFile(path, []byte(global), 0o600); err != nil {
		t.Fatal(err)
	}
	for i, d := range domains {
		if err := os.WriteFile(filepath.Join(dir, "domains.d", string(rune('a'+i))+".yaml"), []byte(d), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return Load(path, func(string) (string, bool) { return "secret", true })
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(t, minimal)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	d := cfg.Domain("example.com")
	if d == nil || !d.Enabled {
		t.Fatal("domain not loaded")
	}
	if d.Schedule.Peak() != 29 || d.Schedule.Business == nil || d.Schedule.Business.Start != 480 || d.Location.String() != "Europe/Warsaw" {
		t.Errorf("defaults: peak %d, hours %+v, tz %s", d.Schedule.Peak(), d.Schedule.Business, d.Location)
	}
	if d.EnvelopeFrom() != "bounce@example.com" || d.FromAddress != "news@example.com" {
		t.Errorf("envelope %s, from %s", d.EnvelopeFrom(), d.FromAddress)
	}
	if got := cfg.Endpoints[d.SenderEndpoint].String("ehloname"); got != "mail.example.com" {
		t.Errorf("ehloname = %q", got)
	}
	g, _ := d.Target("gmail1")
	f, _ := d.Target("fm1")
	if g.Address != "a@gmail.com" || !g.Engage || f.Address != "b@fastmail.com" || f.Engage {
		t.Errorf("targets = %+v", d.Targets)
	}
	if !d.End.IsZero() || d.Start.Hour() != 8 || d.Engage.MaxDelay != 4*time.Hour {
		t.Errorf("times: %v %v %v", d.Start, d.End, d.Engage)
	}
}

func TestLoadCustomSpeedPerDomain(t *testing.T) {
	other := strings.Replace(minimal, "Example.com", "example.org", 1) + `
business_hours: none
speed: { per_day: 12, ramp_days: 10, curve: linear, min_gap: 20m }
`
	third := strings.Replace(minimal, "Example.com", "example.net", 1) + `
business_hours: { start: "09:00", end: "15:00", days: [monday, tue] }
speed: { business_per_hour: 5, off_hours_per_day: [0, 0] }
`
	cfg, err := load(t, minimal, other, third)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if d := cfg.Domain("example.org"); d.Schedule.Business != nil || d.Schedule.Peak() != 12 || !d.Schedule.Linear {
		t.Errorf("flat domain = %+v", d.Schedule)
	}
	if d := cfg.Domain("example.net"); d.Schedule.Peak() != 30 || len(d.Schedule.Business.Days) != 2 {
		t.Errorf("custom hours domain = %+v", d.Schedule)
	}
}

func TestLoadInvalid(t *testing.T) {
	tests := []struct{ name, add, want string }{
		{"typo", "speeed: {}\n", "speeed"},
		{"typo in hours", "business_hours: { strat: \"08:00\" }\n", "strat"},
		{"end before start", "campaign: { start: 2026-12-01, end: 2026-11-01 }\n", "after campaign.start"},
		{"gap too long", "speed: { min_gap: 30m }\n", "do not fit"},
		{"flat without per_day", "business_hours: none\n", "per_day"},
		{"unknown target", "targets: [nope]\n", "not defined"},
		{"smtp as target", "targets: [site-mta]\n", "supported"},
		{"static content", "content: { sources: [static] }\n", "content.sources"},
		{"timezone", "timezone: Mars/Base\n", "timezone"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := minimal
			key, _, _ := strings.Cut(tt.add, ":")
			var kept []string
			for _, line := range strings.Split(doc, "\n") {
				if !strings.HasPrefix(line, key+":") {
					kept = append(kept, line)
				}
			}
			_, err := load(t, strings.Join(kept, "\n")+tt.add)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want %q", err, tt.want)
			}
		})
	}
	foreign := strings.Replace(minimal, "bounce: { endpoint: bounce }", "bounce: { endpoint: bounce, address: x@gmail.com }", 1)
	if _, err := load(t, foreign); err == nil || !strings.Contains(err.Error(), "align") {
		t.Errorf("foreign envelope domain error = %v", err)
	}
	if _, err := load(t, minimal, minimal); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Errorf("duplicate domain error = %v", err)
	}
}
