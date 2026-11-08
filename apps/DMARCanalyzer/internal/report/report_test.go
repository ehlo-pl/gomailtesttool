package report

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ehlo-pl/gomailtesttool/apps/DMARCanalyzer/internal/store"
)

func TestBucket(t *testing.T) {
	warsaw, _ := time.LoadLocation("Europe/Warsaw")
	tests := []struct {
		t     time.Time
		p     Period
		loc   *time.Location
		label string
		start string
	}{
		{time.Date(2026, 11, 8, 15, 0, 0, 0, time.UTC), Day, time.UTC, "2026-11-08", "2026-11-08"},
		{time.Date(2026, 11, 8, 15, 0, 0, 0, time.UTC), Week, time.UTC, "2026-W45", "2026-11-02"}, // Sunday -> Monday before
		{time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), Week, time.UTC, "2026-W53", "2026-12-28"},   // ISO year boundary
		{time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC), Month, time.UTC, "2026-11", "2026-11-01"},
		{time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC), Quarter, time.UTC, "2026-Q4", "2026-10-01"},
		{time.Date(2026, 2, 8, 0, 0, 0, 0, time.UTC), Year, time.UTC, "2026", "2026-01-01"},
		{time.Date(2026, 10, 31, 23, 30, 0, 0, time.UTC), Month, warsaw, "2026-11", "2026-11-01"}, // tz shifts month
	}
	for _, tt := range tests {
		start, next, label := Bucket(tt.t, tt.p, tt.loc)
		if label != tt.label || start.Format("2006-01-02") != tt.start || !next.After(start) {
			t.Errorf("Bucket(%v,%s) = %s %v %v, want %s %s", tt.t, tt.p, label, start, next, tt.label, tt.start)
		}
	}
}

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		in   string
		want time.Time
		err  bool
	}{
		{"30d", now.AddDate(0, 0, -30), false},
		{"2w", now.AddDate(0, 0, -14), false},
		{"12h", now.Add(-12 * time.Hour), false},
		{"2026-10-01", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), false},
		{"2026-10-01T05:00:00Z", time.Date(2026, 10, 1, 5, 0, 0, 0, time.UTC), false},
		{"", time.Time{}, false},
		{"yesterday", time.Time{}, true},
		{"5x", time.Time{}, true},
	}
	for _, tt := range tests {
		got, err := ParseSince(tt.in, now, time.UTC)
		if (err != nil) != tt.err || !got.Equal(tt.want) {
			t.Errorf("ParseSince(%q) = %v, %v", tt.in, got, err)
		}
	}
}

func rows() []store.RecordRow {
	d1 := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	mk := func(id int64, org string, begin time.Time, ip string, n int64, pass bool, dkim, spf, disp, p string) store.RecordRow {
		return store.RecordRow{ReportDBID: id, OrgName: org, Begin: begin, End: begin.Add(24 * time.Hour),
			PolicyDomain: "example.com", P: p, Pct: 100, SourceIP: ip, Count: n, DMARCPass: pass, DKIM: dkim, SPF: spf,
			Disposition: disp, HeaderFrom: "example.com", DKIMAuth: "example.com/s1=" + dkim, SPFAuth: "example.com=" + spf}
	}
	r := []store.RecordRow{
		mk(1, "Google", d1, "192.0.2.10", 90, true, "pass", "pass", "none", "none"),
		mk(1, "Google", d1, "198.51.100.7", 10, false, "fail", "fail", "none", "none"),
		mk(2, "Yahoo", d2, "192.0.2.10", 95, true, "pass", "fail", "none", "quarantine"),
		mk(2, "Yahoo", d2, "198.51.100.7", 5, false, "fail", "fail", "quarantine", "quarantine"),
	}
	r[3].Reasons = "local_policy"
	return r
}

