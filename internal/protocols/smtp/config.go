package smtp

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"msgraphtool/internal/common/validation"
)

// Config holds all smtptool configuration.
type Config struct {
	// Core configuration
	ShowVersion bool
	Action      string

	// SMTP server configuration
	Host    string
	Port    int
	Timeout time.Duration

	// Authentication
	Username    string
	Password    string
	AccessToken string // OAuth2 access token for XOAUTH2 authentication
	AuthMethod  string // PLAIN, LOGIN, CRAM-MD5, XOAUTH2, or "auto"

	// Email configuration (for sendmail)
	From    string
	To      []string
	Subject string
	Body    string

	// TLS configuration
	StartTLS   bool   // Force STARTTLS
	SMTPS      bool   // Use SMTPS (implicit TLS on port 465)
	SkipVerify bool   // Skip TLS certificate verification
	TLSVersion string // TLS version to use (exact match): 1.2, 1.3

	// Network configuration
	ConnectAddress string        // Override address for TCP connection (IP or hostname)
	ProxyURL       string
	MaxRetries     int
	RetryDelay     time.Duration

	// Runtime configuration
	VerboseMode  bool
	LogLevel     string
	OutputFormat string
	LogFormat    string  // Log file format: csv, json
	RateLimit    float64 // Maximum requests per second (0 = unlimited)
}

// Action constants
const (
	ActionTestConnect  = "testconnect"
	ActionTestStartTLS = "teststarttls"
	ActionTestAuth     = "testauth"
	ActionSendMail     = "sendmail"
)

// NewConfig creates a new Config with default values.
func NewConfig() *Config {
	return &Config{
		Port:         25,
		Timeout:      30 * time.Second,
		AuthMethod:   "auto",
		Subject:      "SMTP Test",
		Body:         "This is a test message from smtptool",
		StartTLS:     false, // Auto-detect
		SkipVerify:   false,
		TLSVersion:   "1.2",
		MaxRetries:   3,
		RetryDelay:   2000 * time.Millisecond,
		VerboseMode:  false,
		LogLevel:     "INFO",
		OutputFormat: "text",
		LogFormat:    "csv",
		RateLimit:    0, // Unlimited by default
	}
}

// RegisterPersistentFlags registers flags shared by all smtp subcommands
// on the given parent command (the "smtp" cobra.Command).
func RegisterPersistentFlags(cmd *cobra.Command) {
	f := cmd.PersistentFlags()

	// SMTP server
	f.String("host", "", "SMTP server hostname or IP address (env: SMTPHOST)")
	f.Int("port", 25, "SMTP server port (env: SMTPPORT)")
	f.Int("timeout", 30, "Connection timeout in seconds (env: SMTPTIMEOUT)")

	// Authentication
	f.String("username", "", "SMTP username for authentication (env: SMTPUSERNAME)")
	f.String("password", "", "SMTP password for authentication (env: SMTPPASSWORD)")
	f.String("accesstoken", "", "OAuth2 access token for XOAUTH2 authentication (env: SMTPACCESSTOKEN)")
	f.String("authmethod", "auto", "Authentication method: PLAIN, LOGIN, CRAM-MD5, NTLM, XOAUTH2, auto (env: SMTPAUTHMETHOD)")

	// TLS
	f.Bool("starttls", false, "Force STARTTLS usage (env: SMTPSTARTTLS)")
	f.Bool("smtps", false, "Use SMTPS (implicit TLS), typically on port 465 (env: SMTPSMTPS)")
	f.Bool("skipverify", false, "Skip TLS certificate verification (insecure) (env: SMTPSKIPVERIFY)")
	f.String("tlsversion", "1.2", "TLS version to use (exact): 1.2, 1.3 (env: SMTPTLSVERSION)")

	// Network
	f.String("address", "", "Override IP address or hostname for TCP connection (env: SMTPADDRESS)")
	f.String("proxy", "", "HTTP/HTTPS proxy URL (env: SMTPPROXY)")
	f.Int("maxretries", 3, "Maximum retry attempts (env: SMTPMAXRETRIES)")
	f.Int("retrydelay", 2000, "Retry delay in milliseconds (env: SMTPRETRYDELAY)")
	f.Float64("ratelimit", 0, "Maximum SMTP requests per second (0 = unlimited) (env: SMTPRATELIMIT)")

	// Output
	f.Bool("verbose", false, "Enable verbose output")
	f.String("loglevel", "INFO", "Logging level: DEBUG, INFO, WARN, ERROR")
	f.String("output", "text", "Output format: text, json (env: SMTPOUTPUT)")
	f.String("logformat", "csv", "Log file format: csv, json (env: SMTPLOGFORMAT)")
}

