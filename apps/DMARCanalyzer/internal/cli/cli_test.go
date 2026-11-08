package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"

	"github.com/ehlo-pl/gomailtesttool/apps/DMARCanalyzer/internal/config"
	"github.com/ehlo-pl/gomailtesttool/apps/DMARCanalyzer/internal/report"
	"github.com/ehlo-pl/gomailtesttool/apps/DMARCanalyzer/internal/store"
	tu "github.com/ehlo-pl/gomailtesttool/apps/DMARCanalyzer/internal/testutil"
	"github.com/ehlo-pl/gomailtesttool/internal/common/fetch"
)

var day = time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)

func run(t *testing.T, args ...string) string {
	t.Helper()
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out.String())
	}
	return out.String()
}

func TestExtractAndReportEndToEnd(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in")
	_ = os.MkdirAll(in, 0o700)
	write := func(name string, data []byte) { _ = os.WriteFile(filepath.Join(in, name), data, 0o600) }
	g := tu.Sample("Google", "g-1", "example.com", day).XML()
	write("google.eml", tu.EML("Report domain: example.com", tu.Attachment{ContentType: "application/zip", Filename: "g.zip", Data: tu.Zip(map[string][]byte{"g.xml": g})}))
	write("google-dup.eml", tu.EML("Report domain: example.com (resent)", tu.Attachment{ContentType: "application/gzip", Filename: "g.xml.gz", Data: tu.Gzip(g)}))
	write("yahoo.xml.gz", tu.Gzip(tu.Sample("Yahoo", "y-1", "example.com", day.AddDate(0, 1, 0)).XML()))
	write("hello.eml", tu.EML("Hello"))
	write("broken.zip", []byte("PK\x03\x04garbage"))

	db := filepath.Join(dir, "dmarc.db")
	common := []string{"--datadir", dir, "--db", db}

	out := run(t, append([]string{"extract", "--input", in}, common...)...)
	for _, want := range []string{"Files: 5", "New reports: 2", "Duplicates: 1", "No report: 1", "Errors: 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("extract output missing %q:\n%s", want, out)
		}
	}
	out = run(t, append([]string{"extract", "--input", in}, common...)...)
	if !strings.Contains(out, "unchanged 5") {
		t.Errorf("second extract should skip unchanged files:\n%s", out)
	}

	out = run(t, append([]string{"report", "--format", "json", "--period", "month"}, common...)...)
	var s report.Summary
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if s.Overall.Messages != 30 || s.Overall.Reports != 2 || len(s.Periods) != 2 {
		t.Errorf("summary = %+v periods=%d", s.Overall, len(s.Periods))
	}

	out = run(t, append([]string{"report", "--from", "2026-11-01"}, common...)...)
	if !strings.Contains(out, "Messages: 15") || !strings.Contains(out, "Yahoo") || strings.Contains(out, "Google") {
		t.Errorf("filtered text report:\n%s", out)
	}

	html := filepath.Join(dir, "r.html")
	run(t, append([]string{"report", "--format", "html", "--out", html}, common...)...)
	if b, err := os.ReadFile(html); err != nil || !bytes.Contains(b, []byte("<svg")) {
		t.Errorf("html report: %v", err)
	}
	csvDir := filepath.Join(dir, "csv")
	run(t, append([]string{"report", "--format", "csv", "--section", "all", "--out", csvDir}, common...)...)
	if _, err := os.Stat(filepath.Join(csvDir, "failing.csv")); err != nil {
		t.Errorf("csv dir: %v", err)
	}

	out = run(t, append([]string{"db", "stats"}, common...)...)
	if !strings.Contains(out, "Reports:          2") {
		t.Errorf("stats:\n%s", out)
	}
}

func TestReportFlagValidation(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{
		{"report", "--from", "2026-01-01", "--last", "7d"},
		{"report", "--period", "decade"},
		{"report", "--format", "pdf"},
		{"report", "--section", "failing"},
		{"report", "--format", "csv", "--section", "all"},
		{"report", "--from", "2026-02-01", "--to", "2026-01-01"},
		{"report", "--tz", "Mars/Olympus"},
	} {
		cmd := NewRootCmd()
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs(append(args, "--datadir", dir))
		if err := cmd.Execute(); err == nil {
			t.Errorf("%v: expected error", args)
		}
	}
}

// stubSession is an in-memory fetch.Session.
type stubSession struct {
	t        *testing.T
	msgs     []fetch.RawMessage
	deleted  []string
	since    time.Time
	fetching bool
}

func (s *stubSession) Fetch(ctx context.Context, c fetch.Criteria, sink fetch.Sink) error {
	s.fetching = true
	defer func() { s.fetching = false }()
	s.since = c.Since
	for _, m := range s.msgs {
		if c.SkipKey != nil && c.SkipKey(m.Key) {
			continue
		}
		if err := sink(ctx, m); err != nil {
			return err
		}
	}
	return nil
}
func (s *stubSession) Delete(_ context.Context, key string) error {
	if s.fetching {
		s.t.Error("Delete called while Fetch is still paging")
	}
	s.deleted = append(s.deleted, key)
	return nil
}
func (s *stubSession) Close() error { return nil }

