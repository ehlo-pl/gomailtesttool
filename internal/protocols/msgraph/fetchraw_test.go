//go:build !integration

package msgraph

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/microsoft/kiota-abstractions-go/authentication"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"

	"github.com/ehlo-pl/gomailtesttool/internal/common/fetch"
)

func TestBuildDateFilter(t *testing.T) {
	since := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	before := time.Date(2026, 11, 1, 0, 0, 0, 0, time.FixedZone("CET", 3600))
	tests := []struct {
		since, before time.Time
		want          string
	}{
		{time.Time{}, time.Time{}, ""},
		{since, time.Time{}, "receivedDateTime ge 2026-10-01T00:00:00Z"},
		{since, before, "receivedDateTime ge 2026-10-01T00:00:00Z and receivedDateTime lt 2026-10-31T23:00:00Z"},
	}
	for _, tt := range tests {
		if got := buildDateFilter(tt.since, tt.before); got != tt.want {
			t.Errorf("buildDateFilter = %q, want %q", got, tt.want)
		}
	}
}

func TestFetchSessionGraphPaging(t *testing.T) {
	var mu sync.Mutex
	var listQueries []string
	var deleted []string
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		path := r.URL.Path
		switch {
		case r.Method == http.MethodDelete:
			deleted = append(deleted, path[strings.LastIndex(path, "/")+1:])
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(path, "/$value"):
			id := strings.TrimSuffix(path, "/$value")
			id = id[strings.LastIndex(id, "/")+1:]
			w.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(w, "Subject: "+id+"\r\n\r\nbody")
		case strings.HasSuffix(path, "/mailFolders/dmarc/messages"):
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("page") == "2" {
				_ = json.NewEncoder(w).Encode(map[string]any{"value": []map[string]any{
					{"id": "m3", "subject": "Report domain: example.com", "receivedDateTime": "2026-11-02T10:00:00Z"},
				}})
				return
			}
			listQueries = append(listQueries, r.URL.RawQuery)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"value": []map[string]any{
					{"id": "m1", "subject": "Report domain: example.com", "receivedDateTime": "2026-11-01T10:00:00Z"},
					{"id": "m2", "subject": "Out of office", "receivedDateTime": "2026-11-01T11:00:00Z"},
				},
				"@odata.nextLink": srv.URL + "/v1.0/users/dmarc@example.com/mailFolders/dmarc/messages?page=2",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	adapter, err := msgraphsdk.NewGraphRequestAdapter(&authentication.AnonymousAuthenticationProvider{})
	if err != nil {
		t.Fatal(err)
	}
	adapter.SetBaseUrl(srv.URL + "/v1.0")
	cfg := NewConfig()
	cfg.MaxRetries = 0
	s := &fetchSession{client: msgraphsdk.NewGraphServiceClient(adapter), config: cfg,
		mailbox: "dmarc@example.com", log: slog.New(slog.NewTextHandler(io.Discard, nil))}

	var got []string
	err = s.Fetch(context.Background(), fetch.Criteria{
		Folder: "dmarc", Subject: "report domain", Since: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}, func(_ context.Context, m fetch.RawMessage) error {
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
	if len(listQueries) != 1 || !strings.Contains(listQueries[0], "receivedDateTime+ge+2026-10-01T00") && !strings.Contains(listQueries[0], "receivedDateTime%20ge%202026-10-01T00") {
		t.Errorf("list query = %v", listQueries)
	}
	if err := s.Delete(context.Background(), "m1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(deleted) != 1 || deleted[0] != "m1" {
		t.Errorf("deleted = %v", deleted)
	}
}
