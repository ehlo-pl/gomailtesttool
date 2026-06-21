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
	ActionListMail    = "listmail"
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
		RateLimit:    0, // Unlimited by default
	}
}

// parseAndConfigureFlags parses command-line flags and environment variables.
func parseAndConfigureFlags() *Config {
	config := NewConfig()

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "POP3 Connectivity Testing Tool - Part of gomailtesttool suite\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Repository: https://github.com/ehlo-pl/gomailtesttool\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(flag.CommandLine.Output(), "\nEnvironment Variables:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  All flags can be set via environment variables with POP3 prefix\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  Example: POP3HOST, POP3PORT, POP3USERNAME\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Actions:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  testconnect   - Test TCP connection and capabilities\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  testauth      - Test authentication (USER/PASS, APOP, XOAUTH2)\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  listmail      - List messages in mailbox\n")
	}

	// Core flags
	showVersion := flag.Bool("version", false, "Show version information")
	action := flag.String("action", "", "Action to perform: testconnect, testauth, listmail (env: POP3ACTION)")

	// POP3 server configuration
	host := flag.String("host", "", "POP3 server hostname (env: POP3HOST)")
	port := flag.Int("port", 110, "POP3 server port (env: POP3PORT)")
	timeout := flag.Int("timeout", 30, "Connection timeout in seconds (env: POP3TIMEOUT)")

	// Authentication
	username := flag.String("username", "", "Username for authentication (env: POP3USERNAME)")
	password := flag.String("password", "", "Password for authentication (env: POP3PASSWORD)")
	accessToken := flag.String("accesstoken", "", "OAuth2 access token for XOAUTH2 (env: POP3ACCESSTOKEN)")
	authMethod := flag.String("authmethod", "auto", "Auth method: auto, USER, APOP, XOAUTH2 (env: POP3AUTHMETHOD)")

	// List options
	maxMessages := flag.Int("maxmessages", 100, "Maximum messages to list (env: POP3MAXMESSAGES)")

	// TLS configuration
	pop3s := flag.Bool("pop3s", false, "Use POP3S (implicit TLS on port 995) (env: POP3POP3S)")
	startTLS := flag.Bool("starttls", false, "Force STLS upgrade (env: POP3STARTTLS)")
	skipVerify := flag.Bool("skipverify", false, "Skip TLS certificate verification (env: POP3SKIPVERIFY)")
	tlsVersion := flag.String("tlsversion", "1.2", "TLS version: 1.2, 1.3 (env: POP3TLSVERSION)")

	// Network configuration
	proxyURL := flag.String("proxy", "", "Proxy URL (env: POP3PROXY)")
	maxRetries := flag.Int("maxretries", 3, "Maximum retry attempts (env: POP3MAXRETRIES)")
	retryDelay := flag.Int("retrydelay", 2000, "Retry delay in milliseconds (env: POP3RETRYDELAY)")

	// Runtime configuration
	verbose := flag.Bool("verbose", false, "Enable verbose output")
	logLevel := flag.String("loglevel", "INFO", "Log level: DEBUG, INFO, WARN, ERROR")
	output := flag.String("output", "text", "Output format: text, json (env: POP3OUTPUT)")
	logFormat := flag.String("logformat", "csv", "Log file format: csv, json (env: POP3LOGFORMAT)")
	rateLimit := flag.Float64("ratelimit", 0, "Rate limit (requests/second, 0=unlimited) (env: POP3RATELIMIT)")

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
	config.MaxMessages = *maxMessages
	config.POP3S = *pop3s
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
	if config.POP3S && config.Port == 110 {
		config.Port = 995
	}

	return config
}

// applyEnvOverrides applies environment variable overrides.
func applyEnvOverrides(config *Config) {
	if v := os.Getenv("POP3ACTION"); v != "" && config.Action == "" {
		config.Action = v
	}
	if v := os.Getenv("POP3HOST"); v != "" && config.Host == "" {
		config.Host = v
	}
	if v := os.Getenv("POP3PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			config.Port = port
		}
	}
	if v := os.Getenv("POP3TIMEOUT"); v != "" {
		if timeout, err := strconv.Atoi(v); err == nil {
			config.Timeout = time.Duration(timeout) * time.Second
		}
	}
	if v := os.Getenv("POP3USERNAME"); v != "" && config.Username == "" {
		config.Username = v
	}
	if v := os.Getenv("POP3PASSWORD"); v != "" && config.Password == "" {
		config.Password = v
	}
	if v := os.Getenv("POP3ACCESSTOKEN"); v != "" && config.AccessToken == "" {
		config.AccessToken = v
	}
	if v := os.Getenv("POP3AUTHMETHOD"); v != "" && config.AuthMethod == "auto" {
		config.AuthMethod = v
	}
	if v := os.Getenv("POP3MAXMESSAGES"); v != "" {
		if max, err := strconv.Atoi(v); err == nil {
			config.MaxMessages = max
		}
	}
	if parseBoolEnv("POP3POP3S") {
		config.POP3S = true
	}
	if parseBoolEnv("POP3STARTTLS") {
		config.StartTLS = true
	}
	if parseBoolEnv("POP3SKIPVERIFY") {
		config.SkipVerify = true
	}
	if v := os.Getenv("POP3TLSVERSION"); v != "" {
		config.TLSVersion = v
	}
	if v := os.Getenv("POP3PROXY"); v != "" && config.ProxyURL == "" {
		config.ProxyURL = v
	}
	if v := os.Getenv("POP3MAXRETRIES"); v != "" {
		if max, err := strconv.Atoi(v); err == nil {
			config.MaxRetries = max
		}
	}
	if v := os.Getenv("POP3RETRYDELAY"); v != "" {
		if delay, err := strconv.Atoi(v); err == nil {
			config.RetryDelay = time.Duration(delay) * time.Millisecond
		}
	}
	if v := os.Getenv("POP3OUTPUT"); v != "" {
		config.OutputFormat = v
	}
	if v := os.Getenv("POP3LOGFORMAT"); v != "" {
		config.LogFormat = v
	}
	if v := os.Getenv("POP3RATELIMIT"); v != "" {
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
	validActions := []string{ActionTestConnect, ActionTestAuth, ActionListMail}
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
	if config.POP3S && config.StartTLS {
		return fmt.Errorf("cannot use both -pop3s and -starttls; choose one")
	}

	// Action-specific validation
	switch config.Action {
	case ActionTestAuth, ActionListMail:
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
