package msgraph

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/microsoft/kiota-abstractions-go/authentication"
	nethttplibrary "github.com/microsoft/kiota-http-go"
	kiotajson "github.com/microsoft/kiota-serialization-json-go"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
)

func TestBuildAvailabilityView(t *testing.T) {
	start := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	events := []eventSlot{
		{start: start.Add(30 * time.Minute), end: start.Add(90 * time.Minute), status: models.BUSY_FREEBUSYSTATUS},
		{start: start.Add(time.Hour), end: start.Add(2 * time.Hour), status: models.TENTATIVE_FREEBUSYSTATUS},
		{start: start.Add(3 * time.Hour), end: start.Add(4 * time.Hour), status: models.OOF_FREEBUSYSTATUS},
	}
	got := buildAvailabilityView(events, start, start.Add(5*time.Hour), time.Hour)
	if got != "22030" {
		t.Errorf("got %q, want 22030", got)
	}
}

func TestFindTimeSlot_GetMethodUsesCalendarView(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"value": []map[string]any{
				{
					"start":  map[string]string{"dateTime": "2026-01-05T08:30:00", "timeZone": "UTC"},
					"end":    map[string]string{"dateTime": "2026-01-05T09:00:00", "timeZone": "UTC"},
					"showAs": "busy",
				},
			},
		})
	}))
	defer server.Close()

	authProvider := &authentication.AnonymousAuthenticationProvider{}
	adapter, err := nethttplibrary.NewNetHttpRequestAdapterWithParseNodeFactoryAndSerializationWriterFactoryAndHttpClient(authProvider, kiotajson.NewJsonParseNodeFactory(), nil, server.Client())
	if err != nil {
		t.Fatalf("NewNetHttpRequestAdapter() error = %v", err)
	}
	adapter.SetBaseUrl(server.URL + "/v1.0")
	client := msgraphsdk.NewGraphServiceClient(adapter)

	config := NewConfig()
	config.Method = MethodGet
	config.MaxRetries = 0
	config.StartTime = "2026-01-05T08:00:00Z"
	config.EndTime = "2026-01-05T09:00:00Z"
	config.Duration = 30
	config.Count = 1
	config.OutputFormat = "json"
	if err := findTimeSlot(context.Background(), client, "user@example.com", "colleague@example.com", config, nil); err != nil {
		t.Fatalf("findTimeSlot() error = %v", err)
	}

	if gotMethod != http.MethodGet {
		t.Errorf("request method = %q, want %q", gotMethod, http.MethodGet)
	}
	if gotPath != "/v1.0/users/colleague@example.com/calendarView" {
		t.Errorf("request path = %q, want calendarView for the recipient", gotPath)
	}
}
