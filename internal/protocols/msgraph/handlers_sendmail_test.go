//go:build !integration
// +build !integration

package msgraph

import (
	"net/http"
	"testing"
)

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

