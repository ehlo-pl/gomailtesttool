//go:build !integration

package wikicontent

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

const testPoems = `[
 {"title":"","author":"Nobody","lines":["skipped"]},
 {"title":"First","author":"A. Poet","lines":["Roses <red>","  indented"],"linecount":"2"},
 {"title":"Second","author":"B. Poet","lines":["Two"],"linecount":"1"}
]`

func fakePoetry(t *testing.T, response string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/random/10" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = io.WriteString(w, response)
	}))
}

func TestRandomPoem(t *testing.T) {
	srv := fakePoetry(t, testPoems)
	defer srv.Close()
	f := &Fetcher{PoetryURL: srv.URL}

	tests := []struct {
		name       string
		used       map[string]bool
		want       string
		wantReused bool
	}{
		{name: "first complete poem", want: "First by A. Poet"},
		{name: "used one skipped", used: map[string]bool{"First by A. Poet": true}, want: "Second by B. Poet"},
		{name: "all used repeats", used: map[string]bool{"First by A. Poet": true, "Second by B. Poet": true}, want: "First by A. Poet", wantReused: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := f.RandomPoem(context.Background(), tt.used)
			if err != nil {
				t.Fatalf("RandomPoem() error = %v", err)
			}
			if p.Name() != tt.want || p.Reused != tt.wantReused {
				t.Errorf("RandomPoem() = %+v", p)
			}
		})
	}
}

func TestFill_Poetry(t *testing.T) {
	const defBody = "It's a test message, please ignore"
	srv := fakePoetry(t, testPoems)
	defer srv.Close()
	broken := fakePoetry(t, `{"status":404,"reason":"Not found"}`)
	defer broken.Close()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))

	logDir := t.TempDir()
	writeFile(t, filepath.Join(logDir, "_tool_sendmail_2026-11-21.csv"), "Subject\nPoem: First by A. Poet\n")
	history := History{Dir: logDir, Tool: "tool", Action: "sendmail"}

	fill := func(url, subject string) (string, string, string) {
		body, bodyHTML := defBody, ""
		Fill(context.Background(), &Fetcher{PoetryURL: url}, BodySourcePoetry,
			Message{Subject: &subject, Body: &body, BodyHTML: &bodyHTML}, testSubject, history, quiet)
		return subject, body, bodyHTML
	}

	subject, body, _ := fill(srv.URL, testSubject)
	if subject != "Poem: Second by B. Poet" {
		t.Errorf("subject = %q", subject)
	}
	for _, want := range []string{defBody, "Two", "CC BY 4.0", srv.URL} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}

	history.Dir = t.TempDir()
	subject, body, bodyHTML := fill(srv.URL, "Mine")
	if subject != "Mine" || !strings.Contains(body, "Roses <red>\n  indented") {
		t.Errorf("fill() = %q, %q", subject, body)
	}
	if !strings.Contains(bodyHTML, "<pre style=\"white-space:pre-wrap\">Roses &lt;red&gt;\n  indented</pre>") {
		t.Errorf("bodyHTML = %q", bodyHTML)
	}
	if subject, body, bodyHTML := fill(broken.URL, testSubject); subject != testSubject || body != defBody || bodyHTML != "" {
		t.Errorf("failed fetch changed the message: %q %q %q", subject, body, bodyHTML)
	}
}
