package imap

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/ehlo-pl/gomailtesttool/internal/common/network"
	"github.com/ehlo-pl/gomailtesttool/internal/common/validation"
)

// Config holds all imaptool configuration.
type Config struct {
	// Core configuration
	ShowVersion bool
	Action      string

	// IMAP server configuration
	Host    string
	Port    int
	Timeout time.Duration

	// Authentication
	Username    string
	Password    string
	AccessToken string // OAuth2 access token for XOAUTH2 authentication
	AuthMethod  string // PLAIN, LOGIN, XOAUTH2, or "auto"

	// TLS configuration
	IMAPS      bool   // Use IMAPS (implicit TLS on port 993)
	StartTLS   bool   // Force STARTTLS
	NoStartTLS bool   // Force plain connection: disable STARTTLS; error if --starttls is also set
	NoIMAPS    bool   // Force plain connection: error if --imaps is also set
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
	Mailbox   string // Mailbox to search (default: INBOX)
	Count     int    // Maximum number of matching messages to export
	ExportDir string // Directory under which to create the dated export folder (default: OS temp dir)
}

// Action constants
const (
	ActionTestConnect    = "testconnect"
	ActionTestAuth       = "testauth"
	ActionListFolders    = "listfolders"
	ActionExportMessages = "exportmessages"
)

// NewConfig creates a new Config with default values.
func NewConfig() *Config {
	return &Config{
		Port:         143,
		Timeout:      30 * time.Second,
		AuthMethod:   "auto",
		IMAPS:        false,
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
		Mailbox:      "INBOX",
		Count:        25,
	}
}

// RegisterPersistentFlags registers flags shared by all imap subcommands
// on the given parent command (the "imap" cobra.Command).
func RegisterPersistentFlags(cmd *cobra.Command) {
	f := cmd.PersistentFlags()

	// IMAP server
	f.String("host", "", "IMAP server hostname (required) — the service to connect to; also used for TLS SNI/certificate checks and authentication (env: IMAPHOST)")
	f.Int("port", 143, "IMAP server port (env: IMAPPORT)")
	f.Int("timeout", 30, "Connection timeout in seconds (env: IMAPTIMEOUT)")

	// Authentication
	f.String("username", "", "Username for authentication (env: IMAPUSERNAME)")
	f.String("password", "", "Password for authentication (env: IMAPPASSWORD)")
	f.String("accesstoken", "", "OAuth2 access token for XOAUTH2 authentication (env: IMAPACCESSTOKEN)")
	f.String("authmethod", "auto", "Authentication method: PLAIN, LOGIN, XOAUTH2, auto (env: IMAPAUTHMETHOD)")

	// TLS
	f.Bool("starttls", false, "Force STARTTLS usage (env: IMAPSTARTTLS)")
	f.Bool("imaps", false, "Use IMAPS (implicit TLS), typically on port 993 (env: IMAPIMAPS)")
	f.Bool("no-starttls", false, "Force plain connection: disable STARTTLS; error if --starttls is also set (env: IMAPNOSTARTTLS)")
	f.Bool("no-imaps", false, "Force plain connection: error if --imaps is also set (env: IMAPNOIMAPS)")
	f.Bool("skipverify", false, "Skip TLS certificate verification (insecure) (env: IMAPSKIPVERIFY)")
	f.String("tlsversion", "1.2", "TLS version to use (exact): 1.2, 1.3 (env: IMAPTLSVERSION)")

	// Network
	f.String("address", "", "Optional: connect to this IP/hostname instead of --host (e.g. to test a specific server behind a load balancer); --host is still used for SNI, certificate checks, and authentication (env: IMAPADDRESS)")
	f.Bool("ipv4", false, "Force IPv4: resolve --host/--address to an A record and connect over IPv4 (env: IMAPIPV4)")
	f.Bool("ipv6", false, "Force IPv6: resolve --host/--address to an AAAA record and connect over IPv6 (env: IMAPIPV6)")
	f.String("proxy", "", "HTTP/HTTPS proxy URL (env: IMAPPROXY)")
	f.Int("maxretries", 3, "Maximum retry attempts (env: IMAPMAXRETRIES)")
	f.Int("retrydelay", 2000, "Retry delay in milliseconds (env: IMAPRETRYDELAY)")
	f.Float64("ratelimit", 0, "Maximum IMAP requests per second (0 = unlimited) (env: IMAPRATELIMIT)")

	// Output
	f.Bool("verbose", false, "Enable verbose output")
	f.String("loglevel", "INFO", "Logging level: DEBUG, INFO, WARN, ERROR")
	f.String("output", "text", "Output format: text, json (env: IMAPOUTPUT)")
	f.String("logformat", "csv", "Log file format: csv, json (env: IMAPLOGFORMAT)")
}

