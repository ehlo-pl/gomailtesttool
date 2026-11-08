package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gmailapi "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/ehlo-pl/gomailtesttool/internal/common/fetch"
)

func TestBuildFetchQuery(t *testing.T) {
	since := time.Unix(1790000000, 0)
	tests := []struct {
		c       fetch.Criteria
		want    string
		wantErr bool
	}{
		{fetch.Criteria{}, "", false},
		{fetch.Criteria{Folder: "DMARC Reports", Since: since}, `label:"DMARC Reports" after:1790000000`, false},
		{fetch.Criteria{Subject: "Report domain", Query: "has:attachment"}, `subject:"Report domain" has:attachment`, false},
		{fetch.Criteria{Folder: `x" OR in:anywhere`}, "", true},
		{fetch.Criteria{Subject: "a\"b"}, "", true},
	}
	for _, tt := range tests {
		got, err := buildFetchQuery(tt.c)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("buildFetchQuery(%+v) = %q, %v; want %q", tt.c, got, err, tt.want)
		}
	}
}

func TestFetchSessionGmailPaging(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/messages") && r.URL.Query().Get("pageToken") == "":
			queries = append(queries, r.URL.Query().Get("q"))
			_ = json.NewEncoder(w).Encode(map[string]any{
				"messages":      []map[string]string{{"id": "m1"}, {"id": "m2"}},
				"nextPageToken": "p2",
			})
		case strings.HasSuffix(r.URL.Path, "/messages"):
			_ = json.NewEncoder(w).Encode(map[string]any{"messages": []map[string]string{{"id": "m3"}}})
		case strings.Contains(r.URL.Path, "/messages/"):
			id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			raw := base64.URLEncoding.EncodeToString([]byte("Subject: " + id + "\r\n\r\nbody"))
			_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "raw": raw, "internalDate": "1793534400000"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc, err := gmailapi.NewService(context.Background(), option.WithHTTPClient(srv.Client()), option.WithEndpoint(srv.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	s := &fetchSession{svc: svc, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	var got []string
	err = s.Fetch(context.Background(), fetch.Criteria{Folder: "DMARC", SkipKey: func(k string) bool { return k == "m2" }},
		func(_ context.Context, m fetch.RawMessage) error {
			got = append(got, m.Key)
			if !strings.HasPrefix(string(m.Raw), "Subject: "+m.Key) || m.Received.IsZero() {
				t.Errorf("bad message %+v", m)
			}
			return nil
		})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if strings.Join(got, ",") != "m1,m3" {
		t.Errorf("fetched = %v, want [m1 m3]", got)
	}
	if len(queries) != 1 || queries[0] != `label:"DMARC"` {
		t.Errorf("queries = %v", queries)
	}
}