// BindEnvs registers Viper environment variable bindings for all smtp config keys.
// Must be called after RegisterPersistentFlags.
func BindEnvs(v *viper.Viper) {
	bindings := map[string]string{
		"host":        "SMTPHOST",
		"port":        "SMTPPORT",
		"timeout":     "SMTPTIMEOUT",
		"username":    "SMTPUSERNAME",
		"password":    "SMTPPASSWORD",
		"accesstoken": "SMTPACCESSTOKEN",
		"authmethod":  "SMTPAUTHMETHOD",
		"from":        "SMTPFROM",
		"to":          "SMTPTO",
		"starttls":    "SMTPSTARTTLS",
		"smtps":       "SMTPSMTPS",
		"skipverify":  "SMTPSKIPVERIFY",
		"tlsversion":  "SMTPTLSVERSION",
		"address":     "SMTPADDRESS",
		"proxy":       "SMTPPROXY",
		"maxretries":  "SMTPMAXRETRIES",
		"retrydelay":  "SMTPRETRYDELAY",
		"output":      "SMTPOUTPUT",
		"logformat":   "SMTPLOGFORMAT",
		"ratelimit":   "SMTPRATELIMIT",
	}
	for key, env := range bindings {
		_ = v.BindEnv(key, env)
	}
}

// ConfigFromViper reads all smtp config values from the given Viper instance.
// The action field must be set by the caller (it comes from which subcommand ran).
func ConfigFromViper(v *viper.Viper) *Config {
	defaults := NewConfig()

	port := v.GetInt("port")
	if port <= 0 {
		port = defaults.Port
	}

	timeoutSec := v.GetInt("timeout")
	if timeoutSec <= 0 {
		timeoutSec = 30
	}

	retryDelayMs := v.GetInt("retrydelay")
	if retryDelayMs <= 0 {
		retryDelayMs = 2000
	}

	maxRetries := v.GetInt("maxretries")
	if maxRetries < 0 {
		maxRetries = defaults.MaxRetries
	}

	authMethod := v.GetString("authmethod")
	if authMethod == "" {
		authMethod = defaults.AuthMethod
	}

	tlsVersion := v.GetString("tlsversion")
	if tlsVersion == "" {
		tlsVersion = defaults.TLSVersion
	}

	logLevel := v.GetString("loglevel")
	if logLevel == "" {
		logLevel = defaults.LogLevel
	}

	outputFormat := strings.ToLower(v.GetString("output"))
	if outputFormat == "" {
		outputFormat = defaults.OutputFormat
	}

	logFormat := strings.ToLower(v.GetString("logformat"))
	if logFormat == "" {
		logFormat = defaults.LogFormat
	}

	// Parse "to" as comma-separated list
	var toList []string
	if toStr := v.GetString("to"); toStr != "" {
		for _, addr := range strings.Split(toStr, ",") {
			if trimmed := strings.TrimSpace(addr); trimmed != "" {
				toList = append(toList, trimmed)
			}
		}
	}

	subject := v.GetString("subject")
	if subject == "" {
		subject = defaults.Subject
	}

	body := v.GetString("body")
	if body == "" {
		body = defaults.Body
	}

	return &Config{
		Host:           v.GetString("host"),
		Port:           port,
		Timeout:        time.Duration(timeoutSec) * time.Second,
		Username:       v.GetString("username"),
		Password:       v.GetString("password"),
		AccessToken:    v.GetString("accesstoken"),
		AuthMethod:     authMethod,
		From:           v.GetString("from"),
		To:             toList,
		Subject:        subject,
		Body:           body,
		StartTLS:       v.GetBool("starttls"),
		SMTPS:          v.GetBool("smtps"),
		SkipVerify:     v.GetBool("skipverify"),
		TLSVersion:     tlsVersion,
		ConnectAddress: v.GetString("address"),
		ProxyURL:       v.GetString("proxy"),
		MaxRetries:     maxRetries,
		RetryDelay:     time.Duration(retryDelayMs) * time.Millisecond,
		VerboseMode:    v.GetBool("verbose"),
		LogLevel:       logLevel,
		OutputFormat:   outputFormat,
		LogFormat:      logFormat,
		RateLimit:      v.GetFloat64("ratelimit"),
	}
}