func TestRunCollectIncrementalAndDelete(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(dir, "dmarc.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	e := &env{cfg: &config.Config{DataDir: dir}, log: slog.New(slog.NewTextHandler(io.Discard, nil)), store: st}

	recv := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	report := tu.EML("Report", tu.Attachment{ContentType: "application/gzip", Filename: "r.xml.gz",
		Data: tu.Gzip(tu.Sample("Google", "g-1", "example.com", day).XML())})
	sess := &stubSession{t: t, msgs: []fetch.RawMessage{
		{Key: "AAMk/abc=", Received: recv, Raw: report},
		{Key: "aamk/abc=", Received: recv, Raw: tu.EML("Hello")}, // differs only by case
	}}
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	co := &collectOpts{protocol: "msgraph", extract: true, deleteAfter: true, overlap: 48 * time.Hour,
		now:  func() time.Time { return now },
		open: func(context.Context, bool) (fetch.Session, string, error) { return sess, "dmarc@example.com", nil }}

	stats, err := runCollect(ctx, e, co, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Downloaded != 2 || stats.Extract.Reports != 1 || stats.Deleted != 1 {
		t.Errorf("stats = %+v", stats)
	}
	if len(sess.deleted) != 1 || sess.deleted[0] != "AAMk/abc=" {
		t.Errorf("deleted = %v (non-report mail must not be deleted)", sess.deleted)
	}
	if !sess.since.Equal(now.Add(-defaultLookback)) {
		t.Errorf("first run since = %v", sess.since)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "eml", "msgraph", "2026-11", "*.eml"))
	if len(files) != 2 {
		t.Errorf("eml files = %v", files)
	}
	if runtime.GOOS != "windows" {
		if fi, err := os.Stat(files[0]); err == nil && fi.Mode().Perm() != 0o600 {
			t.Errorf("eml mode = %v", fi.Mode().Perm())
		}
	}

	// Second run: nothing new, window starts at last run - overlap.
	now2 := now.Add(24 * time.Hour)
	co.now = func() time.Time { return now2 }
	sess.deleted = nil
	stats, err = runCollect(ctx, e, co, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Downloaded != 0 || len(sess.deleted) != 0 {
		t.Errorf("second run stats = %+v deleted=%v", stats, sess.deleted)
	}
	if !sess.since.Equal(now.Add(-48 * time.Hour)) {
		t.Errorf("incremental since = %v, want %v", sess.since, now.Add(-48*time.Hour))
	}
}

func TestCollectOptsValidation(t *testing.T) {
	tests := []struct {
		set     map[string]any
		proto   string
		wantErr bool
	}{
		{map[string]any{"overlap": "2d", "extract": true}, "imap", false},
		{map[string]any{"overlap": "2d", "extract": true, "delete-after": true}, "imap", true},
		{map[string]any{"overlap": "2d", "extract": false, "delete-after": true, "yes": true}, "imap", true},
		{map[string]any{"overlap": "2d", "query": "has:attachment"}, "imap", true},
		{map[string]any{"overlap": "2d", "query": "has:attachment"}, "gmail", false},
		{map[string]any{"overlap": "soon"}, "imap", true},
		{map[string]any{"overlap": "2d", "since": "bogus"}, "imap", true},
		{map[string]any{"overlap": "2d", "max": -1}, "imap", true},
	}
	for i, tt := range tests {
		v := newViper(tt.set)
		_, err := collectOptsFromViper(v, tt.proto, time.Now())
		if (err != nil) != tt.wantErr {
			t.Errorf("case %d: err = %v, wantErr %v", i, err, tt.wantErr)
		}
	}
}

func newViper(set map[string]any) *viper.Viper {
	v := viper.New()
	for k, val := range set {
		v.Set(k, val)
	}
	return v
}

func TestEmlName(t *testing.T) {
	a, b := emlName("AAMk/abc="), emlName("aamk/abc=")
	if strings.EqualFold(a, b) {
		t.Errorf("case-only different keys collide: %s %s", a, b)
	}
	if strings.ContainsAny(a, `/\:=`) || !strings.HasSuffix(a, ".eml") {
		t.Errorf("unsafe name %q", a)
	}
	if n := emlName(strings.Repeat("x", 500)); len(n) > 70 {
		t.Errorf("name too long: %d", len(n))
	}
}

func TestReportDefaultsFromConfigSection(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "dmarc.yaml")
	yaml := "dmarc:\n  period: week\n  format: json\nimap:\n  host: imap.example.com\n"
	if err := os.WriteFile(cfg, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	out := run(t, "report", "--config", cfg, "--datadir", dir)
	var s report.Summary
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		t.Fatalf("config format: json not applied: %v\n%s", err, out)
	}
	if s.Period != report.Week {
		t.Errorf("period = %q, want week from config", s.Period)
	}
	// CLI flag still wins over the config file.
	out = run(t, "report", "--config", cfg, "--datadir", dir, "--format", "text")
	if !strings.Contains(out, "Period: week") {
		t.Errorf("flag override / config period:\n%s", out)
	}
}
