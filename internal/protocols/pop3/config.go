package pop3

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/ehlo-pl/gomailtesttool/internal/common/network"
	"github.com/ehlo-pl/gomailtesttool/internal/common/validation"
)

// Config holds all pop3tool configuration.
type Config struct {
	// Core configuration
	ShowVersion bool
	Action      string

	// POP3 server configuration
	Host    string
	Port    int
	Timeout time.Duration

	// Authentication
	Username    string
	Password    string
	AccessToken string // OAuth2 access token for XOAUTH2 authentication
	AuthMethod  string // USER, APOP, XOAUTH2, or "auto"

	// List options
	MaxMessages int // Maximum messages to list

	// TLS configuration
	POP3S      bool   // Use POP3S (implicit TLS on port 995)
	StartTLS   bool   // Force STLS
	NoStartTLS bool   // Force plain connection: disable STLS; error if --starttls is also set
	NoPOP3S    bool   // Force plain connection: error if --pop3s is also set
	SkipVerify bool   // Skip TLS certificate verification
	TLSVersion string // TLS version to use: 1.2, 1.3

	// Network configuration
	ConnectAddress string // Override address for TCP connection (IP or hostname)
	IPv4Only       bool   // Force resolving --host/--address to an IPv4 (A record) address
	IPv6Only       bool   // Force resolving --host/--address to an IPv6 (AAAA record) address
	ProxyURL       string
	MaxRetries     int
	RetryDelay     time.Duration

	// Runtime configuration
	VerboseMode  bool
	LogLevel     string
	OutputFormat string
	LogFormat    string  // Log file format: csv, json
	RateLimit    float64 // Maximum requests per second (0 = unlimited)

	// exportmessages configuration
	Subject   string // Subject substring to search for
	MessageID string // Message-ID header to search for
	Count     int    // Maximum number of matching messages to export
	ExportDir string // Directory under which to create the dated export folder (default: OS temp dir)
}

// Action constants
const (
	ActionTestConnect    = "testconnect"
	ActionTestAuth       = "testauth"
	ActionListMail       = "listmail"
	ActionExportMessages = "exportmessages"
)

// NewConfig creates a new Config with default values.
func NewConfig() *Config {
	return &Config{
		Port:         110,
		Timeout:      30 * time.Second,
		AuthMethod:   "auto",
		MaxMessages:  100,
		POP3S:        false,
		StartTLS:     false,
		SkipVerify:   false,
		TLSVersion:   "1.2",
		MaxRetries:   3,
		RetryDelay:   2000 * time.Millisecond,
		VerboseMode:  false,
		LogLevel:     "INFO",
		OutputFormat: "text",
		LogFormat:    "csv",
		RateLimit:    0,
		Count:        25,
	}
}