// parseBoolEnv parses a boolean environment variable.
// Accepts truthy values: "true", "1", "yes", "on" (case-insensitive)
// Returns false for empty string or any other value.
func parseBoolEnv(value string) bool {
	switch strings.ToLower(value) {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}

// validateConfiguration validates the configuration.
func validateConfiguration(config *Config) error {
	// Validate action
	validActions := []string{ActionTestConnect, ActionTestStartTLS, ActionTestAuth, ActionSendMail}
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

	// Security warning for TLS certificate verification bypass
	if config.SkipVerify {
		fmt.Println("╔════════════════════════════════════════════════════════════════╗")
		fmt.Println("║  ⚠️  WARNING: TLS CERTIFICATE VERIFICATION DISABLED            ║")
		fmt.Println("║                                                                ║")
		fmt.Println("║  The -skipverify flag disables TLS certificate validation.    ║")
		fmt.Println("║  This makes the connection vulnerable to man-in-the-middle    ║")
		fmt.Println("║  attacks. Only use this for testing with self-signed certs.   ║")
		fmt.Println("╚════════════════════════════════════════════════════════════════╝")
		fmt.Println()
	}

	// Validate mutual exclusion: -smtps and -starttls cannot be used together
	if config.SMTPS && config.StartTLS {
		return fmt.Errorf("cannot use both -smtps and -starttls flags simultaneously")
	}

	// Smart port default: if -smtps is set and port is 25 (default), change to 465
	if config.SMTPS && config.Port == 25 {
		config.Port = 465
	}

	// Validate host (required for all actions)
	if config.Host == "" {
		return fmt.Errorf("host is required (-host flag)")
	}
	if err := validation.ValidateHostname(config.Host); err != nil {
		return fmt.Errorf("invalid host: %w", err)
	}

	// Validate port
	if err := validation.ValidatePort(config.Port); err != nil {
		return fmt.Errorf("invalid port: %w", err)
	}

	// Validate proxy URL (if provided)
	if err := validation.ValidateProxyURL(config.ProxyURL); err != nil {
		return fmt.Errorf("invalid proxy URL: %w", err)
	}

	// Validate connect address (if provided)
	if config.ConnectAddress != "" {
		if err := validation.ValidateHostname(config.ConnectAddress); err != nil {
			return fmt.Errorf("invalid connect address: %w", err)
		}
	}

	// Action-specific validation
	switch config.Action {
	case ActionTestAuth:
		if config.Username == "" {
			return fmt.Errorf("testauth requires -username")
		}
		// XOAUTH2 requires accesstoken instead of password
		if strings.EqualFold(config.AuthMethod, "XOAUTH2") {
			if config.AccessToken == "" {
				return fmt.Errorf("XOAUTH2 authentication requires -accesstoken")
			}
			if config.Password != "" {
				fmt.Println("Warning: both -password and -accesstoken provided; -password will be ignored for XOAUTH2")
			}
		} else if config.AccessToken != "" {
			// If accesstoken provided, assume XOAUTH2
			// No password required
			if config.Password != "" {
				fmt.Println("Warning: both -password and -accesstoken provided; -password will be ignored (using XOAUTH2)")
			}
		} else if config.Password == "" {
			return fmt.Errorf("testauth requires -password (or -accesstoken for XOAUTH2)")
		}

	case ActionSendMail:
		if config.From == "" {
			return fmt.Errorf("sendmail requires -from")
		}
		if err := validation.ValidateEmail(config.From); err != nil {
			return fmt.Errorf("invalid sender email: %w", err)
		}
		if len(config.To) == 0 {
			return fmt.Errorf("sendmail requires -to")
		}
		for _, email := range config.To {
			if err := validation.ValidateEmail(strings.TrimSpace(email)); err != nil {
				return fmt.Errorf("invalid recipient email: %w", err)
			}
		}
		if config.Subject == "" {
			return fmt.Errorf("sendmail requires -subject")
		}
	}

	return nil
}
