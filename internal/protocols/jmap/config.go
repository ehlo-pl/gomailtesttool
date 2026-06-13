package jmap

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/ziembor/gomailtesttool/internal/common/network"
	"github.com/ziembor/gomailtesttool/internal/common/validation"
)

// Config holds all jmaptool configuration.
type Config struct {
	// Core configuration
	Action string

	// JMAP server configuration
	Host           string
	Port           int
	ConnectAddress string // Override address for TCP connection (IP or hostname)
	IPv4Only       bool   // Force resolving --host/--address to an IPv4 (A record) address
	IPv6Only       bool   // Force resolving --host/--address to an IPv6 (AAAA record) address

	// Authentication
	Username    string
	Password    string
	AccessToken string // OAuth2 bearer token
	AuthMethod  string // auto, basic, bearer

	// TLS configuration
	SkipVerify bool

	// Runtime configuration
	VerboseMode bool
	LogLevel    string
	LogFormat   string
}

// Action constants
const (
	ActionTestConnect  = "testconnect"
	ActionTestAuth     = "testauth"
	ActionGetMailboxes = "getmailboxes"
)

// NewConfig creates a new Config with default values.
func NewConfig() *Config {
	return &Config{
		Port:       443,
		AuthMethod: "auto",
		LogLevel:   "info",
		LogFormat:  "csv",
	}
}

// RegisterPersistentFlags registers flags shared by all jmap subcommands
// on the given parent command (the "jmap" cobra.Command).
func RegisterPersistentFlags(cmd *cobra.Command) {
	f := cmd.PersistentFlags()

	// JMAP server
	f.String("host", "", "JMAP server hostname (required) — the service to connect to; also used for TLS SNI/certificate checks and authentication (env: JMAPHOST)")
	f.Int("port", 443, "JMAP server port (env: JMAPPORT)")
	f.String("address", "", "Optional: connect to this IP/hostname instead of --host (e.g. to test a specific server behind a load balancer); --host is still used for SNI, certificate checks, and authentication (env: JMAPADDRESS)")
	f.Bool("ipv4", false, "Force IPv4: resolve --host/--address to an A record and connect over IPv4 (env: JMAPIPV4)")
	f.Bool("ipv6", false, "Force IPv6: resolve --host/--address to an AAAA record and connect over IPv6 (env: JMAPIPV6)")

	// Authentication
	f.String("username", "", "Username for authentication (env: JMAPUSERNAME)")
	f.String("password", "", "Password for authentication (env: JMAPPASSWORD)")
	f.String("accesstoken", "", "Access token for Bearer authentication (env: JMAPACCESSTOKEN)")
	f.String("authmethod", "auto", "Authentication method: auto, basic, bearer (env: JMAPAUTHMETHOD)")

	// TLS
	f.Bool("skipverify", false, "Skip TLS certificate verification (insecure) (env: JMAPSKIPVERIFY)")

	// Output
	f.Bool("verbose", false, "Enable verbose output (env: JMAPVERBOSE)")
	f.String("loglevel", "info", "Log level: debug, info, warn, error (env: JMAPLOGLEVEL)")
	f.String("logformat", "csv", "Log file format: csv, json (env: JMAPLOGFORMAT)")
}

// BindEnvs registers Viper environment variable bindings for all jmap config keys.
// Must be called after RegisterPersistentFlags.
func BindEnvs(v *viper.Viper) {
	bindings := map[string]string{
		"host":        "JMAPHOST",
		"port":        "JMAPPORT",
		"address":     "JMAPADDRESS",
		"ipv4":        "JMAPIPV4",
		"ipv6":        "JMAPIPV6",
		"username":    "JMAPUSERNAME",
		"password":    "JMAPPASSWORD",
		"accesstoken": "JMAPACCESSTOKEN",
		"authmethod":  "JMAPAUTHMETHOD",
		"skipverify":  "JMAPSKIPVERIFY",
		"verbose":     "JMAPVERBOSE",
		"loglevel":    "JMAPLOGLEVEL",
		"logformat":   "JMAPLOGFORMAT",
	}
	for key, env := range bindings {
		_ = v.BindEnv(key, env)
	}
}

// ConfigFromViper reads all jmap config values from the given Viper instance.
// The action field must be set by the caller (it comes from which subcommand ran).
func ConfigFromViper(v *viper.Viper) *Config {
	defaults := NewConfig()

	port := v.GetInt("port")
	if port <= 0 {
		port = defaults.Port
	}

	authMethod := v.GetString("authmethod")
	if authMethod == "" {
		authMethod = defaults.AuthMethod
	}

	logLevel := strings.ToLower(v.GetString("loglevel"))
	if logLevel == "" {
		logLevel = defaults.LogLevel
	}

	logFormat := strings.ToLower(v.GetString("logformat"))
	if logFormat == "" {
		logFormat = defaults.LogFormat
	}

	return &Config{
		Host:           v.GetString("host"),
		Port:           port,
		ConnectAddress: v.GetString("address"),
		IPv4Only:       v.GetBool("ipv4"),
		IPv6Only:       v.GetBool("ipv6"),
		Username:       v.GetString("username"),
		Password:       v.GetString("password"),
		AccessToken:    v.GetString("accesstoken"),
		AuthMethod:     authMethod,
		SkipVerify:     v.GetBool("skipverify"),
		VerboseMode:    v.GetBool("verbose"),
		LogLevel:       logLevel,
		LogFormat:      logFormat,
	}
}

// validateConfiguration validates the configuration.
func validateConfiguration(config *Config) error {
	// Validate action
	validActions := []string{ActionTestConnect, ActionTestAuth, ActionGetMailboxes}
	valid := false
	for _, a := range validActions {
		if config.Action == a {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid action: %s (must be one of: %s)", config.Action, strings.Join(validActions, ", "))
	}

	// Validate host
	if config.Host == "" {
		return fmt.Errorf("host is required (--host flag)")
	}

	// Validate port
	if config.Port <= 0 || config.Port > 65535 {
		return fmt.Errorf("invalid port: %d (must be 1-65535)", config.Port)
	}

	// Validate connect address (if provided)
	if config.ConnectAddress != "" {
		if err := validation.ValidateHostname(config.ConnectAddress); err != nil {
			return fmt.Errorf("invalid connect address: %w", err)
		}
	}

	// Validate mutual exclusion: --ipv4 and --ipv6 cannot be used together
	if err := network.ValidateIPVersionFlags(config.IPv4Only, config.IPv6Only); err != nil {
		return err
	}

	// Validate auth method
	config.AuthMethod = strings.ToLower(config.AuthMethod)
	validAuthMethods := map[string]bool{
		"auto":   true,
		"basic":  true,
		"bearer": true,
	}
	if !validAuthMethods[config.AuthMethod] {
		return fmt.Errorf("invalid auth method: %s (valid: auto, basic, bearer)", config.AuthMethod)
	}

	// Action-specific credential validation
	switch config.Action {
	case ActionTestAuth, ActionGetMailboxes:
		if config.AccessToken == "" && config.Password == "" {
			return fmt.Errorf("%s requires either --password or --accesstoken", config.Action)
		}
	}

	// Validate log level
	config.LogLevel = strings.ToLower(config.LogLevel)
	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[config.LogLevel] {
		return fmt.Errorf("invalid log level: %s (valid: debug, info, warn, error)", config.LogLevel)
	}

	// Validate log format
	config.LogFormat = strings.ToLower(config.LogFormat)
	if config.LogFormat != "csv" && config.LogFormat != "json" {
		return fmt.Errorf("invalid log format: %s (valid: csv, json)", config.LogFormat)
	}

	return nil
}
