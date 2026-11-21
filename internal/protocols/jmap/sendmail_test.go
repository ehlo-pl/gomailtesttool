package jmap

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ehlo-pl/gomailtesttool/internal/jmap/protocol"
)

func mailbox(id, role string) protocol.Mailbox {
	mb := protocol.Mailbox{Id: protocol.Id(id), Name: id}
	if role != "" {
		mb.Role = &role
	}
	return mb
}

func TestSelectSendMailboxes(t *testing.T) {
	full := []protocol.Mailbox{mailbox("in", "inbox"), mailbox("dr", "drafts"), mailbox("se", "sent")}

	tests := []struct {
		name       string
		mailboxes  []protocol.Mailbox
		saveToSent bool
		wantCreate protocol.Id
		wantSent   protocol.Id
		wantErr    bool
	}{
		{name: "default creates in drafts, no sent copy", mailboxes: full, wantCreate: "dr"},
		{name: "save-to-sent creates in drafts, moves to sent", mailboxes: full, saveToSent: true, wantCreate: "dr", wantSent: "se"},
		{name: "no drafts falls back to sent", mailboxes: []protocol.Mailbox{mailbox("se", "sent")}, wantCreate: "se"},
		{name: "save-to-sent without sent mailbox", mailboxes: []protocol.Mailbox{mailbox("dr", "drafts")}, saveToSent: true, wantCreate: "dr"},
		{name: "neither drafts nor sent", mailboxes: []protocol.Mailbox{mailbox("in", "inbox")}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ids, sent, err := selectSendMailboxes(tt.mailboxes, tt.saveToSent)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("selectSendMailboxes() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("selectSendMailboxes() error = %v", err)
			}
			if len(ids) != 1 || !ids[tt.wantCreate] {
				t.Errorf("mailboxIds = %v, want {%s: true}", ids, tt.wantCreate)
			}
			if sent != tt.wantSent {
				t.Errorf("sentMailboxId = %q, want %q", sent, tt.wantSent)
			}
		})
	}
}

func TestCheckSendResponse(t *testing.T) {
	tests := []struct {
		name          string
		createMethod  string
		emailCreateId string
		body          string
		wantErr       string // substring; empty means success
	}{
		{
			name:          "success with implicit cleanup response",
			createMethod:  protocol.MethodEmailSet,
			emailCreateId: protocol.SendDraftCreationId,
			body: `{"methodResponses":[
				["Email/set",{"created":{"draft":{"id":"e1"}}},"c1"],
				["EmailSubmission/set",{"created":{"submission1":{"id":"s1"}}},"c2"],
				["Email/set",{"destroyed":["e1"]},"c2"]]}`,
		},
		{
			// Stalwart: an empty mailboxIds makes c1 fail, and c2 then reports an
			// invalidResultReference. The c1 cause must be surfaced instead.
			name:          "create failure reported before dependent submission error",
			createMethod:  protocol.MethodEmailSet,
			emailCreateId: protocol.SendDraftCreationId,
			body: `{"methodResponses":[
				["Email/set",{"notCreated":{"draft":{"type":"invalidProperties","properties":["mailboxIds"],"description":"Message has to belong to at least one mailbox."}}},"c1"],
				["error",{"type":"invalidResultReference","description":"Invalid reference to non-existing object"},"c2"]]}`,
			wantErr: "Email/set did not create the message: invalidProperties (mailboxIds): Message has to belong",
		},
		{
			name:          "rejected submission is not success",
			createMethod:  protocol.MethodEmailSet,
			emailCreateId: protocol.SendDraftCreationId,
			body: `{"methodResponses":[
				["Email/set",{"created":{"draft":{"id":"e1"}}},"c1"],
				["EmailSubmission/set",{"notCreated":{"submission1":{"type":"forbiddenFrom","description":"not allowed"}}},"c2"]]}`,
			wantErr: "EmailSubmission/set did not submit the message: forbiddenFrom: not allowed",
		},
		{
			name:          "method error",
			createMethod:  protocol.MethodEmailSet,
			emailCreateId: protocol.SendDraftCreationId,
			body:          `{"methodResponses":[["error",{"type":"accountNotFound"},"c1"]]}`,
			wantErr:       "JMAP error in c1",
		},
		{
			name:          "cleanup failure after send is ignored",
			createMethod:  protocol.MethodEmailSet,
			emailCreateId: protocol.SendDraftCreationId,
			body: `{"methodResponses":[
				["Email/set",{"created":{"draft":{"id":"e1"}}},"c1"],
				["EmailSubmission/set",{"created":{"submission1":{"id":"s1"}}},"c2"],
				["Email/set",{"notDestroyed":{"e1":{"type":"forbidden"}}},"c2"]]}`,
		},
		{
			name:          "import failure",
			createMethod:  protocol.MethodEmailImport,
			emailCreateId: protocol.SendImportCreationId,
			body:          `{"methodResponses":[["Email/import",{"notCreated":{"imported":{"type":"invalidProperties","properties":["mailboxIds"]}}},"c1"]]}`,
			wantErr:       "Email/import did not create the message: invalidProperties (mailboxIds)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp protocol.Response
			if err := json.Unmarshal([]byte(tt.body), &resp); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}
			err := checkSendResponse(&resp, tt.createMethod, tt.emailCreateId)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("checkSendResponse() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("checkSendResponse() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestSelectIdentity(t *testing.T) {
	ids := []protocol.Identity{
		{Id: "a", Email: "alice@example.com"},
		{Id: "w", Email: "*@example.org"},
	}
	tests := []struct {
		name       string
		identities []protocol.Identity
		from       string
		wantId     protocol.Id
		wantErr    bool
	}{
		{name: "exact match case-insensitive", identities: ids, from: "Alice@Example.com", wantId: "a"},
		{name: "wildcard domain match", identities: ids, from: "bob@example.org", wantId: "w"},
		{name: "single identity fallback", identities: ids[:1], from: "other@example.net", wantId: "a"},
		{name: "no match among several", identities: ids, from: "other@example.net", wantErr: true},
		{name: "no identities", identities: nil, from: "alice@example.com", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectIdentity(tt.identities, tt.from)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("selectIdentity() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("selectIdentity() error = %v", err)
			}
			if got.Id != tt.wantId {
				t.Errorf("selectIdentity() = %q, want %q", got.Id, tt.wantId)
			}
		})
	}
}
