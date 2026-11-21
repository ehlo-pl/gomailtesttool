//go:build !integration

package wikicontent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// fakeWiki serves a minimal MediaWiki API. categories maps a category to its
// members (title -> namespace); "Category:Broken" answers HTTP 500. Every
// article exists.
func fakeWiki(t *testing.T, categories map[string]map[string]int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "gomailtest/") {
			t.Errorf("User-Agent = %q, want gomailtest/...", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "application/json")
		if q.Get("list") == "categorymembers" {
			if q.Get("cmtitle") == "Category:Broken" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			list := []map[string]any{}
			for title, ns := range categories[q.Get("cmtitle")] {
				list = append(list, map[string]any{"ns": ns, "title": title})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"query": map[string]any{"categorymembers": list}})
			return
		}
		title := q.Get("titles")
		if strings.HasPrefix(title, "Talk:") {
			t.Errorf("article fetched with talk prefix: %q", title)
		}
		extract := "<p>HTML of " + title + "</p>"
		if q.Get("explaintext") == "1" {
			extract = "Text of " + title
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"query": map[string]any{"pages": []map[string]any{{
			"title": title, "fullurl": "https://example.test/wiki/" + title, "extract": extract,
		}}}})
	}))
}

func newTestFetcher(url string, categories ...string) *Fetcher {
	return &Fetcher{APIURL: url, Categories: categories, Client: http.DefaultClient}
}

func TestRandom_StripsTalkPrefix(t *testing.T) {
	srv := fakeWiki(t, map[string]map[string]int{"Category:A": {"Talk:Caesar cipher": 1}})
	defer srv.Close()

	a, err := newTestFetcher(srv.URL, "Category:A").Random(context.Background(), nil)
	if err != nil {
		t.Fatalf("Random() error = %v", err)
	}
	if a.Title != "Caesar cipher" || a.Text != "Text of Caesar cipher" || a.HTML != "<p>HTML of Caesar cipher</p>" {
		t.Errorf("Random() = %+v", a)
	}
	if a.URL != "https://example.test/wiki/Caesar cipher" || a.Category != "Category:A" || a.Reused {
		t.Errorf("Random() = %+v", a)
	}
}

func TestRandom_SkipsUsedArticles(t *testing.T) {
	srv := fakeWiki(t, map[string]map[string]int{
		"Category:A": {"Talk:One": 1, "Talk:Two": 1},
		"Category:B": {"Three": 0},
	})
	defer srv.Close()

	tests := []struct {
		name       string
		categories []string
		used       map[string]bool
		want       []string // acceptable titles
		wantReused bool
	}{
		{name: "unused article in same category", categories: []string{"Category:A"}, used: map[string]bool{"One": true}, want: []string{"Two"}},
		{name: "moves on to another category", categories: []string{"Category:A", "Category:B"}, used: map[string]bool{"One": true, "Two": true}, want: []string{"Three"}},
		{name: "broken category is skipped", categories: []string{"Category:Broken", "Category:B"}, want: []string{"Three"}},
		{name: "all used repeats one", categories: []string{"Category:A", "Category:B"}, used: map[string]bool{"One": true, "Two": true, "Three": true}, want: []string{"One", "Two", "Three"}, wantReused: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Category order and picks are random: repeat to cover them.
			for range 20 {
				a, err := newTestFetcher(srv.URL, tt.categories...).Random(context.Background(), tt.used)
				if err != nil {
					t.Fatalf("Random() error = %v", err)
				}
				if !slices.Contains(tt.want, a.Title) {
					t.Fatalf("Random() title = %q, want one of %v", a.Title, tt.want)
				}
				if a.Reused != tt.wantReused {
					t.Fatalf("Reused = %v, want %v", a.Reused, tt.wantReused)
				}
			}
		})
	}
}

func TestRandom_Errors(t *testing.T) {
	srv := fakeWiki(t, map[string]map[string]int{"Category:Other": {"Category:Sub": 14, "Template:X": 10}})
	defer srv.Close()

	if _, err := newTestFetcher(srv.URL, "Category:Other").Random(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "no members") {
		t.Errorf("Random() error = %v, want no members", err)
	}
	if _, err := newTestFetcher(srv.URL, "Category:Broken").Random(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("Random() error = %v, want HTTP 500", err)
	}
}

