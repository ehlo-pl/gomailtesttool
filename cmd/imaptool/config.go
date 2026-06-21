package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gomailtesttool/internal/common/validation"
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
	SkipVerify bool   // Skip TLS certificate verification
	TLSVersion string // TLS version to use: 1.2, 1.3

	// Network configuration
	ProxyURL   string
	MaxRetries int
	RetryDelay time.Duration

	// Runtime configuration
	VerboseMode  bool
	LogLevel     string
	OutputFormat string
	LogFormat    string  // Log file format: csv, json
	RateLimit    float64 // Maximum requests per second (0 = unlimited)
}

// Action constants
const (
	ActionTestConnect = "testconnect"
	ActionTestAuth    = "testauth"
	ActionListFolders = "listfolders"
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
		RateLimit:    0, // Unlimited by default
	}
}

// parseAndConfigureFlags parses command-line flags and environment variables.
func parseAndConfigureFlags() *Config {
	config := NewConfig()

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "IMAP Connectivity Testing Tool - Part of gomailtesttool suite\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Repository: https://github.com/ehlo-pl/gomailtesttool\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(flag.CommandLine.Output(), "\nEnvironment Variables:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  All flags can be set via environment variables with IMAP prefix\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  Example: IMAPHOST, IMAPPORT, IMAPUSERNAME\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Actions:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  testconnect   - Test TCP connection and capabilities\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  testauth      - Test authentication (PLAIN, LOGIN, XOAUTH2)\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  listfolders   - List mailbox folders\n")
	}

	// Core flags
	showVersion := flag.Bool("version", false, "Show version information")
	action := flag.String("action", "", "Action to perform: testconnect, testauth, listfolders (env: IMAPACTION)")

	// IMAP server configuration
	host := flag.String("host", "", "IMAP server hostname (env: IMAPHOST)")
	port := flag.Int("port", 143, "IMAP server port (env: IMAPPORT)")
	timeout := flag.Int("timeout", 30, "Connection timeout in seconds (env: IMAPTIMEOUT)")

	// Authentication
	username := flag.String("username", "", "Username for authentication (env: IMAPUSERNAME)")
	password := flag.String("password", "", "Password for authentication (env: IMAPPASSWORD)")
	accessToken := flag.String("accesstoken", "", "OAuth2 access token for XOAUTH2 (env: IMAPACCESSTOKEN)")
	authMethod := flag.String("authmethod", "auto", "Auth method: auto, PLAIN, LOGIN, XOAUTH2 (env: IMAPAUTHMETHOD)")

	// TLS configuration
	imaps := flag.Bool("imaps", false, "Use IMAPS (implicit TLS on port 993) (env: IMAPIMAPS)")
	startTLS := flag.Bool("starttls", false, "Force STARTTLS upgrade (env: IMAPSTARTTLS)")
	skipVerify := flag.Bool("skipverify", false, "Skip TLS certificate verification (env: IMAPSKIPVERIFY)")
	tlsVersion := flag.String("tlsversion", "1.2", "TLS version: 1.2, 1.3 (env: IMAPTLSVERSION)")

	// Network configuration
	proxyURL := flag.String("proxy", "", "Proxy URL (env: IMAPPROXY)")
	maxRetries := flag.Int("maxretries", 3, "Maximum retry attempts (env: IMAPMAXRETRIES)")
	retryDelay := flag.Int("retrydelay", 2000, "Retry delay in milliseconds (env: IMAPRETRYDELAY)")

	// Runtime configuration
	verbose := flag.Bool("verbose", false, "Enable verbose output")
	logLevel := flag.String("loglevel", "INFO", "Log level: DEBUG, INFO, WARN, ERROR")
	output := flag.String("output", "text", "Output format: text, json (env: IMAPOUTPUT)")
	logFormat := flag.String("logformat", "csv", "Log file format: csv, json (env: IMAPLOGFORMAT)")
	rateLimit := flag.Float64("ratelimit", 0, "Rate limit (requests/second, 0=unlimited) (env: IMAPRATELIMIT)")

	flag.Parse()

	// Apply flag values
	config.ShowVersion = *showVersion
	config.Action = *action
	config.Host = *host
	config.Port = *port
	config.Timeout = time.Duration(*timeout) * time.Second
	config.Username = *username
	config.Password = *password
	config.AccessToken = *accessToken
	config.AuthMethod = *authMethod
	config.IMAPS = *imaps
	config.StartTLS = *startTLS
	config.SkipVerify = *skipVerify
	config.TLSVersion = *tlsVersion
	config.ProxyURL = *proxyURL
	config.MaxRetries = *maxRetries
	config.RetryDelay = time.Duration(*retryDelay) * time.Millisecond
	config.VerboseMode = *verbose
	config.LogLevel = *logLevel
	config.OutputFormat = *output
	config.LogFormat = *logFormat
	config.RateLimit = *rateLimit

	// Apply environment variables (override defaults if flags not set)
	applyEnvOverrides(config)

	// Smart port defaults
	if config.IMAPS && config.Port == 143 {
		config.Port = 993
	}

	return config
}