func TestBuild(t *testing.T) {
	s := Build(context.Background(), rows(), Options{Period: Month,
		Resolver: func(_ context.Context, ip string) string { return "host-" + ip },
		Now:      func() time.Time { return time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC) }})

	o := s.Overall
	if o.Messages != 200 || o.Pass != 185 || o.Fail != 15 || o.PassPct != 92.5 || o.Quarantine != 5 {
		t.Errorf("overall = %+v", o)
	}
	if o.DKIMPct != 92.5 || o.SPFPct != 45 || o.Reports != 2 || o.Reporters != 2 || o.SourceIPs != 2 {
		t.Errorf("overall rates = %+v", o)
	}
	if len(s.Periods) != 2 || s.Periods[0].Label != "2026-10" || s.Periods[1].PassPct != 95 {
		t.Fatalf("periods = %+v", s.Periods)
	}
	if s.Periods[1].DeltaPassPct == nil || *s.Periods[1].DeltaPassPct != 5 {
		t.Errorf("delta = %v", s.Periods[1].DeltaPassPct)
	}
	if len(s.PolicyChanges) != 1 || s.PolicyChanges[0].To != "p=quarantine sp=- pct=100" {
		t.Errorf("policy changes = %+v", s.PolicyChanges)
	}
	if len(s.Domains) != 1 || s.Domains[0].Policy != "p=quarantine sp=- pct=100" {
		t.Errorf("domains = %+v", s.Domains)
	}
	if len(s.Failing) != 2 || s.Failing[0].Messages != 10 || s.Failing[0].PTR != "host-198.51.100.7" {
		t.Errorf("failing = %+v", s.Failing)
	}
	if len(s.Reasons) != 1 || s.Reasons[0].Messages != 5 {
		t.Errorf("reasons = %+v", s.Reasons)
	}
	if s.TopSources[0].IP != "192.0.2.10" {
		t.Errorf("top source = %+v", s.TopSources[0])
	}

	s2 := Build(context.Background(), rows(), Options{Period: Total, FailingOnly: true})
	if len(s2.Periods) != 1 || len(s2.TopSources) != 1 || s2.TopSources[0].IP != "198.51.100.7" {
		t.Errorf("failing-only/total = %+v / %+v", s2.Periods, s2.TopSources)
	}
}

func TestRenderAllFormats(t *testing.T) {
	r := rows()
	r[1].OrgName = `<script>alert(1)</script>`
	r[1].HeaderFrom = "=cmd|' /C calc'!A0"
	s := Build(context.Background(), r, Options{Period: Week})

	for _, f := range []Format{FormatText, FormatMD, FormatCSV, FormatJSON, FormatHTML} {
		var buf bytes.Buffer
		if err := Render(&buf, s, f, ""); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		out := buf.String()
		if buf.Len() == 0 {
			t.Errorf("%s: empty output", f)
		}
		switch f {
		case FormatHTML:
			if strings.Contains(out, "<script>alert") {
				t.Error("html: reporter string not escaped")
			}
			if !strings.Contains(out, "<svg") {
				t.Error("html: missing chart")
			}
		case FormatMD:
			if strings.Contains(out, "<script>") {
				t.Error("md: reporter string not escaped")
			}
		case FormatJSON:
			var back Summary
			if err := json.Unmarshal(buf.Bytes(), &back); err != nil || back.Overall.Messages != 200 {
				t.Errorf("json round trip: %v %+v", err, back.Overall)
			}
		case FormatCSV:
			recs, err := csv.NewReader(&buf).ReadAll()
			if err != nil || len(recs) != 3 { // header + 2 weeks
				t.Errorf("csv: %v %d", err, len(recs))
			}
		}
	}

	var buf bytes.Buffer
	if err := Render(&buf, s, FormatCSV, "failing"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "'=cmd") {
		t.Errorf("csv formula injection not neutralized:\n%s", buf.String())
	}
	if err := Render(&buf, s, FormatCSV, "nope"); err == nil {
		t.Error("expected unknown section error")
	}

	files, err := WriteCSVDir(t.TempDir(), s)
	if err != nil || len(files) != len(CSVSections) {
		t.Errorf("WriteCSVDir: %v %v", files, err)
	}
}

func TestRenderEmpty(t *testing.T) {
	s := Build(context.Background(), nil, Options{})
	for _, f := range []Format{FormatText, FormatMD, FormatHTML, FormatJSON} {
		var buf bytes.Buffer
		if err := Render(&buf, s, f, ""); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}
