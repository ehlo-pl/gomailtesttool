package jmap

import (
	"testing"
)

// newTestConfig returns a valid Config for testing with all required defaults set.
func newTestConfig() *Config {
	return &Config{
		Action:     ActionTestConnect,
		Host:       "jmap.example.com",
		Port:       443,
		AuthMethod: "auto",
		LogLevel:   "info",
		LogFormat:  "csv",
	}
}

func TestNewConfig(t *testing.T) {
	config := NewConfig()

	if config.Port != 443 {
		t.Errorf("NewConfig() Port = %d, want 443", config.Port)
	}
	if config.AuthMethod != "auto" {
		t.Errorf("NewConfig() AuthMethod = %s, want auto", config.AuthMethod)
	}
	if config.LogLevel != "info" {
		t.Errorf("NewConfig() LogLevel = %s, want info", config.LogLevel)
	}
	if config.LogFormat != "csv" {
		t.Errorf("NewConfig() LogFormat = %s, want csv", config.LogFormat)
	}
}

func TestValidateConfiguration_Action(t *testing.T) {
	tests := []struct {
		name    string
		action  string
		wantErr bool
	}{
		{"valid testconnect", ActionTestConnect, false},
		{"valid testauth", ActionTestAuth, false},
		{"valid getmailboxes", ActionGetMailboxes, false},
		{"invalid action", "invalid", true},
		{"empty action", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := newTestConfig()
			config.Action = tt.action
			if tt.action == ActionTestAuth || tt.action == ActionGetMailboxes {
				config.AccessToken = "test-token"
			}
			err := validateConfiguration(config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfiguration() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateConfiguration_Host(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		wantErr bool
	}{
		{"valid host", "jmap.example.com", false},
		{"empty host", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := newTestConfig()
			config.Host = tt.host
			err := validateConfiguration(config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfiguration() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateConfiguration_Port(t *testing.T) {
	tests := []struct {
		name    string
		port    int
		wantErr bool
	}{
		{"valid port 443", 443, false},
		{"valid port 8080", 8080, false},
		{"port 0", 0, true},
		{"port negative", -1, true},
		{"port too high", 70000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := newTestConfig()
			config.Port = tt.port
			err := validateConfiguration(config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfiguration() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateConfiguration_AuthMethod(t *testing.T) {
	tests := []struct {
		name       string
		authMethod string
		wantErr    bool
	}{
		{"auto", "auto", false},
		{"basic", "basic", false},
		{"bearer", "bearer", false},
		{"uppercase AUTO", "AUTO", false}, // normalised by validateConfiguration
		{"invalid", "oauth", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := newTestConfig()
			config.AuthMethod = tt.authMethod
			err := validateConfiguration(config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfiguration() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateConfiguration_Credentials(t *testing.T) {
	tests := []struct {
		name        string
		action      string
		password    string
		accessToken string
		wantErr     bool
	}{
		{"testconnect no creds", ActionTestConnect, "", "", false},
		{"testauth with token", ActionTestAuth, "", "token", false},
		{"testauth with password", ActionTestAuth, "pass", "", false},
		{"testauth no creds", ActionTestAuth, "", "", true},
		{"getmailboxes with token", ActionGetMailboxes, "", "token", false},
		{"getmailboxes no creds", ActionGetMailboxes, "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := newTestConfig()
			config.Action = tt.action
			config.Password = tt.password
			config.AccessToken = tt.accessToken
			err := validateConfiguration(config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfiguration() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateConfiguration_LogLevel(t *testing.T) {
	tests := []struct {
		name     string
		logLevel string
		wantErr  bool
	}{
		{"debug", "debug", false},
		{"info", "info", false},
		{"warn", "warn", false},
		{"error", "error", false},
		{"uppercase INFO", "INFO", false}, // normalised
		{"invalid", "verbose", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := newTestConfig()
			config.LogLevel = tt.logLevel
			err := validateConfiguration(config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfiguration() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateConfiguration_LogFormat(t *testing.T) {
	tests := []struct {
		name      string
		logFormat string
		wantErr   bool
	}{
		{"csv", "csv", false},
		{"json", "json", false},
		{"uppercase CSV", "CSV", false}, // normalised
		{"invalid", "xml", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := newTestConfig()
			config.LogFormat = tt.logFormat
			err := validateConfiguration(config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfiguration() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
