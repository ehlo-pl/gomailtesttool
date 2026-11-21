//go:build !integration

package smtp

import (
	"testing"

	"github.com/ehlo-pl/gomailtesttool/internal/common/wikicontent"
	"github.com/spf13/viper"
)

// TestConfigFromViper_BodySource checks that the body and body source can come
// from env vars, and that an env-supplied body counts as "a body was given".
func TestConfigFromViper_BodySource(t *testing.T) {
	tests := []struct {
		name       string
		env        map[string]string
		wantSource string
		wantNoBody bool
	}{
		{name: "defaults", wantSource: "", wantNoBody: true},
		{name: "SMTPBODY env counts as a body", env: map[string]string{"SMTPBODY": "from env"}, wantNoBody: false},
		{name: "SMTPBODYSOURCE env", env: map[string]string{"SMTPBODYSOURCE": "Static"}, wantSource: wikicontent.BodySourceStatic, wantNoBody: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			v := viper.New()
			BindEnvs(v)
			config := ConfigFromViper(v)
			if config.BodySource != tt.wantSource {
				t.Errorf("BodySource = %q, want %q", config.BodySource, tt.wantSource)
			}
			if got := wikicontent.NoBodyGiven(config.Body, NewConfig().Body, config.BodyHTML, config.Template); got != tt.wantNoBody {
				t.Errorf("NoBodyGiven = %v, want %v (body %q)", got, tt.wantNoBody, config.Body)
			}
		})
	}
}

func TestValidateConfiguration_BodySource(t *testing.T) {
	config := NewConfig()
	config.Host = "smtp.example.com"
	config.Action = ActionTestConnect
	config.BodySource = "random"
	if err := validateConfiguration(config); err == nil {
		t.Fatal("validateConfiguration() error = nil, want invalid body source")
	}
}
