//go:build !integration
// +build !integration

package msgraph

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	abstractions "github.com/microsoft/kiota-abstractions-go"
)

func TestBuildMIMERequestFromBytes(t *testing.T) {
	message := []byte("X-TestMailFlow-Id: abc\r\nSubject: [TMF] flow abc\r\n\r\nBody\r\n")
	adapter := &testMIMERequestAdapter{baseURL: "https://graph.microsoft.com/v1.0"}

	request, err := buildMIMERequestFromBytes(adapter, "probe user@example.com", message)
	if err != nil {
		t.Fatalf("buildMIMERequestFromBytes() error = %v", err)
	}
	if request.Method != abstractions.POST {
		t.Errorf("Method = %v, want POST", request.Method)
	}
	if got, want := string(request.Content), base64.StdEncoding.EncodeToString(message); got != want {
		t.Errorf("request body = %q, want %q", got, want)
	}
	requestURL, err := request.GetUri()
	if err != nil {
		t.Fatalf("GetUri() error = %v", err)
	}
	if got, want := requestURL.String(), "https://graph.microsoft.com/v1.0/users/probe%20user@example.com/sendMail"; got != want {
		t.Errorf("URL = %q, want %q", got, want)
	}
}

func TestOpenSender_RequiresMailbox(t *testing.T) {
	_, err := OpenSender(context.Background(), NewConfig(), nil)
	if err == nil || !strings.Contains(err.Error(), "mailbox is required") {
		t.Fatalf("OpenSender() error = %v, want mailbox error", err)
	}
}
