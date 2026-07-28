//go:build !integration
// +build !integration

package ews

import (
	"fmt"
	"strings"
	"testing"
)

func TestMeetingResponseElement(t *testing.T) {
	tests := []struct {
		response string
		want     string
		wantErr  bool
	}{
		{"accept", "AcceptItem", false},
		{"decline", "DeclineItem", false},
		{"tentative", "TentativelyAcceptItem", false},
		{"ACCEPT", "AcceptItem", false},
		{"Decline", "DeclineItem", false},
		{"", "", true},
		{"maybe", "", true},
		{"reject", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.response, func(t *testing.T) {
			got, err := meetingResponseElement(tt.response)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for response %q, got nil", tt.response)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateRespondMeeting(t *testing.T) {
	base := func() *Config {
		c := NewConfig()
		c.Host = "exchange.example.com"
		c.Username = "user@example.com"
		c.Password = "pass"
		c.Action = ActionRespondMeeting
		c.ItemID = "AAMkAGI2THVSAAA="
		c.MeetingResponse = "accept"
		return c
	}

	tests := []struct {
		name     string
		mutate   func(*Config)
		errorMsg string
	}{
		{
			name:     "valid accept",
			mutate:   func(c *Config) {},
			errorMsg: "",
		},
		{
			name:     "valid decline",
			mutate:   func(c *Config) { c.MeetingResponse = "decline" },
			errorMsg: "",
		},
		{
			name:     "valid tentative",
			mutate:   func(c *Config) { c.MeetingResponse = "tentative" },
			errorMsg: "",
		},
		{
			name:     "missing item-id",
			mutate:   func(c *Config) { c.ItemID = "" },
			errorMsg: "respondmeeting requires --item-id",
		},
		{
			name:     "invalid response",
			mutate:   func(c *Config) { c.MeetingResponse = "maybe" },
			errorMsg: "respondmeeting --response must be one of",
		},
		{
			name:     "empty response",
			mutate:   func(c *Config) { c.MeetingResponse = "" },
			errorMsg: "respondmeeting --response must be one of",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base()
			tt.mutate(cfg)
			err := validateConfiguration(cfg)
			if tt.errorMsg == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.errorMsg)
			}
			if got := err.Error(); !strings.Contains(got, tt.errorMsg) {
				t.Errorf("error %q does not contain %q", got, tt.errorMsg)
			}
		})
	}
}

func buildRespondMeetingSOAP(t *testing.T, response, itemID, changeKey, comment string, send bool) string {
	t.Helper()
	elementName, err := meetingResponseElement(response)
	if err != nil {
		t.Fatalf("meetingResponseElement: %v", err)
	}
	disposition := "SendAndSaveCopy"
	if !send {
		disposition = "SaveOnly"
	}
	refItem := fmt.Sprintf(`<t:ReferenceItemId Id="%s"/>`, xmlEscape(itemID))
	if changeKey != "" {
		refItem = fmt.Sprintf(`<t:ReferenceItemId Id="%s" ChangeKey="%s"/>`, xmlEscape(itemID), xmlEscape(changeKey))
	}
	bodyXML := ""
	if comment != "" {
		bodyXML = fmt.Sprintf("\n          <t:Body BodyType=\"Text\">%s</t:Body>", xmlEscape(comment))
	}
	return fmt.Sprintf(respondMeetingSOAPBodyFmt, disposition, elementName, refItem, bodyXML, elementName)
}

func TestRespondMeetingSOAPBody(t *testing.T) {
	tests := []struct {
		name      string
		response  string
		itemID    string
		changeKey string
		comment   string
		send      bool
		contains  []string
		absent    []string
	}{
		{
			name:     "accept SendAndSaveCopy",
			response: "accept",
			itemID:   "ITEM001",
			send:     true,
			contains: []string{
				`MessageDisposition="SendAndSaveCopy"`,
				`<t:AcceptItem>`,
				`Id="ITEM001"`,
				`</t:AcceptItem>`,
			},
			absent: []string{"SaveOnly", "DeclineItem", "ChangeKey"},
		},
		{
			name:     "decline SaveOnly",
			response: "decline",
			itemID:   "ITEM002",
			send:     false,
			contains: []string{
				`MessageDisposition="SaveOnly"`,
				`<t:DeclineItem>`,
				`Id="ITEM002"`,
			},
			absent: []string{"SendAndSaveCopy", "AcceptItem"},
		},
		{
			name:      "tentative with ChangeKey and comment",
			response:  "tentative",
			itemID:    "ITEM003",
			changeKey: "CK==",
			comment:   "I might attend",
			send:      true,
			contains: []string{
				`<t:TentativelyAcceptItem>`,
				`Id="ITEM003"`,
				`ChangeKey="CK=="`,
				`<t:Body BodyType="Text">I might attend</t:Body>`,
			},
			absent: []string{"<t:AcceptItem>", "<t:DeclineItem>"},
		},
		{
			name:     "XML escaping in item ID",
			response: "accept",
			itemID:   `A&B<C>`,
			send:     true,
			contains: []string{`Id="A&amp;B&lt;C&gt;"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			soap := buildRespondMeetingSOAP(t, tt.response, tt.itemID, tt.changeKey, tt.comment, tt.send)
			for _, want := range tt.contains {
				if !strings.Contains(soap, want) {
					t.Errorf("SOAP body missing %q\nGot:\n%s", want, soap)
				}
			}
			for _, notWant := range tt.absent {
				if strings.Contains(soap, notWant) {
					t.Errorf("SOAP body unexpectedly contains %q\nGot:\n%s", notWant, soap)
				}
			}
		})
	}
}
