//go:build !integration

package wikicontent

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const testIndex = `                                RFC INDEX

  #### Title of RFC. Author 1. Issue date.
       (Format: ASCII)

1 Host Software. S. Crocker. April 1969. (Format: TXT, HTML) (Status:
  UNKNOWN)

8 ARPA Network Functional Specifications. G. Deloche. May 1969. (Format:
  PDF) (Status: UNKNOWN)

14 Not Issued.

20 A title long enough to wrap the format list. A. Author. May 1969. (Format:
   HTML, TXT) (Status: UNKNOWN)

30 Listed but missing. A. Author. May 1969. (Format: TXT) (Status:
   UNKNOWN)
`

// fakeRFCArchive serves RFC 1 and 20; RFC 30 is listed but answers 404.
func fakeRFCArchive(t *testing.T) *httptest.Server {
	t.Helper()
	files := map[string]string{
		"/rfc-index.txt":  testIndex,
		"/rfc/rfc1.json":  `{"title":"Host Software"}`,
		"/rfc/rfc1.txt":   "\uFEFF\n\nText of <one>\r\n\fsecond page\n",
		"/rfc/rfc20.json": `{"title":"  Wrapped Title "}`,
		"/rfc/rfc20.txt":  "Text of twenty\n",
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, body)
	}))
}

func TestParseRFCIndex(t *testing.T) {
	if got, want := parseRFCIndex(testIndex), []int{1, 20, 30}; !reflect.DeepEqual(got, want) {
		t.Errorf("parseRFCIndex() = %v, want %v", got, want)
	}
}

func TestRandomRFC(t *testing.T) {
	srv := fakeRFCArchive(t)
	defer srv.Close()
	f := &Fetcher{RFCURL: srv.URL}

	// Repeated: picks are random, and the missing RFC 30 must be skipped over.
	for range 20 {
		doc, err := f.RandomRFC(context.Background(), map[int]bool{20: true})
		if err != nil {
			t.Fatalf("RandomRFC() error = %v", err)
		}
		if doc.Number != 1 || doc.Title != "Host Software" || doc.Text != "Text of <one>\nsecond page" || doc.Reused {
			t.Fatalf("RandomRFC() = %+v", doc)
		}
	}
	doc, err := f.RandomRFC(context.Background(), map[int]bool{1: true, 20: true, 30: true})
	if err != nil || !doc.Reused {
		t.Errorf("RandomRFC() with all used = %+v, %v, want Reused", doc, err)
	}
}

func TestFill_RFC(t *testing.T) {
	const defBody = "It's a test message, please ignore"
	srv := fakeRFCArchive(t)
	defer srv.Close()
	down := httptest.NewServer(http.NotFoundHandler())
	defer down.Close()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))

	logDir := t.TempDir()
	writeFile(t, filepath.Join(logDir, "_tool_sendmail_2026-11-21.csv"),
		"Subject\nRFC 20: Wrapped Title\nRFC 30: Missing\nRFC without a number\n")
	history := History{Dir: logDir, Tool: "tool", Action: "sendmail"}
	if got, want := history.UsedRFCs(), map[int]bool{20: true, 30: true}; !reflect.DeepEqual(got, want) {
		t.Fatalf("UsedRFCs() = %v, want %v", got, want)
	}

	fill := func(url, subject string) (string, string, string) {
		body, bodyHTML := defBody, ""
		Fill(context.Background(), &Fetcher{RFCURL: url}, BodySourceRFC,
			Message{Subject: &subject, Body: &body, BodyHTML: &bodyHTML}, testSubject, history, quiet)
		return subject, body, bodyHTML
	}

	subject, body, bodyHTML := fill(srv.URL, testSubject)
	if subject != "RFC 1: Host Software" {
		t.Errorf("subject = %q", subject)
	}
	for _, want := range []string{defBody, "Text of <one>", "BCP 78", srv.URL + "/rfc/rfc1.txt"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
	if !strings.Contains(bodyHTML, "<pre>Text of &lt;one&gt;\nsecond page</pre>") {
		t.Errorf("bodyHTML = %q", bodyHTML)
	}

	if subject, _, _ := fill(srv.URL, "Mine"); subject != "Mine" {
		t.Errorf("explicit subject changed to %q", subject)
	}
	if subject, body, bodyHTML := fill(down.URL, testSubject); subject != testSubject || body != defBody || bodyHTML != "" {
		t.Errorf("failed fetch changed the message: %q %q %q", subject, body, bodyHTML)
	}
}

func TestTruncateText(t *testing.T) {
	if got := truncateText("short", 10, "u"); got != "short" {
		t.Errorf("truncateText() = %q", got)
	}
	if got, want := truncateText("one\ntwo\nthree", 10, "u"), "one\ntwo\n\n[Truncated. Full text: u]"; got != want {
		t.Errorf("truncateText() = %q, want %q", got, want)
	}
}
