//go:build !integration
// +build !integration

package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// TestConfigFilePrecedence verifies viper's precedence order once empirically:
// CLI flag > env var > config file > pflag default. This underpins the
// "--config provides defaults, flags/env still win" behavior for --config.
func TestConfigFilePrecedence(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: 587\nstarttls: true\n"), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	newViperWithFlags := func() (*viper.Viper, *pflag.FlagSet) {
		v := viper.New()
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		fs.Int("port", 25, "port")
		fs.Bool("starttls", false, "starttls")
		_ = v.BindPFlags(fs)
		_ = v.BindEnv("port", "SMTPPORT")
		return v, fs
	}

	t.Run("config file overrides pflag default", func(t *testing.T) {
		v, _ := newViperWithFlags()
		if err := LoadConfigFile(v, configPath); err != nil {
			t.Fatalf("LoadConfigFile() error = %v", err)
		}
		if got := v.GetInt("port"); got != 587 {
			t.Errorf("port = %d, want 587 (from config file)", got)
		}
		if got := v.GetBool("starttls"); !got {
			t.Errorf("starttls = %v, want true (from config file)", got)
		}
	})

	t.Run("explicit CLI flag overrides config file", func(t *testing.T) {
		v, fs := newViperWithFlags()
		if err := LoadConfigFile(v, configPath); err != nil {
			t.Fatalf("LoadConfigFile() error = %v", err)
		}
		if err := fs.Set("port", "25"); err != nil {
			t.Fatalf("fs.Set: %v", err)
		}
		if got := v.GetInt("port"); got != 25 {
			t.Errorf("port = %d, want 25 (explicit CLI flag)", got)
		}
	})

	t.Run("env var overrides config file when flag not set", func(t *testing.T) {
		v, _ := newViperWithFlags()
		if err := LoadConfigFile(v, configPath); err != nil {
			t.Fatalf("LoadConfigFile() error = %v", err)
		}
		t.Setenv("SMTPPORT", "2525")
		if got := v.GetInt("port"); got != 2525 {
			t.Errorf("port = %d, want 2525 (env overrides config file)", got)
		}
	})

	t.Run("no config path is a no-op", func(t *testing.T) {
		v, _ := newViperWithFlags()
		if err := LoadConfigFile(v, ""); err != nil {
			t.Fatalf("LoadConfigFile(\"\") error = %v", err)
		}
		if got := v.GetInt("port"); got != 25 {
			t.Errorf("port = %d, want 25 (pflag default)", got)
		}
	})

	t.Run("missing config file returns error", func(t *testing.T) {
		v, _ := newViperWithFlags()
		err := LoadConfigFile(v, filepath.Join(dir, "missing.yaml"))
		if err == nil {
			t.Error("LoadConfigFile() error = nil, want error for missing file")
		}
	})
}

// TestLoadConfigFileSection verifies the nested-section loading used by
// `serve` to pull "smtp"/"msgraph" defaults out of a single --config file.
func TestLoadConfigFileSection(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "serve.yaml")
	content := `
port: 9090
smtp:
  host: smtp.fromconfig.example.com
  port: 2525
  starttls: true
msgraph:
  tenantid: 11111111-1111-1111-1111-111111111111
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	t.Run("merges section into target viper", func(t *testing.T) {
		v := viper.New()
		_ = v.BindEnv("port", "SMTPPORT")
		if err := LoadConfigFileSection(v, configPath, "smtp"); err != nil {
			t.Fatalf("LoadConfigFileSection() error = %v", err)
		}
		if got := v.GetString("host"); got != "smtp.fromconfig.example.com" {
			t.Errorf("host = %q, want smtp.fromconfig.example.com", got)
		}
		if got := v.GetInt("port"); got != 2525 {
			t.Errorf("port = %d, want 2525", got)
		}
		if got := v.GetBool("starttls"); !got {
			t.Error("starttls = false, want true")
		}
	})

	t.Run("env var overrides merged section value", func(t *testing.T) {
		v := viper.New()
		_ = v.BindEnv("port", "SMTPPORT")
		if err := LoadConfigFileSection(v, configPath, "smtp"); err != nil {
			t.Fatalf("LoadConfigFileSection() error = %v", err)
		}
		t.Setenv("SMTPPORT", "9999")
		if got := v.GetInt("port"); got != 9999 {
			t.Errorf("port = %d, want 9999 (env overrides config section)", got)
		}
	})

	t.Run("missing section is a no-op", func(t *testing.T) {
		v := viper.New()
		if err := LoadConfigFileSection(v, configPath, "ews"); err != nil {
			t.Fatalf("LoadConfigFileSection() error = %v", err)
		}
		if got := v.GetString("host"); got != "" {
			t.Errorf("host = %q, want empty for absent section", got)
		}
	})

	t.Run("empty path is a no-op", func(t *testing.T) {
		v := viper.New()
		if err := LoadConfigFileSection(v, "", "smtp"); err != nil {
			t.Fatalf("LoadConfigFileSection() error = %v", err)
		}
		if got := v.GetString("host"); got != "" {
			t.Errorf("host = %q, want empty", got)
		}
	})

	t.Run("top-level key unaffected by section load", func(t *testing.T) {
		v := viper.New()
		_ = v.BindEnv("port", "SOME_OTHER_ENV")
		if err := LoadConfigFile(v, configPath); err != nil {
			t.Fatalf("LoadConfigFile() error = %v", err)
		}
		if got := v.GetInt("port"); got != 9090 {
			t.Errorf("port = %d, want 9090 (top-level serve port)", got)
		}
	})
}
