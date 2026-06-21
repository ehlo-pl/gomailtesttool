package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gomailtesttool/internal/common/version"
)

// Config holds all configuration for jmaptool.
type Config struct {
	// Connection settings
	Host        string
	Port        int
	Username    string
	Password    string
	AccessToken string

	// Action
	Action string

	// Authentication
	AuthMethod string // auto, basic, bearer

	// TLS settings
	SkipVerify bool

	// Logging
	VerboseMode bool
	LogLevel    string
	LogFormat   string

	// Other
	ShowVersion bool
}

// NewConfig creates a new Config with sensible default values.
func NewConfig() *Config {
	return &Config{
		Port:       443,
		AuthMethod: "auto",
		LogLevel:   "info",
		LogFormat:  "csv",
	}
}

// parseAndConfigureFlags parses command-line flags and environment variables.
func parseAndConfigureFlags() *Config {
	config := NewConfig()

	// Define flags
	flag.StringVar(&config.Action, "action", "", "Action to perform: testconnect, testauth, getmailboxes (env: JMAPACTION)")
	flag.StringVar(&config.Host, "host", "", "JMAP server hostname (env: JMAPHOST)")
	flag.IntVar(&config.Port, "port", 443, "JMAP server port (default: 443) (env: JMAPPORT)")
	flag.StringVar(&config.Username, "username", "", "Username for authentication (env: JMAPUSERNAME)")
	flag.StringVar(&config.Password, "password", "", "Password for authentication (env: JMAPPASSWORD)")
	flag.StringVar(&config.AccessToken, "accesstoken", "", "Access token for Bearer authentication (env: JMAPACCESSTOKEN)")
	flag.StringVar(&config.AuthMethod, "authmethod", "auto", "Authentication method: auto, basic, bearer (env: JMAPAUTHMETHOD)")
	flag.BoolVar(&config.SkipVerify, "skipverify", false, "Skip TLS certificate verification (env: JMAPSKIPVERIFY)")
	flag.BoolVar(&config.VerboseMode, "verbose", false, "Enable verbose output (env: JMAPVERBOSE)")
	flag.StringVar(&config.LogLevel, "loglevel", "info", "Log level: debug, info, warn, error (env: JMAPLOGLEVEL)")
	flag.StringVar(&config.LogFormat, "logformat", "csv", "Log format: csv, json (env: JMAPLOGFORMAT)")
	flag.BoolVar(&config.ShowVersion, "version", false, "Show version information")

	// Custom usage
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "jmaptool - JMAP Testing Tool - Version %s\n\n", version.Get())
		fmt.Fprintf(os.Stderr, "JMAP testing tool for testing JMAP server connectivity and operations.\n\n")
		fmt.Fprintf(os.Stderr, "Actions:\n")
		fmt.Fprintf(os.Stderr, "  testconnect   Test JMAP server connectivity and discover session\n")
		fmt.Fprintf(os.Stderr, "  testauth      Test authentication\n")
		fmt.Fprintf(os.Stderr, "  getmailboxes  Get list of mailboxes\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEnvironment variables:\n")
		fmt.Fprintf(os.Stderr, "  JMAPHOST        Server hostname\n")
		fmt.Fprintf(os.Stderr, "  JMAPPORT        Server port\n")
		fmt.Fprintf(os.Stderr, "  JMAPUSERNAME    Username\n")
		fmt.Fprintf(os.Stderr, "  JMAPPASSWORD    Password\n")
		fmt.Fprintf(os.Stderr, "  JMAPACCESSTOKEN Access token\n")
		fmt.Fprintf(os.Stderr, "  JMAPAUTHMETHOD  Authentication method\n")
		fmt.Fprintf(os.Stderr, "  JMAPSKIPVERIFY  Skip TLS verification (true/false)\n")
		fmt.Fprintf(os.Stderr, "  JMAPVERBOSE     Verbose output (true/false)\n")
		fmt.Fprintf(os.Stderr, "  JMAPLOGLEVEL    Log level\n")
		fmt.Fprintf(os.Stderr, "  JMAPLOGFORMAT   Log format\n")
		fmt.Fprintf(os.Stderr, "  JMAPACTION      Action to perform\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  jmaptool -action testconnect -host jmap.fastmail.com\n")
		fmt.Fprintf(os.Stderr, "  jmaptool -action testauth -host jmap.fastmail.com -username user@example.com -accesstoken \"token\"\n")
		fmt.Fprintf(os.Stderr, "  jmaptool -action getmailboxes -host jmap.fastmail.com -username user@example.com -accesstoken \"token\"\n")
	}

	flag.Parse()

	// Track which flags were explicitly set via command line
	providedFlags := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		providedFlags[f.Name] = true
	})

	// Read from environment variables if not set via flags
	// Note: Using JMAP* prefix (no underscore) for consistency with other tools
	if !providedFlags["host"] {
		if envHost := os.Getenv("JMAPHOST"); envHost != "" {
			config.Host = envHost
		}
	}
	if !providedFlags["port"] {
		if envPort := os.Getenv("JMAPPORT"); envPort != "" {
			if port, err := strconv.Atoi(envPort); err == nil && port > 0 && port < 65536 {
				config.Port = port
			}
		}
	}
	if !providedFlags["username"] {
		if envUsername := os.Getenv("JMAPUSERNAME"); envUsername != "" {
			config.Username = envUsername
		}
	}
	if !providedFlags["password"] {
		if envPassword := os.Getenv("JMAPPASSWORD"); envPassword != "" {
			config.Password = envPassword
		}
	}
	if !providedFlags["accesstoken"] {
		if envToken := os.Getenv("JMAPACCESSTOKEN"); envToken != "" {
			config.AccessToken = envToken
		}
	}
	if !providedFlags["authmethod"] {
		if envAuthMethod := os.Getenv("JMAPAUTHMETHOD"); envAuthMethod != "" {
			config.AuthMethod = envAuthMethod
		}
	}
	if !providedFlags["skipverify"] {
		if envSkipVerify := os.Getenv("JMAPSKIPVERIFY"); envSkipVerify != "" {
			config.SkipVerify = strings.EqualFold(envSkipVerify, "true") || envSkipVerify == "1"
		}
	}
	if !providedFlags["verbose"] {
		if envVerbose := os.Getenv("JMAPVERBOSE"); envVerbose != "" {
			config.VerboseMode = strings.EqualFold(envVerbose, "true") || envVerbose == "1"
		}
	}
	if !providedFlags["loglevel"] {
		if envLogLevel := os.Getenv("JMAPLOGLEVEL"); envLogLevel != "" {
			config.LogLevel = envLogLevel
		}
	}
	if !providedFlags["logformat"] {
		if envLogFormat := os.Getenv("JMAPLOGFORMAT"); envLogFormat != "" {
			config.LogFormat = envLogFormat
		}
	}
	if !providedFlags["action"] {
		if envAction := os.Getenv("JMAPACTION"); envAction != "" {
			config.Action = envAction
		}
	}

	return config
}

// validateConfiguration validates the configuration.
func validateConfiguration(config *Config) error {
	// Validate action
	validActions := map[string]bool{
		"testconnect":  true,
		"testauth":     true,
		"getmailboxes": true,
	}

	action := strings.ToLower(config.Action)
	if !validActions[action] {
		return fmt.Errorf("invalid action: %s (valid: testconnect, testauth, getmailboxes)", config.Action)
	}
	config.Action = action

	// Validate host
	if config.Host == "" {
		return fmt.Errorf("host is required")
	}

	// Validate port
	if config.Port <= 0 || config.Port > 65535 {
		return fmt.Errorf("invalid port: %d (must be 1-65535)", config.Port)
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

	// Validate credentials for auth actions
	if config.Action == "testauth" || config.Action == "getmailboxes" {
		if config.AccessToken == "" && config.Password == "" {
			return fmt.Errorf("either password or accesstoken is required for %s", config.Action)
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
