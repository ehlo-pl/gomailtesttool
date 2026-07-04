package smtp

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/ehlo-pl/gomailtesttool/internal/common/email"
	"github.com/ehlo-pl/gomailtesttool/internal/common/network"
	"github.com/ehlo-pl/gomailtesttool/internal/common/validation"
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
	AccessToken string // OAuth2 bearer token for XOAUTH2 or OAUTHBEARER authentication
	AuthMethod  string // PLAIN, LOGIN, CRAM-MD5, NTLM, GSSAPI, XOAUTH2, OAUTHBEARER, or "auto"
	Realm       string // Kerberos realm for GSSAPI (auto-extracted from user@REALM if empty)
	KDCAddress  string // KDC host:port override for GSSAPI (uses DNS SRV if empty)

	// Email configuration (for sendmail)
	From              string
	To                []string
	Cc                []string // CC recipients: added to the SMTP envelope (RCPT TO) and the Cc: header
	Bcc               []string // BCC recipients: added to the SMTP envelope (RCPT TO) only, never to message headers
	Subject           string
	Body              string
	BodyHTML          string   // HTML body content; if set alongside Body, a multipart/alternative message is sent
	Attachments       []string // File paths to attach to email
	InlineAttachments []string // File paths to embed inline via cid: (referenced from BodyHTML)
	Headers           []string // Custom headers in "Name: Value" form
	Priority          string   // Email priority: high, normal, low (normal adds no extra headers)

	// TLS configuration
	StartTLS   bool   // Force STARTTLS
	SMTPS      bool   // Use SMTPS (implicit TLS on port 465)
	NoStartTLS bool   // Force plain connection: disable STARTTLS (incl. automatic upgrade)
	NoSMTPS    bool   // Force plain connection: error if --smtps is also set
	SkipVerify bool   // Skip TLS certificate verification
	TLSVersion string // TLS version to use (exact match): 1.2, 1.3

	// Network configuration
	ConnectAddress string // Override address for TCP connection (IP or hostname)
	IPv4Only       bool   // Force resolving --host/--address to an IPv4 (A record) address
	IPv6Only       bool   // Force resolving --host/--address to an IPv6 (AAAA record) address
	UseMX          bool   // Treat --host as a domain and connect to its MX record instead
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
		Priority:     "normal",
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
	f.String("host", "", "SMTP server hostname (required) — the service to connect to; also used for TLS SNI/certificate checks and authentication (env: SMTPHOST)")
	f.Int("port", 25, "SMTP server port (env: SMTPPORT)")
	f.Int("timeout", 30, "Connection timeout in seconds (env: SMTPTIMEOUT)")

	// Authentication
	f.String("username", "", "SMTP username for authentication (env: SMTPUSERNAME)")
	f.String("password", "", "SMTP password for authentication (env: SMTPPASSWORD)")
	f.String("accesstoken", "", "OAuth2 bearer token for XOAUTH2 or OAUTHBEARER authentication (env: SMTPACCESSTOKEN)")
	f.String("authmethod", "auto", "Authentication method: PLAIN, LOGIN, CRAM-MD5, NTLM, GSSAPI, XOAUTH2, OAUTHBEARER, auto (env: SMTPAUTHMETHOD)")
	f.String("realm", "", "Kerberos realm for GSSAPI (auto-extracted from user@REALM if omitted) (env: SMTPREALM)")
	f.String("kdc", "", "KDC address override for GSSAPI (host or host:port; uses DNS SRV if omitted) (env: SMTPKDC)")

	// TLS
	f.Bool("starttls", false, "Force STARTTLS usage (env: SMTPSTARTTLS)")
	f.Bool("smtps", false, "Use SMTPS (implicit TLS), typically on port 465 (env: SMTPSMTPS)")
	f.Bool("no-starttls", false, "Force plain connection: disable STARTTLS (including automatic upgrade) even if the server advertises it; error if --starttls is also set (env: SMTPNOSTARTTLS)")
	f.Bool("no-smtps", false, "Force plain connection: error if --smtps is also set (env: SMTPNOSMTPS)")
	f.Bool("skipverify", false, "Skip TLS certificate verification (insecure) (env: SMTPSKIPVERIFY)")
	f.String("tlsversion", "1.2", "TLS version to use (exact): 1.2, 1.3 (env: SMTPTLSVERSION)")

	// Network
	f.String("address", "", "Optional: connect to this IP/hostname instead of --host (e.g. to test a specific server behind a load balancer); --host is still used for SNI, certificate checks, and authentication (env: SMTPADDRESS)")
	f.Bool("ipv4", false, "Force IPv4: resolve --host/--address to an A record and connect over IPv4 (env: SMTPIPV4)")
	f.Bool("ipv6", false, "Force IPv6: resolve --host/--address to an AAAA record and connect over IPv6 (env: SMTPIPV6)")
	f.Bool("use-mx", false, "Treat --host as a domain name and connect to its MX record instead (env: SMTPUSEMX)")
	f.String("proxy", "", "HTTP/HTTPS proxy URL (env: SMTPPROXY)")
	f.Int("maxretries", 3, "Maximum retry attempts (env: SMTPMAXRETRIES)")
	f.Int("retrydelay", 2000, "Retry delay in milliseconds (env: SMTPRETRYDELAY)")
	f.Float64("ratelimit", 0, "Maximum SMTP requests per second (0 = unlimited) (env: SMTPRATELIMIT)")

	// Output
	f.Bool("verbose", false, "Enable verbose output (env: SMTPVERBOSE)")
	f.String("loglevel", "INFO", "Logging level: DEBUG, INFO, WARN, ERROR (env: SMTPLOGLEVEL)")
	f.String("output", "text", "Output format: text, json (env: SMTPOUTPUT)")
	f.String("logformat", "csv", "Log file format: csv, json (env: SMTPLOGFORMAT)")
}

