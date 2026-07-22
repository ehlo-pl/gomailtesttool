package ews

import (
	"fmt"
	"strings"
	"testing"
)

// TestCreateItemSOAPDispositions verifies each CreateItem body template carries
// the correct MessageDisposition, and that the draft template saves into the
// Drafts folder without sending.
func TestCreateItemSOAPDispositions(t *testing.T) {
	tests := []struct {
		name            string
		tpl             string
		wantDisposition string
		wantFolder      string // "" means no SavedItemFolderId expected
	}{
		{"send only", createItemSOAPBodySendOnlyFmt, `MessageDisposition="SendOnly"`, ""},
		{"save to sent", createItemSOAPBodySaveToSentFmt, `MessageDisposition="SendAndSaveCopy"`, `Id="sentitems"`},
		{"draft", createItemSOAPBodySaveOnlyDraftFmt, `MessageDisposition="SaveOnly"`, `Id="drafts"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := fmt.Sprintf(tt.tpl, "Subj", "Text", "hello",
				"", buildRecipientsXML([]string{"a@example.com"}), "")

			if !strings.Contains(body, tt.wantDisposition) {
				t.Errorf("body missing disposition %q:\n%s", tt.wantDisposition, body)
			}
			if tt.wantFolder != "" && !strings.Contains(body, tt.wantFolder) {
				t.Errorf("body missing saved-folder marker %q:\n%s", tt.wantFolder, body)
			}
			if tt.wantFolder == "" && strings.Contains(body, "SavedItemFolderId") {
				t.Errorf("send-only body unexpectedly sets SavedItemFolderId:\n%s", body)
			}
			// The draft template must never send.
			if tt.name == "draft" && strings.Contains(body, "Send") {
				t.Errorf("draft body must not contain a Send disposition:\n%s", body)
			}
		})
	}
}