// RegisterPersistentFlags registers flags shared by all pop3 subcommands
// on the given parent command (the "pop3" cobra.Command).
func RegisterPersistentFlags(cmd *cobra.Command) {
	f := cmd.PersistentFlags()

	// POP3 server
	f.String("host", "", "POP3 server hostname (required) — the service to connect to; also used for TLS SNI/certificate checks and authentication (env: POP3HOST)")
	f.Int("port", 110, "POP3 server port (env: POP3PORT)")
	f.Int("timeout", 30, "Connection timeout in seconds (env: POP3TIMEOUT)")

	// Authentication
	f.String("username", "", "Username for authentication (env: POP3USERNAME)")
	f.String("password", "", "Password for authentication (env: POP3PASSWORD)")
	f.String("accesstoken", "", "OAuth2 access token for XOAUTH2 authentication (env: POP3ACCESSTOKEN)")
	f.String("authmethod", "auto", "Authentication method: USER, APOP, XOAUTH2, auto (env: POP3AUTHMETHOD)")

	// TLS
	f.Bool("starttls", false, "Force STLS usage (env: POP3STARTTLS)")
	f.Bool("pop3s", false, "Use POP3S (implicit TLS), typically on port 995 (env: POP3POP3S)")
	f.Bool("no-starttls", false, "Force plain connection: disable STLS; error if --starttls is also set (env: POP3NOSTARTTLS)")
	f.Bool("no-pop3s", false, "Force plain connection: error if --pop3s is also set (env: POP3NOPOP3S)")
	f.Bool("skipverify", false, "Skip TLS certificate verification (insecure) (env: POP3SKIPVERIFY)")
	f.String("tlsversion", "1.2", "TLS version to use (exact): 1.2, 1.3 (env: POP3TLSVERSION)")

	// Network
	f.String("address", "", "Optional: connect to this IP/hostname instead of --host (e.g. to test a specific server behind a load balancer); --host is still used for SNI, certificate checks, and authentication (env: POP3ADDRESS)")
	f.Bool("ipv4", false, "Force IPv4: resolve --host/--address to an A record and connect over IPv4 (env: POP3IPV4)")
	f.Bool("ipv6", false, "Force IPv6: resolve --host/--address to an AAAA record and connect over IPv6 (env: POP3IPV6)")
	f.String("proxy", "", "HTTP/HTTPS proxy URL (env: POP3PROXY)")
	f.Int("maxretries", 3, "Maximum retry attempts (env: POP3MAXRETRIES)")
	f.Int("retrydelay", 2000, "Retry delay in milliseconds (env: POP3RETRYDELAY)")
	f.Float64("ratelimit", 0, "Maximum POP3 requests per second (0 = unlimited) (env: POP3RATELIMIT)")

	// Output
	f.Bool("verbose", false, "Enable verbose output (env: POP3VERBOSE)")
	f.String("loglevel", "INFO", "Logging level: DEBUG, INFO, WARN, ERROR (env: POP3LOGLEVEL)")
	f.String("output", "text", "Output format: text, json (env: POP3OUTPUT)")
	f.String("logformat", "csv", "Log file format: csv, json (env: POP3LOGFORMAT)")
}

// BindEnvs registers Viper environment variable bindings for all pop3 config keys.
// Must be called after RegisterPersistentFlags.
func BindEnvs(v *viper.Viper) {
	bindings := map[string]string{
		"host":        "POP3HOST",
		"port":        "POP3PORT",
		"timeout":     "POP3TIMEOUT",
		"username":    "POP3USERNAME",
		"password":    "POP3PASSWORD",
		"accesstoken": "POP3ACCESSTOKEN",
		"authmethod":  "POP3AUTHMETHOD",
		"starttls":    "POP3STARTTLS",
		"pop3s":       "POP3POP3S",
		"no-starttls": "POP3NOSTARTTLS",
		"no-pop3s":    "POP3NOPOP3S",
		"skipverify":  "POP3SKIPVERIFY",
		"tlsversion":  "POP3TLSVERSION",
		"address":     "POP3ADDRESS",
		"ipv4":        "POP3IPV4",
		"ipv6":        "POP3IPV6",
		"proxy":       "POP3PROXY",
		"maxretries":  "POP3MAXRETRIES",
		"retrydelay":  "POP3RETRYDELAY",
		"output":      "POP3OUTPUT",
		"logformat":   "POP3LOGFORMAT",
		"ratelimit":   "POP3RATELIMIT",
		"verbose":     "POP3VERBOSE",
		"loglevel":    "POP3LOGLEVEL",
		"maxmessages": "POP3MAXMESSAGES",
		"subject":     "POP3SUBJECT",
		"messageid":   "POP3MESSAGEID",
		"count":       "POP3COUNT",
		"exportdir":   "POP3EXPORTDIR",
	}
	for key, env := range bindings {
		_ = v.BindEnv(key, env)
	}
}

