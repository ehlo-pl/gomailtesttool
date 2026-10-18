package jmap

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ehlo-pl/gomailtesttool/internal/jmap/protocol"
)

func TestDeleteMessagesCommandAndExportFlag(t *testing.T) {
	root := NewCmd()
	deleteCmd, _, err := root.Find([]string{"deletemessages"})
	if err != nil || deleteCmd.Name() != "deletemessages" {
		t.Fatalf("deletemessages command = %v, err = %v", deleteCmd, err)
	}
	exportCmd, _, err := root.Find([]string{"exportmessages"})
	if err != nil {
		t.Fatal(err)
	}
	if exportCmd.Flags().Lookup("delete") == nil {
		t.Fatal("exportmessages is missing --delete")
	}
}

func TestDeleteEmailsSendsEmailSetDestroy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"Email/set"`) || !strings.Contains(string(body), `"destroy":["email-1"]`) {
			t.Errorf("unexpected Email/set request: %s", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"methodResponses": [][]any{{"Email/set", map[string]any{"destroyed": []string{"email-1"}}, "delete"}},
		})
	}))
	defer server.Close()

	client := NewJMAPClient(NewConfig())
	client.session = &protocol.Session{APIURL: server.URL}
	if err := client.DeleteEmails(context.Background(), "account-1", []protocol.Id{"email-1"}); err != nil {
		t.Fatalf("DeleteEmails() error = %v", err)
	}
}