// BindEnvs registers Viper environment variable bindings for all imap config keys.
// Must be called after RegisterPersistentFlags.
func BindEnvs(v *viper.Viper) {
	bindings := map[string]string{
		"host":        "IMAPHOST",
		"port":        "IMAPPORT",
		"timeout":     "IMAPTIMEOUT",
		"username":    "IMAPUSERNAME",
		"password":    "IMAPPASSWORD",
		"accesstoken": "IMAPACCESSTOKEN",
		"authmethod":  "IMAPAUTHMETHOD",
		"starttls":    "IMAPSTARTTLS",
		"imaps":       "IMAPIMAPS",
		"no-starttls": "IMAPNOSTARTTLS",
		"no-imaps":    "IMAPNOIMAPS",
		"skipverify":  "IMAPSKIPVERIFY",
		"tlsversion":  "IMAPTLSVERSION",
		"address":     "IMAPADDRESS",
		"ipv4":        "IMAPIPV4",
		"ipv6":        "IMAPIPV6",
		"proxy":       "IMAPPROXY",
		"maxretries":  "IMAPMAXRETRIES",
		"retrydelay":  "IMAPRETRYDELAY",
		"output":      "IMAPOUTPUT",
		"logformat":   "IMAPLOGFORMAT",
		"ratelimit":   "IMAPRATELIMIT",
		"subject":     "IMAPSUBJECT",
		"messageid":   "IMAPMESSAGEID",
		"mailbox":     "IMAPMAILBOX",
		"count":       "IMAPCOUNT",
		"exportdir":   "IMAPEXPORTDIR",
	}
	for key, env := range bindings {
		_ = v.BindEnv(key, env)
	}
}

// ConfigFromViper reads all imap config values from the given Viper instance.
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

	mailbox := v.GetString("mailbox")
	if mailbox == "" {
		mailbox = defaults.Mailbox
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
		IMAPS:          v.GetBool("imaps"),
		StartTLS:       v.GetBool("starttls"),
		NoStartTLS:     v.GetBool("no-starttls"),
		NoIMAPS:        v.GetBool("no-imaps"),
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
		Mailbox:        mailbox,
		Count:          count,
		ExportDir:      v.GetString("exportdir"),
	}
}

// validateConfiguration validates the configuration.
func validateConfiguration(config *Config) error {
	// Validate action
	validActions := []string{ActionTestConnect, ActionTestAuth, ActionListFolders, ActionExportMessages}
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

	// Validate mutual exclusion: --imaps and --starttls cannot be used together
	if config.IMAPS && config.StartTLS {
		return fmt.Errorf("cannot use both --imaps and --starttls flags simultaneously")
	}

	// Validate mutual exclusion: --no-imaps/--no-starttls cannot be combined
	// with the flags they negate
	if config.IMAPS && config.NoIMAPS {
		return fmt.Errorf("cannot use both --imaps and --no-imaps flags simultaneously")
	}
	if config.StartTLS && config.NoStartTLS {
		return fmt.Errorf("cannot use both --starttls and --no-starttls flags simultaneously")
	}

	// Smart port default: if --imaps is set and port is 143 (default), change to 993
	if config.IMAPS && config.Port == 143 {
		config.Port = 993
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
	case ActionTestAuth, ActionListFolders, ActionExportMessages:
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