// BindEnvs registers Viper environment variable bindings for all smtp config keys.
// Must be called after RegisterPersistentFlags.
func BindEnvs(v *viper.Viper) {
	bindings := map[string]string{
		"host":              "SMTPHOST",
		"port":              "SMTPPORT",
		"timeout":           "SMTPTIMEOUT",
		"username":          "SMTPUSERNAME",
		"password":          "SMTPPASSWORD",
		"accesstoken":       "SMTPACCESSTOKEN",
		"authmethod":        "SMTPAUTHMETHOD",
		"realm":             "SMTPREALM",
		"kdc":               "SMTPKDC",
		"from":              "SMTPFROM",
		"to":                "SMTPTO",
		"cc":                "SMTPCC",
		"bcc":               "SMTPBCC",
		"priority":          "SMTPPRIORITY",
		"bodyhtml":          "SMTPBODYHTML",
		"attachments":       "SMTPATTACHMENTS",
		"inlineattachments": "SMTPINLINEATTACHMENTS",
		"starttls":          "SMTPSTARTTLS",
		"smtps":             "SMTPSMTPS",
		"no-starttls":       "SMTPNOSTARTTLS",
		"no-smtps":          "SMTPNOSMTPS",
		"skipverify":        "SMTPSKIPVERIFY",
		"tlsversion":        "SMTPTLSVERSION",
		"address":           "SMTPADDRESS",
		"ipv4":              "SMTPIPV4",
		"ipv6":              "SMTPIPV6",
		"use-mx":            "SMTPUSEMX",
		"proxy":             "SMTPPROXY",
		"maxretries":        "SMTPMAXRETRIES",
		"retrydelay":        "SMTPRETRYDELAY",
		"output":            "SMTPOUTPUT",
		"logformat":         "SMTPLOGFORMAT",
		"ratelimit":         "SMTPRATELIMIT",
		"verbose":           "SMTPVERBOSE",
		"loglevel":          "SMTPLOGLEVEL",
		"header":            "SMTPHEADER",
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

	// Parse comma-separated lists
	toList := splitCommaSeparated(v.GetString("to"))
	ccList := splitCommaSeparated(v.GetString("cc"))
	bccList := splitCommaSeparated(v.GetString("bcc"))
	attachments := splitCommaSeparated(v.GetString("attachments"))
	inlineAttachments := splitCommaSeparated(v.GetString("inline-attachments"))

	subject := v.GetString("subject")
	if subject == "" {
		subject = defaults.Subject
	}

	body := v.GetString("body")
	if body == "" {
		body = defaults.Body
	}

	priority := strings.ToLower(v.GetString("priority"))
	if priority == "" {
		priority = defaults.Priority
	}

	return &Config{
		Host:              v.GetString("host"),
		Port:              port,
		Timeout:           time.Duration(timeoutSec) * time.Second,
		Username:          v.GetString("username"),
		Password:          v.GetString("password"),
		AccessToken:       v.GetString("accesstoken"),
		AuthMethod:        authMethod,
		Realm:             strings.ToUpper(v.GetString("realm")),
		KDCAddress:        v.GetString("kdc"),
		From:              v.GetString("from"),
		To:                toList,
		Cc:                ccList,
		Bcc:               bccList,
		Subject:           subject,
		Body:              body,
		BodyHTML:          v.GetString("bodyhtml"),
		Attachments:       attachments,
		InlineAttachments: inlineAttachments,
		Headers:           v.GetStringSlice("header"),
		Priority:          priority,
		StartTLS:          v.GetBool("starttls"),
		SMTPS:             v.GetBool("smtps"),
		NoStartTLS:        v.GetBool("no-starttls"),
		NoSMTPS:           v.GetBool("no-smtps"),
		SkipVerify:        v.GetBool("skipverify"),
		TLSVersion:        tlsVersion,
		ConnectAddress:    v.GetString("address"),
		IPv4Only:          v.GetBool("ipv4"),
		IPv6Only:          v.GetBool("ipv6"),
		UseMX:             v.GetBool("use-mx"),
		ProxyURL:          v.GetString("proxy"),
		MaxRetries:        maxRetries,
		RetryDelay:        time.Duration(retryDelayMs) * time.Millisecond,
		VerboseMode:       v.GetBool("verbose"),
		LogLevel:          logLevel,
		OutputFormat:      outputFormat,
		LogFormat:         logFormat,
		RateLimit:         v.GetFloat64("ratelimit"),
	}
}

// splitCommaSeparated splits a comma-separated string into a trimmed,
// non-empty list of values. Returns nil if the input is empty.
func splitCommaSeparated(s string) []string {
	var result []string
	for _, item := range strings.Split(s, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
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

	// Validate mutual exclusion: -no-smtps/-no-starttls cannot be combined
	// with the flags they negate
	if config.SMTPS && config.NoSMTPS {
		return fmt.Errorf("cannot use both -smtps and -no-smtps flags simultaneously")
	}
	if config.StartTLS && config.NoStartTLS {
		return fmt.Errorf("cannot use both -starttls and -no-starttls flags simultaneously")
	}

	// Smart port default: if -smtps is set and port is 25 (default), change to 465
	if config.SMTPS && config.Port == 25 {
		config.Port = 465
	}

	// Validate host (required for all actions, except sendmail with --use-mx,
	// where the MX lookup domain is derived from --to instead).
	if !(config.Action == ActionSendMail && config.UseMX) {
		if config.Host == "" {
			return fmt.Errorf("host is required (-host flag)")
		}
		if err := validation.ValidateHostname(config.Host); err != nil {
			return fmt.Errorf("invalid host: %w", err)
		}
	}

	// Validate port
	if err := validation.ValidatePort(config.Port); err != nil {
		return fmt.Errorf("invalid port: %w", err)
	}

	// Validate proxy URL (if provided)
	if err := validation.ValidateProxyURL(config.ProxyURL); err != nil {
		return fmt.Errorf("invalid proxy URL: %w", err)
	}

	// Validate mutual exclusion: -ipv4 and -ipv6 cannot be used together
	if err := network.ValidateIPVersionFlags(config.IPv4Only, config.IPv6Only); err != nil {
		return err
	}

	// Validate connect address (if provided)
	if config.ConnectAddress != "" {
		if err := validation.ValidateHostname(config.ConnectAddress); err != nil {
			return fmt.Errorf("invalid connect address: %w", err)
		}
	}

	// Validate mutual exclusion: -use-mx and -address cannot be used together
	if config.UseMX && config.ConnectAddress != "" {
		return fmt.Errorf("cannot use both -use-mx and -address simultaneously")
	}

	// Action-specific validation
	switch config.Action {
	case ActionTestStartTLS:
		if config.NoStartTLS && !config.SMTPS {
			return fmt.Errorf("teststarttls requires STARTTLS or -smtps; remove -no-starttls or use -smtps")
		}

	case ActionTestAuth:
		if config.Username == "" {
			return fmt.Errorf("testauth requires -username")
		}
		// Token-based mechanisms (XOAUTH2, OAUTHBEARER) require accesstoken instead of password
		if strings.EqualFold(config.AuthMethod, "XOAUTH2") || strings.EqualFold(config.AuthMethod, "OAUTHBEARER") {
			method := strings.ToUpper(config.AuthMethod)
			if config.AccessToken == "" {
				return fmt.Errorf("%s authentication requires -accesstoken", method)
			}
			if config.Password != "" {
				fmt.Printf("Warning: both -password and -accesstoken provided; -password will be ignored for %s\n", method)
			}
		} else if config.AccessToken != "" {
			// If accesstoken provided, a token-based mechanism (XOAUTH2/OAUTHBEARER) is used
			// No password required
			if config.Password != "" {
				fmt.Println("Warning: both -password and -accesstoken provided; -password will be ignored (using token-based auth)")
			}
		} else if config.Password == "" {
			return fmt.Errorf("testauth requires -password (or -accesstoken for XOAUTH2/OAUTHBEARER)")
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
		for _, addr := range config.To {
			if err := validation.ValidateEmail(strings.TrimSpace(addr)); err != nil {
				return fmt.Errorf("invalid recipient email: %w", err)
			}
		}

		// Validate mutual exclusion: -use-mx and -host cannot be used together
		// for sendmail; the MX lookup domain is derived from -to instead.
		if config.UseMX {
			if config.Host != "" {
				return fmt.Errorf("cannot use both -use-mx and -host simultaneously for sendmail")
			}
			domain, err := validation.ExtractEmailDomain(strings.TrimSpace(config.To[0]))
			if err != nil {
				return fmt.Errorf("cannot derive MX lookup domain from -to: %w", err)
			}
			if err := validation.ValidateHostname(domain); err != nil {
				return fmt.Errorf("invalid domain derived from -to recipient %q: %w", config.To[0], err)
			}
			config.Host = domain
		}
		for _, addr := range config.Cc {
			if err := validation.ValidateEmail(strings.TrimSpace(addr)); err != nil {
				return fmt.Errorf("invalid cc email: %w", err)
			}
		}
		for _, addr := range config.Bcc {
			if err := validation.ValidateEmail(strings.TrimSpace(addr)); err != nil {
				return fmt.Errorf("invalid bcc email: %w", err)
			}
		}
		if config.Subject == "" {
			return fmt.Errorf("sendmail requires -subject")
		}
		switch config.Priority {
		case "high", "normal", "low":
		default:
			return fmt.Errorf("invalid -priority: %s (must be one of: high, normal, low)", config.Priority)
		}
		for i, path := range config.Attachments {
			if err := validation.ValidateFilePath(path, fmt.Sprintf("Attachment file #%d", i+1)); err != nil {
				return err
			}
		}
		for i, path := range config.InlineAttachments {
			if err := validation.ValidateFilePath(path, fmt.Sprintf("Inline attachment file #%d", i+1)); err != nil {
				return err
			}
		}
		if _, err := email.ParseHeaders(config.Headers); err != nil {
			return fmt.Errorf("invalid -header: %w", err)
		}
	}

	return nil
}
