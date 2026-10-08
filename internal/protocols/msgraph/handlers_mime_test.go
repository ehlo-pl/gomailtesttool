//go:build !integration
// +build !integration

package msgraph

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	abstractions "github.com/microsoft/kiota-abstractions-go"
	"github.com/spf13/viper"
)

func TestBuildMIMERequest(t *testing.T) {
	const message = "From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: Test\r\n\r\nBody\r\n"
	path := filepath.Join(t.TempDir(), "message.eml")
	if err := os.WriteFile(path, []byte(message), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	adapter := &testMIMERequestAdapter{baseURL: "https://graph.microsoft.com/v1.0/"}

	request, err := buildMIMERequest(adapter, "sender@example.com", path)
	if err != nil {
		t.Fatalf("buildMIMERequest() error = %v", err)
	}
	if request.Method != abstractions.POST {
		t.Errorf("Method = %v, want POST", request.Method)
	}
	if got := request.Headers.Get("Content-Type"); len(got) != 1 || got[0] != "text/plain" {
		t.Errorf("Content-Type = %v, want [text/plain]", got)
	}
	if got, want := string(request.Content), base64.StdEncoding.EncodeToString([]byte(message)); got != want {
		t.Errorf("request body = %q, want base64 encoding %q", got, want)
	}
	requestURL, err := request.GetUri()
	if err != nil {
		t.Fatalf("GetUri() error = %v", err)
	}
	if got, want := requestURL.String(), "https://graph.microsoft.com/v1.0/users/sender@example.com/sendMail"; got != want {
		t.Errorf("URL = %q, want %q", got, want)
	}
}

type testMIMERequestAdapter struct {
	abstractions.RequestAdapter
	baseURL string
}

func (a *testMIMERequestAdapter) GetBaseUrl() string {
	return a.baseURL
}

func TestValidateConfiguration_MIMEBase64(t *testing.T) {
	messagePath := filepath.Join(t.TempDir(), "message.eml")
	if err := os.WriteFile(messagePath, []byte("not parsed"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	templatePath := filepath.Join(t.TempDir(), "template.html")
	if err := os.WriteFile(templatePath, []byte("template"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	valid := func() *Config {
		config := validConfigForPriorityTest("normal")
		config.Action = ActionSendMail
		config.MIMEBase64 = messagePath
		return config
	}

	t.Run("complete eml passes", func(t *testing.T) {
		if err := validateConfiguration(valid()); err != nil {
			t.Fatalf("validateConfiguration() error = %v", err)
		}
	})

	tests := []struct {
		name      string
		configure func(*Config)
		want      string
	}{
		{
			name: "missing file",
			configure: func(config *Config) {
				config.MIMEBase64 = strings.TrimSuffix(messagePath, ".eml") + ".missing.eml"
			},
			want: "file not found",
		},
		{
			name: "wrong extension",
			configure: func(config *Config) {
				config.MIMEBase64 = strings.TrimSuffix(messagePath, ".eml") + ".txt"
			},
			want: "requires a .eml file",
		},
		{
			name: "template cannot be combined",
			configure: func(config *Config) {
				config.Template = templatePath
			},
			want: "--template",
		},
		{
			name: "template variables cannot be combined",
			configure: func(config *Config) {
				config.TemplateVars = []string{"Name=World"}
			},
			want: "--template",
		},
		{
			name: "recipients cannot be overridden",
			configure: func(config *Config) {
				config.To = stringSlice{"recipient@example.com"}
			},
			want: "recipient",
		},
		{
			name: "body cannot be overridden",
			configure: func(config *Config) {
				config.Body = "override"
			},
			want: "message content",
		},
		{
			name: "only sendmail supports mime",
			configure: func(config *Config) {
				config.Action = ActionSaveDraft
			},
			want: "only supported by sendmail",
		},
		{
			name: "save to sent is unavailable",
			configure: func(config *Config) {
				config.SaveToSent = true
			},
			want: "--save-to-sent",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := valid()
			test.configure(config)
			err := validateConfiguration(config)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateConfiguration() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestBuildMIMERequestRejectsUnreadablePath(t *testing.T) {
	_, err := buildMIMERequest(&testMIMERequestAdapter{baseURL: "https://graph.microsoft.com/v1.0"}, "sender@example.com", "/no/such/message.eml")
	if err == nil || !strings.Contains(err.Error(), "failed to read MIME message file") {
		t.Fatalf("buildMIMERequest() error = %v, want read error", err)
	}
}

func TestSendmailMIMEBase64RejectsExplicitDefaultContentFlag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "message.eml")
	if err := os.WriteFile(path, []byte("opaque message"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	command := newSendMailCmd(viper.New())
	if err := command.Flags().Set("mimebase64", path); err != nil {
		t.Fatalf("Set(mimebase64) error = %v", err)
	}
	if err := command.Flags().Set("subject", NewConfig().Subject); err != nil {
		t.Fatalf("Set(subject) error = %v", err)
	}
	err := command.RunE(command, nil)
	if err == nil || !strings.Contains(err.Error(), "--mimebase64 cannot be combined with --subject") {
		t.Fatalf("RunE() error = %v, want explicit subject conflict", err)
	}
}