// applyEnvOverrides applies environment variable overrides.
func applyEnvOverrides(config *Config) {
	if v := os.Getenv("IMAPACTION"); v != "" && config.Action == "" {
		config.Action = v
	}
	if v := os.Getenv("IMAPHOST"); v != "" && config.Host == "" {
		config.Host = v
	}
	if v := os.Getenv("IMAPPORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			config.Port = port
		}
	}
	if v := os.Getenv("IMAPTIMEOUT"); v != "" {
		if timeout, err := strconv.Atoi(v); err == nil {
			config.Timeout = time.Duration(timeout) * time.Second
		}
	}
	if v := os.Getenv("IMAPUSERNAME"); v != "" && config.Username == "" {
		config.Username = v
	}
	if v := os.Getenv("IMAPPASSWORD"); v != "" && config.Password == "" {
		config.Password = v
	}
	if v := os.Getenv("IMAPACCESSTOKEN"); v != "" && config.AccessToken == "" {
		config.AccessToken = v
	}
	if v := os.Getenv("IMAPAUTHMETHOD"); v != "" && config.AuthMethod == "auto" {
		config.AuthMethod = v
	}
	if parseBoolEnv("IMAPIMAPS") {
		config.IMAPS = true
	}
	if parseBoolEnv("IMAPSTARTTLS") {
		config.StartTLS = true
	}
	if parseBoolEnv("IMAPSKIPVERIFY") {
		config.SkipVerify = true
	}
	if v := os.Getenv("IMAPTLSVERSION"); v != "" {
		config.TLSVersion = v
	}
	if v := os.Getenv("IMAPPROXY"); v != "" && config.ProxyURL == "" {
		config.ProxyURL = v
	}
	if v := os.Getenv("IMAPMAXRETRIES"); v != "" {
		if max, err := strconv.Atoi(v); err == nil {
			config.MaxRetries = max
		}
	}
	if v := os.Getenv("IMAPRETRYDELAY"); v != "" {
		if delay, err := strconv.Atoi(v); err == nil {
			config.RetryDelay = time.Duration(delay) * time.Millisecond
		}
	}
	if v := os.Getenv("IMAPOUTPUT"); v != "" {
		config.OutputFormat = v
	}
	if v := os.Getenv("IMAPLOGFORMAT"); v != "" {
		config.LogFormat = v
	}
	if v := os.Getenv("IMAPRATELIMIT"); v != "" {
		if rate, err := strconv.ParseFloat(v, 64); err == nil {
			config.RateLimit = rate
		}
	}
}

// parseBoolEnv parses a boolean environment variable.
func parseBoolEnv(key string) bool {
	v := strings.ToLower(os.Getenv(key))
	return v == "true" || v == "1" || v == "yes" || v == "on"
}

// validateConfiguration validates the configuration.
func validateConfiguration(config *Config) error {
	// Validate action
	validActions := []string{ActionTestConnect, ActionTestAuth, ActionListFolders}
	actionValid := false
	for _, a := range validActions {
		if config.Action == a {
			actionValid = true
			break
		}
	}
	if !actionValid {
		return fmt.Errorf("invalid action: %s (valid: %s)", config.Action, strings.Join(validActions, ", "))
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

	// Validate host
	if config.Host == "" {
		return fmt.Errorf("host is required")
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

	// Validate mutual exclusion
	if config.IMAPS && config.StartTLS {
		return fmt.Errorf("cannot use both -imaps and -starttls; choose one")
	}

	// Action-specific validation
	switch config.Action {
	case ActionTestAuth, ActionListFolders:
		if config.Username == "" {
			return fmt.Errorf("%s requires -username", config.Action)
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
			if config.Password != "" {
				fmt.Println("Warning: both -password and -accesstoken provided; -password will be ignored (using XOAUTH2)")
			}
		} else if config.Password == "" {
			return fmt.Errorf("%s requires -password (or -accesstoken for XOAUTH2)", config.Action)
		}
	}

	return nil
}