const testSubject = "Automated Tool Notification"

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestHistory_UsedTitles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "_jmaptool_sendmail_2026-11-20.csv"),
		"Action,Status,Server,To,Subject,Error\n"+
			"sendmail,SUCCESS,host,a@b,Wikipedia: Binary search,\n"+
			"sendmail,FAILURE,host,a@b,\"Wikipedia: Comma, quoted\",boom\n"+
			"sendmail,SUCCESS,host,a@b,My own subject,\n"+
			"sendmail,SUCCESS,host,a@b,Automated Tool Notification,\n")
	writeFile(t, filepath.Join(dir, "_jmaptool_sendmail_2026-11-21.jsonl"),
		`{"Action":"sendmail","Subject":"Wikipedia: AT&T","timestamp":"x"}`+"\n"+
			"not json\n")
	// Other actions and tools are not this action's history.
	writeFile(t, filepath.Join(dir, "_jmaptool_listmail_2026-11-21.csv"), "Subject\nWikipedia: Listmail\n")
	writeFile(t, filepath.Join(dir, "_smtptool_sendmail_2026-11-21.csv"), "Subject\nWikipedia: Other tool\n")

	got := History{Dir: dir, Tool: "jmaptool", Action: "sendmail"}.UsedTitles()
	want := map[string]bool{"Binary search": true, "Comma, quoted": true, "AT&T": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("UsedTitles() = %v, want %v", got, want)
	}

	if got := (History{Dir: t.TempDir(), Tool: "jmaptool", Action: "sendmail"}).UsedTitles(); len(got) != 0 {
		t.Errorf("UsedTitles() with no logs = %v, want empty", got)
	}
}

func TestFill(t *testing.T) {
	const defBody = "It's a test message, please ignore"
	srv := fakeWiki(t, map[string]map[string]int{"Category:A": {"Binary search": 0, "Caesar cipher": 0}})
	defer srv.Close()
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer down.Close()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))

	// "Caesar cipher" was already sent, so Fill must pick "Binary search".
	logDir := t.TempDir()
	writeFile(t, filepath.Join(logDir, "_tool_sendmail_2026-11-21.csv"), "Subject\nWikipedia: Caesar cipher\n")
	history := History{Dir: logDir, Tool: "tool", Action: "sendmail"}

	tests := []struct {
		name        string
		url         string
		source      string
		subject     string
		wantSubject string
		wantWiki    bool
	}{
		{name: "default subject gets unused title", url: srv.URL, source: BodySourceWikipedia, subject: testSubject, wantSubject: "Wikipedia: Binary search", wantWiki: true},
		{name: "explicit subject kept", url: srv.URL, source: BodySourceWikipedia, subject: "Mine", wantSubject: "Mine", wantWiki: true},
		{name: "static source untouched", url: srv.URL, source: BodySourceStatic, subject: testSubject, wantSubject: testSubject},
		{name: "fetch failure falls back", url: down.URL, source: BodySourceWikipedia, subject: testSubject, wantSubject: testSubject},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subject, body, bodyHTML := tt.subject, defBody, ""
			msg := Message{Subject: &subject, Body: &body, BodyHTML: &bodyHTML}
			Fill(context.Background(), newTestFetcher(tt.url, "Category:A"), tt.source, msg, testSubject, history, quiet)
			if subject != tt.wantSubject {
				t.Errorf("subject = %q, want %q", subject, tt.wantSubject)
			}
			if !tt.wantWiki {
				if body != defBody || bodyHTML != "" {
					t.Errorf("body changed: %q / %q", body, bodyHTML)
				}
				return
			}
			for _, want := range []string{defBody, "Text of Binary search", "CC BY-SA 4.0", "https://example.test/wiki/Binary search"} {
				if !strings.Contains(body, want) {
					t.Errorf("body missing %q", want)
				}
			}
			if !strings.Contains(bodyHTML, "<p>HTML of Binary search</p>") {
				t.Errorf("bodyHTML = %q", bodyHTML)
			}
		})
	}
}

func TestParseCategories(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{in: "", want: nil},
		{in: " , ", want: nil},
		{in: "FA-Class Computing articles", want: []string{"Category:FA-Class Computing articles"}},
		{in: "Category:A, category:B ,C", want: []string{"Category:A", "category:B", "Category:C"}},
	}
	for _, tt := range tests {
		if got := ParseCategories(tt.in); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ParseCategories(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestNewFetcher_DefaultCategories(t *testing.T) {
	if got := NewFetcher(nil).Categories; !reflect.DeepEqual(got, DefaultCategories) {
		t.Errorf("NewFetcher(nil).Categories = %v, want DefaultCategories", got)
	}
	if got := NewFetcher([]string{"Category:X"}).Categories; !reflect.DeepEqual(got, []string{"Category:X"}) {
		t.Errorf("NewFetcher(X).Categories = %v", got)
	}
}

func TestNoBodyGiven(t *testing.T) {
	tests := []struct {
		name                     string
		body, bodyHTML, template string
		want                     bool
	}{
		{name: "nothing given", body: "def", want: true},
		{name: "explicit body", body: "mine"},
		{name: "html body", body: "def", bodyHTML: "<p>x</p>"},
		{name: "template", body: "def", template: "t.html"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NoBodyGiven(tt.body, "def", tt.bodyHTML, tt.template); got != tt.want {
				t.Errorf("NoBodyGiven() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateBodySource(t *testing.T) {
	for _, ok := range []string{BodySourceWikipedia, BodySourceStatic} {
		if err := ValidateBodySource(ok); err != nil {
			t.Errorf("ValidateBodySource(%q) error = %v", ok, err)
		}
	}
	if err := ValidateBodySource("random"); err == nil {
		t.Error("ValidateBodySource(\"random\") error = nil, want error")
	}
}
