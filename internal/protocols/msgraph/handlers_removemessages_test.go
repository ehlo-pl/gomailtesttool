//go:build !integration
// +build !integration

package msgraph

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/microsoft/kiota-abstractions-go/authentication"
	nethttplibrary "github.com/microsoft/kiota-http-go"
	kiotajson "github.com/microsoft/kiota-serialization-json-go"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
)

func TestRemoveMessages_PreviewAndDeleteModes(t *testing.T) {
	tests := []struct {
		name          string
		confirmDelete bool
		permanent     bool
		input         string
		wantMethods   []string
		wantPrompts   int
	}{
		{
			name:        "asks for each message",
			input:       "y\nn\n",
			wantMethods: []string{"DELETE"},
			wantPrompts: 2,
		},
		{
			name:          "batch confirmation can cancel all",
			confirmDelete: true,
			input:         "n\n",
			wantPrompts:   1,
		},
		{
			name:          "batch confirmation permanently deletes",
			confirmDelete: true,
			permanent:     true,
			input:         "yes\n",
			wantMethods:   []string{"POST", "POST"},
			wantPrompts:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var deleteMethods []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{
						"value": []map[string]string{
							{"id": "message-1", "subject": "Invoice one"},
							{"id": "message-2", "subject": "Invoice two"},
						},
					})
					return
				}
				deleteMethods = append(deleteMethods, r.Method)
				if tt.permanent && !strings.HasSuffix(r.URL.Path, "/permanentDelete") {
					t.Errorf("permanent deletion path = %q, want /permanentDelete", r.URL.Path)
				}
				w.WriteHeader(http.StatusNoContent)
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
			config.MaxRetries = 0
			config.RetryDelay = time.Millisecond
			config.Subject = "Invoice"
			config.ConfirmDelete = tt.confirmDelete
			config.Permanent = tt.permanent

			var preview, result strings.Builder
			err = removeMessages(context.Background(), client, "user@example.com", "", "Invoice", "", 25, config, nil, strings.NewReader(tt.input), &preview, &result)
			if err != nil {
				t.Fatalf("removeMessages() error = %v", err)
			}

			if len(deleteMethods) != len(tt.wantMethods) {
				t.Fatalf("delete requests = %v, want %v", deleteMethods, tt.wantMethods)
			}
			for i, method := range tt.wantMethods {
				if deleteMethods[i] != method {
					t.Errorf("delete request %d method = %q, want %q", i, deleteMethods[i], method)
				}
			}
			if got := strings.Count(preview.String(), "Remove "); got != tt.wantPrompts {
				t.Errorf("confirmation prompts = %d, want %d\noutput=%q", got, tt.wantPrompts, preview.String())
			}
			if !strings.Contains(preview.String(), "Invoice one") || !strings.Contains(preview.String(), "Invoice two") {
				t.Errorf("preview omitted matching messages: %q", preview.String())
			}
		})
	}
}