// ConfigFromViper reads all pop3 config values from the given Viper instance.
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

	maxMessages := v.GetInt("maxmessages")
	if maxMessages <= 0 {
		maxMessages = defaults.MaxMessages
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

	count := v.GetInt("count")
	if count <= 0 {
		count = defaults.Count
	}

	return &Config{
		Host:           v.GetString("host"),
		Port:           port,
		Timeout:        time.Duration(timeoutSec) * time.Second,
		Username:       v.GetString("username"),
		Password:       v.GetString("password"),
		AccessToken:    v.GetString("accesstoken"),
		AuthMethod:     authMethod,
		MaxMessages:    maxMessages,
		POP3S:          v.GetBool("pop3s"),
		StartTLS:       v.GetBool("starttls"),
		NoStartTLS:     v.GetBool("no-starttls"),
		NoPOP3S:        v.GetBool("no-pop3s"),
		SkipVerify:     v.GetBool("skipverify"),
		TLSVersion:     tlsVersion,
		ConnectAddress: v.GetString("address"),
		IPv4Only:       v.GetBool("ipv4"),
		IPv6Only:       v.GetBool("ipv6"),
		ProxyURL:       v.GetString("proxy"),
		MaxRetries:     maxRetries,
		RetryDelay:     time.Duration(retryDelayMs) * time.Millisecond,
		VerboseMode:    v.GetBool("verbose"),
		LogLevel:       logLevel,
		OutputFormat:   outputFormat,
		LogFormat:      logFormat,
		RateLimit:      v.GetFloat64("ratelimit"),
		Subject:        v.GetString("subject"),
		MessageID:      v.GetString("messageid"),
		Count:          count,
		ExportDir:      v.GetString("exportdir"),
	}
}

// validateConfiguration validates the configuration.
func validateConfiguration(config *Config) error {
	// Validate action
	validActions := []string{ActionTestConnect, ActionTestAuth, ActionListMail, ActionExportMessages}
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
		fmt.Println("║  The --skipverify flag disables TLS certificate validation.   ║")
		fmt.Println("║  This makes the connection vulnerable to man-in-the-middle    ║")
		fmt.Println("║  attacks. Only use this for testing with self-signed certs.   ║")
		fmt.Println("╚════════════════════════════════════════════════════════════════╝")
		fmt.Println()
	}

	// Validate mutual exclusion: --pop3s and --starttls cannot be used together
	if config.POP3S && config.StartTLS {
		return fmt.Errorf("cannot use both --pop3s and --starttls flags simultaneously")
	}

	// Validate mutual exclusion: --no-pop3s/--no-starttls cannot be combined
	// with the flags they negate
	if config.POP3S && config.NoPOP3S {
		return fmt.Errorf("cannot use both --pop3s and --no-pop3s flags simultaneously")
	}
	if config.StartTLS && config.NoStartTLS {
		return fmt.Errorf("cannot use both --starttls and --no-starttls flags simultaneously")
	}

	// Smart port default: if --pop3s is set and port is 110 (default), change to 995
	if config.POP3S && config.Port == 110 {
		config.Port = 995
	}

	// Validate host (required for all actions)
	if config.Host == "" {
		return fmt.Errorf("host is required (--host flag)")
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

	// Validate mutual exclusion: --ipv4 and --ipv6 cannot be used together
	if err := network.ValidateIPVersionFlags(config.IPv4Only, config.IPv6Only); err != nil {
		return err
	}

	// Validate connect address (if provided)
	if config.ConnectAddress != "" {
		if err := validation.ValidateHostname(config.ConnectAddress); err != nil {
			return fmt.Errorf("invalid connect address: %w", err)
		}
	}

	// Action-specific validation
	switch config.Action {
	case ActionTestAuth, ActionListMail, ActionExportMessages:
		if config.Username == "" {
			return fmt.Errorf("%s requires --username", config.Action)
		}
		if strings.EqualFold(config.AuthMethod, "XOAUTH2") {
			if config.AccessToken == "" {
				return fmt.Errorf("XOAUTH2 authentication requires --accesstoken")
			}
			if config.Password != "" {
				fmt.Println("Warning: both --password and --accesstoken provided; --password will be ignored for XOAUTH2")
			}
		} else if config.AccessToken != "" {
			if config.Password != "" {
				fmt.Println("Warning: both --password and --accesstoken provided; --password will be ignored (using XOAUTH2)")
			}
		} else if config.Password == "" {
			return fmt.Errorf("%s requires --password (or --accesstoken for XOAUTH2)", config.Action)
		}
	}

	// Validate exportmessages-specific requirements
	if config.Action == ActionExportMessages {
		if config.MessageID == "" && strings.TrimSpace(config.Subject) == "" {
			return fmt.Errorf("exportmessages action requires --messageid and/or --subject parameter")
		}
	}

	return nil
}
