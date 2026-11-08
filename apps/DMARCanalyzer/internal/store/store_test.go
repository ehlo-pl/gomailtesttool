package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ehlo-pl/gomailtesttool/apps/DMARCanalyzer/internal/dmarc"
	tu "github.com/ehlo-pl/gomailtesttool/apps/DMARCanalyzer/internal/testutil"
)

var day = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

func openMem(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func mustParse(t *testing.T, r tu.Report) *dmarc.Feedback {
	t.Helper()
	f, err := dmarc.Parse(r.XML())
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestInsertReportDedupe(t *testing.T) {
	ctx := context.Background()
	s := openMem(t)
	f := mustParse(t, tu.Sample("Google", "g-1", "example.com", day))

	ok, err := s.InsertReport(ctx, f, "sha", "a.eml")
	if err != nil || !ok {
		t.Fatalf("first insert: %v %v", ok, err)
	}
	ok, err = s.InsertReport(ctx, f, "sha", "b.eml")
	if err != nil || ok {
		t.Fatalf("duplicate insert: ok=%v err=%v", ok, err)
	}
	st, err := s.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Reports != 1 || st.Records != 3 || st.Messages != 15 || st.Reporters != 1 {
		t.Errorf("stats = %+v", st)
	}
}

func TestRecordsFilter(t *testing.T) {
	ctx := context.Background()
	s := openMem(t)
	for i, org := range []string{"Google", "Yahoo", "Microsoft"} {
		r := tu.Sample(org, org+"-1", "example.com", day.AddDate(0, 0, i))
		if _, err := s.InsertReport(ctx, mustParse(t, r), "x", ""); err != nil {
			t.Fatal(err)
		}
	}
	other := tu.Sample("Google", "g-2", "other.org", day)
	other.Rows[0].HeaderFrom = "mail.other.org"
	if _, err := s.InsertReport(ctx, mustParse(t, other), "x", ""); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		f    RecordFilter
		want int
	}{
		{"all", RecordFilter{}, 12},
		{"date range", RecordFilter{From: day.AddDate(0, 0, 1), To: day.AddDate(0, 0, 2)}, 3},
		{"domain", RecordFilter{Domains: []string{"example.com"}}, 9},
		{"subdomain header_from", RecordFilter{Domains: []string{"other.org"}}, 3},
		{"org", RecordFilter{Orgs: []string{"google"}}, 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, err := s.Records(ctx, tt.f)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != tt.want {
				t.Errorf("rows = %d, want %d", len(rows), tt.want)
			}
		})
	}

	rows, _ := s.Records(ctx, RecordFilter{Orgs: []string{"yahoo"}})
	if rows[1].DKIMAuth != "example.com/s1=fail" || rows[1].SPFAuth != "example.com=fail" || rows[1].DMARCPass {
		t.Errorf("auth detail = %+v", rows[1])
	}
}

func TestSourcesAndFiles(t *testing.T) {
	ctx := context.Background()
	s := openMem(t)
	src, err := s.EnsureSource(ctx, "imap", "DMARC@example.com", "INBOX")
	if err != nil {
		t.Fatal(err)
	}
	again, _ := s.EnsureSource(ctx, "imap", "dmarc@example.com", "inbox")
	if again.ID != src.ID || !again.LastCollectAt.IsZero() {
		t.Fatalf("source not reused: %+v %+v", src, again)
	}
	if err := s.SetLastCollect(ctx, src.ID, day); err != nil {
		t.Fatal(err)
	}
	again, _ = s.EnsureSource(ctx, "imap", "dmarc@example.com", "inbox")
	if !again.LastCollectAt.Equal(day) {
		t.Errorf("last collect = %v", again.LastCollectAt)
	}

	if ok, _ := s.IsFetched(ctx, src.ID, "1:5"); ok {
		t.Fatal("unexpected fetched")
	}
	if err := s.RecordFetched(ctx, src.ID, FetchedMessage{MsgKey: "1:5", EMLPath: "x.eml", SHA256: "h"}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.IsFetched(ctx, src.ID, "1:5"); !ok {
		t.Fatal("expected fetched")
	}

	if sha, _ := s.FileSHA(ctx, "x.eml"); sha != "" {
		t.Fatal("unexpected sha")
	}
	_ = s.MarkFile(ctx, ProcessedFile{Path: "x.eml", SHA256: "h", Status: StatusOK, ReportsFound: 1})
	if sha, _ := s.FileSHA(ctx, "x.eml"); sha != "h" {
		t.Fatalf("sha = %q", sha)
	}
}

func TestOpenFileReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sub", "dmarc.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertReport(ctx, mustParse(t, tu.Sample("G", "1", "example.com", day)), "x", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	st, _ := s.Stats(ctx)
	if st.Reports != 1 {
		t.Errorf("reports after reopen = %d", st.Reports)
	}
}
