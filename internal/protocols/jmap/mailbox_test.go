//go:build !integration

package jmap

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	mb "github.com/ehlo-pl/gomailtesttool/internal/common/mailbox"
)

func TestMailboxSession(t *testing.T) {
	var srv *httptest.Server
	var updates []string
	inJunk := true
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/.well-known/jmap":
			_, _ = io.WriteString(w, `{"capabilities":{"urn:ietf:params:jmap:core":{},"urn:ietf:params:jmap:mail":{}},`+
				`"accounts":{"A":{"name":"a","isPersonal":true,"isReadOnly":false,"accountCapabilities":{}}},`+
				`"primaryAccounts":{"urn:ietf:params:jmap:mail":"A"},"username":"u",`+
				`"apiUrl":"`+srv.URL+`/api","downloadUrl":"`+srv.URL+`/dl/{accountId}/{blobId}/{name}?type={type}",`+
				`"uploadUrl":"`+srv.URL+`/up","eventSourceUrl":"`+srv.URL+`/ev","state":"1"}`)
		case strings.HasPrefix(r.URL.Path, "/dl/"):
			_, _ = io.WriteString(w, "Authentication-Results: mx; spf=pass\r\n\r\nbody")
		case r.URL.Path == "/api":
			body, _ := io.ReadAll(r.Body)
			var req struct {
				MethodCalls [][]json.RawMessage `json:"methodCalls"`
			}
			if err := json.Unmarshal(body, &req); err != nil {
				t.Errorf("request: %v", err)
				return
			}
			var method, call string
			_ = json.Unmarshal(req.MethodCalls[0][0], &method)
			_ = json.Unmarshal(req.MethodCalls[0][2], &call)
			box := "I"
			if inJunk {
				box = "J"
			}
			var result string
			switch method {
			case "Mailbox/get":
				result = `{"accountId":"A","state":"1","list":[{"id":"I","name":"Inbox","role":"inbox"},{"id":"J","name":"Spam","role":"junk"}],"notFound":[]}`
			case "Email/query":
				result = `{"accountId":"A","ids":["E1"]}`
			case "Email/get":
				result = `{"accountId":"A","state":"1","list":[{"id":"E1","blobId":"B1","messageId":["x@example.com"],` +
					`"mailboxIds":{"` + box + `":true},"receivedAt":"2026-12-07T10:00:00Z"}],"notFound":[]}`
			case "Email/set":
				updates = append(updates, string(req.MethodCalls[0][1]))
				if strings.Contains(string(req.MethodCalls[0][1]), "mailboxIds") {
					inJunk = false
				}
				result = `{"accountId":"A","updated":{"E1":null}}`
			}
			_, _ = io.WriteString(w, `{"methodResponses":[["`+method+`",`+result+`,`+strconv.Quote(call)+`]],"sessionState":"1"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := NewConfig()
	cfg.Host, cfg.AccessToken, cfg.SkipVerify = srv.URL, "tok", true
	ctx := context.Background()
	s, err := OpenMailbox(ctx, cfg)
	if err != nil {
		t.Fatalf("OpenMailbox: %v", err)
	}
	m, err := s.Locate(ctx, "x@example.com")
	if err != nil || m == nil || m.Placement != mb.Junk || m.Folder != "Spam" || strings.Contains(string(m.Header), "body") {
		t.Fatalf("Locate() = %+v, %v", m, err)
	}
	if m, err := s.Locate(ctx, "other@example.com"); m != nil || err != nil {
		t.Errorf("a substring match must not count: %+v, %v", m, err)
	}
	if err := s.MarkRead(ctx, "x@example.com"); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if err := s.Rescue(ctx, "x@example.com"); err != nil {
		t.Fatalf("Rescue: %v", err)
	}
	if err := s.Rescue(ctx, "x@example.com"); err != nil { // already in the inbox
		t.Fatalf("second Rescue: %v", err)
	}
	if len(updates) != 2 || !strings.Contains(updates[0], "keywords/$seen") || !strings.Contains(updates[1], `"mailboxIds":{"I":true}`) {
		t.Errorf("updates = %v", updates)
	}
}
