package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	gmailapi "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

func TestExportMessages_JSONOutputIncludesExportedFiles(t *testing.T) {
	t.Helper()

	const messageID = "abc123"
	rawMessage := "Subject: Loop Test\r\n\r\nhello"
	rawEncoded := base64.RawURLEncoding.EncodeToString([]byte(rawMessage))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gmail/v1/users/me/messages":
			if got := r.URL.Query().Get("q"); got != "subject:(Loop Test)" {
				http.Error(w, "unexpected query: "+got, http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"messages": []map[string]string{{"id": messageID}},
			})
		case "/gmail/v1/users/me/messages/" + messageID:
			if got := r.URL.Query().Get("format"); got != "raw" {
				http.Error(w, "unexpected format: "+got, http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":  messageID,
				"raw": rawEncoded,
			})
		default:
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer srv.Close()

	svc, err := gmailapi.NewService(context.Background(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	cfg := &Config{
		Subject:      "Loop Test",
		Count:        1,
		OutputFormat: "json",
		ExportDir:    t.TempDir(),
		MaxRetries:   0,
		RetryDelay:   1,
	}

	stdout := captureStdout(t, func() {
		if err := exportMessages(context.Background(), svc, cfg, nil); err != nil {
			t.Fatalf("exportMessages() error = %v", err)
		}
	})

	var got []struct {
		ID       string `json:"id"`
		FilePath string `json:"filePath"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v\nstdout=%s", err, stdout)
	}
	if len(got) != 1 {
		t.Fatalf("len(json output) = %d, want 1", len(got))
	}
	if got[0].ID != messageID {
		t.Fatalf("output id = %q, want %q", got[0].ID, messageID)
	}

	wantPath := filepath.Join(cfg.ExportDir, "export")
	if got[0].FilePath == "" || filepath.Dir(filepath.Dir(got[0].FilePath)) != wantPath {
		t.Fatalf("unexpected file path %q", got[0].FilePath)
	}

	data, err := os.ReadFile(got[0].FilePath)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", got[0].FilePath, err)
	}
	if string(data) != rawMessage {
		t.Fatalf("exported message = %q, want %q", string(data), rawMessage)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	defer func() { _ = r.Close() }()
	defer func() { os.Stdout = old }()
	os.Stdout = w

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("w.Close() error = %v", err)
	}

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	return string(out)
}
