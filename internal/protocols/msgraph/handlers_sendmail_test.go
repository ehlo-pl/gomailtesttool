//go:build !integration
// +build !integration

package msgraph

import (
	"net/http"
	"strings"
	"testing"
)

func TestBuildMessage_CustomMessageIDHeader(t *testing.T) {
	tests := []struct {
		name       string
		messageID  string
		suffix     string
		wantHeader string
	}{
		{name: "explicit ID", messageID: "custom@example.com", wantHeader: "<custom@example.com>"},
		{name: "suffix", suffix: "example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewConfig()
			config.MessageID = tt.messageID
			config.MessageIDSuffix = tt.suffix

			message, err := buildMessage(nil, nil, nil, "subject", "body", "", nil, config)
			if err != nil {
				t.Fatalf("buildMessage() error = %v", err)
			}
			headers := message.GetInternetMessageHeaders()
			if len(headers) != 1 {
				t.Fatalf("got %d custom headers, want 1", len(headers))
			}
			if got := headers[0].GetName(); got == nil || *got != "X-Message-ID" {
				t.Fatalf("header name = %v, want X-Message-ID", got)
			}
			value := headers[0].GetValue()
			if value == nil {
				t.Fatal("X-Message-ID value is nil")
			}
			if tt.wantHeader != "" && *value != tt.wantHeader {
				t.Fatalf("X-Message-ID = %q, want %q", *value, tt.wantHeader)
			}
			if tt.suffix != "" && !strings.HasSuffix(*value, "@example.com>") {
				t.Fatalf("X-Message-ID = %q, want example.com suffix", *value)
			}
		})
	}
}

func TestExtractSendMailMessageID(t *testing.T) {
	tests := []struct {
		name    string
		headers http.Header
		want    string
	}{
		{
			name:    "nil headers",
			headers: nil,
			want:    "",
		},
		{
			name: "message-id header",
			headers: http.Header{
				"Message-Id": []string{" <abc123@example.com> "},
			},
			want: "<abc123@example.com>",
		},
		{
			name: "internet-message-id header fallback",
			headers: http.Header{
				"Internet-Message-Id": []string{"<def456@example.com>"},
			},
			want: "<def456@example.com>",
		},
		{
			name: "empty values return empty string",
			headers: http.Header{
				"Message-Id":          []string{"   "},
				"Internet-Message-Id": []string{""},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSendMailMessageID(tt.headers)
			if got != tt.want {
				t.Fatalf("extractSendMailMessageID() = %q, want %q", got, tt.want)
			}
		})
	}
}
