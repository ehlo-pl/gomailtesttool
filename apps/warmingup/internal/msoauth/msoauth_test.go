//go:build !integration

package msoauth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestLoginAndRefresh(t *testing.T) {
	refreshes := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/devicecode", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"device_code":"d","user_code":"ABCD","verification_uri":"https://example.test/link","expires_in":600,"interval":1}`)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.FormValue("grant_type") == "refresh_token" {
			refreshes++
			_, _ = io.WriteString(w, `{"access_token":"a2","refresh_token":"r2","token_type":"Bearer","expires_in":3600}`)
			return
		}
		// Expires at once, so the next use must refresh.
		_, _ = io.WriteString(w, `{"access_token":"a1","refresh_token":"r1","token_type":"Bearer","expires_in":1}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	a, err := New(t.TempDir(), "outlook1", "client", "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	a.Endpoint.DeviceAuthURL, a.Endpoint.TokenURL = srv.URL+"/devicecode", srv.URL+"/token"

	ctx := context.Background()
	if _, err := a.AccessToken(ctx); err == nil || !strings.Contains(err.Error(), "warmingup auth") {
		t.Fatalf("AccessToken() before login error = %v", err)
	}
	var out strings.Builder
	if err := a.Login(ctx, &out); err != nil || !strings.Contains(out.String(), "ABCD") {
		t.Fatalf("Login() = %v, output %q", err, out.String())
	}
	for range 2 {
		tok, err := a.AccessToken(ctx)
		if err != nil || tok != "a2" {
			t.Fatalf("AccessToken() = %q, %v; want a2", tok, err)
		}
	}
	if refreshes != 1 {
		t.Errorf("refreshes = %d, want 1 (the new token must be stored)", refreshes)
	}
	data, _ := os.ReadFile(a.TokenFile)
	if !strings.Contains(string(data), "r2") {
		t.Error("rotated refresh token not saved")
	}
	if _, err := New("d", "../x", "client", ""); err == nil {
		t.Error("New accepted a path as endpoint name")
	}
}
