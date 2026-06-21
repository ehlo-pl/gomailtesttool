package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Config holds all application configuration including command-line flags,
// environment variables, and runtime state.
type Config struct {
	// Core configuration
	ShowVersion bool   // Display version information and exit
	TenantID    string // Azure AD Tenant ID (GUID format)
	ClientID    string // Application (Client) ID (GUID format)
	Mailbox     string // Target user email address
	Action      string // Operation to perform (getevents, sendmail, sendinvite, getinbox, getschedule)

	// Authentication configuration (mutually exclusive)
	Secret     string // Client Secret for authentication
	PfxPath    string // Path to .pfx certificate file
	PfxPass    string // Password for .pfx certificate file
	Thumbprint string // SHA1 thumbprint of certificate in Windows Certificate Store

	// Email recipients (using stringSlice type for comma-separated lists)
	To              stringSlice // To recipients for email
	Cc              stringSlice // CC recipients for email
	Bcc             stringSlice // BCC recipients for email
	AttachmentFiles stringSlice // File paths to attach to email

	// Email content
	Subject      string // Email subject line
	Body         string // Email body text content
	BodyHTML     string // Email body HTML content (future use)
	BodyTemplate string // Path to HTML email body template file

	// Calendar invite configuration
	InviteSubject string // Subject of calendar meeting invitation
	StartTime     string // Start time in RFC3339 format (e.g., 2026-01-15T14:00:00Z)
	EndTime       string // End time in RFC3339 format

	// Search configuration
	MessageID string // Internet Message ID for searchandexport action

	// Network configuration
	ProxyURL   string        // HTTP/HTTPS proxy URL (e.g., http://proxy.example.com:8080)
	MaxRetries int           // Maximum retry attempts for transient failures (default: 3)
	RetryDelay time.Duration // Base delay between retries in milliseconds (default: 2000ms)

	// Runtime configuration
	VerboseMode  bool   // Enable verbose diagnostic output (maps to DEBUG log level)
	LogLevel     string // Logging level: DEBUG, INFO, WARN, ERROR (default: INFO)
	OutputFormat string // Output format: text, json (default: text)
	WhatIf       bool   // Dry run mode - preview actions without executing (PowerShell-style)
	Count        int    // Number of items to retrieve (for getevents and getinbox actions)
}

// NewConfig creates a new Config with sensible default values.
// Command-line flags and environment variables will override these defaults.
func NewConfig() *Config {
	return &Config{
		// Default values for optional fields
		Subject:       "Automated Tool Notification",
		Body:          "It's a test message, please ignore",
		InviteSubject: "System Sync",
		Action:        ActionGetInbox,
		Count:         3,
		VerboseMode:   false,
		LogLevel:      "INFO",                  // Default: INFO level logging
		OutputFormat:  "text",                  // Default: text output
		ShowVersion:   false,
		MaxRetries:    3,                       // Default: 3 retry attempts
		RetryDelay:    2000 * time.Millisecond, // Default: 2 second base delay
	}
}

// parseAndConfigureFlags defines all command-line flags, parses them,
// applies environment variables, and returns a populated Config struct with
// all configuration values merged from defaults, environment variables, and
// command-line arguments (in that order of precedence).
func parseAndConfigureFlags() *Config {
	// Customize help output
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Microsoft Graph EXO Mails/Calendar Golang Testing Tool - Version %s\n\n", version)
		fmt.Fprintf(flag.CommandLine.Output(), "Repository: https://github.com/ehlo-pl/gomailtesttool\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(flag.CommandLine.Output(), "\nEnvironment Variables:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  All flags can be set via environment variables with MSGRAPH prefix\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  Example: MSGRAPHTENANTID, MSGRAPHCLIENTID, MSGRAPHSECRET\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  Command-line flags take precedence over environment variables\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Examples:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -tenantid \"...\" -clientid \"...\" -secret \"...\" -mailbox \"user@example.com\" -action getevents\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -tenantid \"...\" -clientid \"...\" -thumbprint \"ABC123\" -mailbox \"user@example.com\" -action sendmail\n\n", os.Args[0])
	}

	// Define Command Line Parameters
	showVersion := flag.Bool("version", false, "Show version information")
	tenantID := flag.String("tenantid", "", "The Azure Tenant ID (env: MSGRAPHTENANTID)")
	clientID := flag.String("clientid", "", "The Application (Client) ID (env: MSGRAPHCLIENTID)")
	secret := flag.String("secret", "", "The Client Secret (env: MSGRAPHSECRET)")
	pfxPath := flag.String("pfx", "", "Path to the .pfx certificate file (env: MSGRAPHPFX)")
	pfxPass := flag.String("pfxpass", "", "Password for the .pfx file (env: MSGRAPHPFXPASS)")
	thumbprint := flag.String("thumbprint", "", "Thumbprint of the certificate in the CurrentUser\\My store (env: MSGRAPHTHUMBPRINT)")
	mailbox := flag.String("mailbox", "", "The target EXO mailbox email address (env: MSGRAPHMAILBOX)")

	// Recipient flags (using custom stringSlice type)
	var to, cc, bcc, attachmentFiles stringSlice
	flag.Var(&to, "to", "Comma-separated list of TO recipients (optional, defaults to mailbox if empty) (env: MSGRAPHTO)")
	flag.Var(&cc, "cc", "Comma-separated list of CC recipients (env: MSGRAPHCC)")
	flag.Var(&bcc, "bcc", "Comma-separated list of BCC recipients (env: MSGRAPHBCC)")

	// Email content flags
	subject := flag.String("subject", "Automated Tool Notification", "Subject of the email or calendar invite (env: MSGRAPHSUBJECT)")
	body := flag.String("body", "It's a test message, please ignore", "Body content of the email (text) (env: MSGRAPHBODY)")
	bodyHTML := flag.String("bodyHTML", "", "HTML body content of the email (optional, creates multipart message if both -body and -bodyHTML are provided) (env: MSGRAPHBODYHTML)")
	bodyTemplate := flag.String("body-template", "", "Path to HTML email body template file (env: MSGRAPHBODYTEMPLATE)")
	flag.Var(&attachmentFiles, "attachments", "Comma-separated list of file paths to attach (env: MSGRAPHATTACHMENTS)")

	// Calendar invite flags
	inviteSubject := flag.String("invite-subject", "", "")  // Deprecated: use -subject instead
	startTime := flag.String("start", "", "Start time for calendar invite (RFC3339 or PowerShell 'Get-Date -Format s' format). Examples: '2026-01-15T14:00:00Z', '2026-01-15T14:00:00'. Defaults to now if empty (env: MSGRAPHSTART)")
	endTime := flag.String("end", "", "End time for calendar invite (RFC3339 or PowerShell 'Get-Date -Format s' format). Examples: '2026-01-15T15:00:00Z', '2026-01-15T15:00:00'. Defaults to 1 hour after start if empty (env: MSGRAPHEND)")

	// Search flags
	messageID := flag.String("messageid", "", "Internet Message ID for searchandexport action (env: MSGRAPHMESSAGEID)")

	// Proxy configuration
	proxyURL := flag.String("proxy", "", "HTTP/HTTPS proxy URL (e.g., http://proxy.example.com:8080) (env: MSGRAPHPROXY)")

	// Retry configuration
	maxRetries := flag.Int("maxretries", 3, "Maximum retry attempts for transient failures (default: 3) (env: MSGRAPHMAXRETRIES)")
	retryDelay := flag.Int("retrydelay", 2000, "Base delay between retries in milliseconds (default: 2000ms) (env: MSGRAPHRETRYDELAY)")

	// Verbose mode
	verbose := flag.Bool("verbose", false, "Enable verbose output (shows configuration, tokens, API details)")

	// Log level
	logLevel := flag.String("loglevel", "INFO", "Logging level: DEBUG, INFO, WARN, ERROR (default: INFO)")

	// Output format
	outputFormat := flag.String("output", "text", "Output format: text, json (default: text) (env: MSGRAPHOUTPUT)")

	// Dry run mode (WhatIf)
	whatif := flag.Bool("whatif", false, "Dry run mode - preview actions without executing (PowerShell-style) (env: MSGRAPHWHATIF)")

	// Count for getevents and getinbox
	count := flag.Int("count", 3, "Number of items to retrieve for getevents and getinbox actions (default: 3) (env: MSGRAPHCOUNT)")

	action := flag.String("action", "getinbox", "Action to perform: getevents, sendmail, sendinvite, getinbox, getschedule, exportinbox, searchandexport (env: MSGRAPHACTION)")
	flag.Parse()

	// Apply environment variables if flags not set via command line
	applyEnvVars(map[string]*string{
		"MSGRAPHTENANTID":      tenantID,
		"MSGRAPHCLIENTID":      clientID,
		"MSGRAPHSECRET":        secret,
		"MSGRAPHPFX":           pfxPath,
		"MSGRAPHPFXPASS":       pfxPass,
		"MSGRAPHTHUMBPRINT":    thumbprint,
		"MSGRAPHMAILBOX":       mailbox,
		"MSGRAPHSUBJECT":       subject,
		"MSGRAPHBODY":          body,
		"MSGRAPHBODYHTML":      bodyHTML,
		"MSGRAPHBODYTEMPLATE":  bodyTemplate,
		"MSGRAPHINVITESUBJECT": inviteSubject,
		"MSGRAPHSTART":         startTime,
		"MSGRAPHEND":           endTime,
		"MSGRAPHMESSAGEID":     messageID,
		"MSGRAPHACTION":        action,
		"MSGRAPHPROXY":         proxyURL,
		"MSGRAPHOUTPUT":        outputFormat,
	})

	// Apply environment variables for stringSlice flags
	applyEnvVarsToSlice("to", &to, "MSGRAPHTO")
	applyEnvVarsToSlice("cc", &cc, "MSGRAPHCC")
	applyEnvVarsToSlice("bcc", &bcc, "MSGRAPHBCC")
	applyEnvVarsToSlice("attachments", &attachmentFiles, "MSGRAPHATTACHMENTS")

	// Apply MSGRAPHCOUNT environment variable if flag wasn't provided
	countFlagProvided := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "count" {
			countFlagProvided = true
		}
	})
	if !countFlagProvided {
		if envCount := os.Getenv("MSGRAPHCOUNT"); envCount != "" {
			if parsedCount, err := strconv.Atoi(envCount); err == nil && parsedCount > 0 {
				*count = parsedCount
			}
		}
	}

	// Apply MSGRAPHMAXRETRIES environment variable if flag wasn't provided
	maxRetriesFlagProvided := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "maxretries" {
			maxRetriesFlagProvided = true
		}
	})
	if !maxRetriesFlagProvided {
		if envMaxRetries := os.Getenv("MSGRAPHMAXRETRIES"); envMaxRetries != "" {
			if parsedMaxRetries, err := strconv.Atoi(envMaxRetries); err == nil && parsedMaxRetries >= 0 {
				*maxRetries = parsedMaxRetries
			}
		}
	}

	// Apply MSGRAPHRETRYDELAY environment variable if flag wasn't provided
	retryDelayFlagProvided := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "retrydelay" {
			retryDelayFlagProvided = true
		}
	})
	if !retryDelayFlagProvided {
		if envRetryDelay := os.Getenv("MSGRAPHRETRYDELAY"); envRetryDelay != "" {
			if parsedRetryDelay, err := strconv.Atoi(envRetryDelay); err == nil && parsedRetryDelay > 0 {
				*retryDelay = parsedRetryDelay
			}
		}
	}

	// Apply MSGRAPHLOGLEVEL environment variable if flag wasn't provided
	logLevelFlagProvided := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "loglevel" {
			logLevelFlagProvided = true
		}
	})
	if !logLevelFlagProvided {
		if envLogLevel := os.Getenv("MSGRAPHLOGLEVEL"); envLogLevel != "" {
			*logLevel = envLogLevel
		}
	}

	// Apply MSGRAPHWHATIF environment variable if flag wasn't provided
	whatifFlagProvided := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "whatif" {
			whatifFlagProvided = true
		}
	})
	if !whatifFlagProvided {
		if envWhatIf := os.Getenv("MSGRAPHWHATIF"); envWhatIf != "" {
			if parsedWhatIf, err := strconv.ParseBool(envWhatIf); err == nil {
				*whatif = parsedWhatIf
			}
		}
	}

	// Create and populate Config struct with all parsed values
	config := &Config{
		ShowVersion: *showVersion,
		TenantID:    *tenantID,
		ClientID:    *clientID,
		Mailbox:     *mailbox,
		Action:      *action,
		Secret:          *secret,
		PfxPath:         *pfxPath,
		PfxPass:         *pfxPass,
		Thumbprint:      *thumbprint,
		To:              to,
		Cc:              cc,
		Bcc:             bcc,
		AttachmentFiles: attachmentFiles,
		Subject:         *subject,
		Body:            *body,
		BodyHTML:        *bodyHTML,
		BodyTemplate:    *bodyTemplate,
		InviteSubject:   *inviteSubject,
		StartTime:       *startTime,
		EndTime:         *endTime,
		MessageID:       *messageID,
		ProxyURL:        *proxyURL,
		MaxRetries:      *maxRetries,
		RetryDelay:      time.Duration(*retryDelay) * time.Millisecond,
		VerboseMode:     *verbose,
		LogLevel:        *logLevel,
		OutputFormat:    strings.ToLower(*outputFormat),
		WhatIf:          *whatif,
		Count:           *count,
	}

	// Print verbose configuration if enabled
	if config.VerboseMode {
		printVerboseConfig(*tenantID, *clientID, *secret, *pfxPath, *thumbprint, *mailbox, *action, *proxyURL, to.String(), cc.String(), bcc.String(), *subject, *body, *bodyHTML, attachmentFiles.String(), *inviteSubject, *startTime, *endTime, *messageID, config.OutputFormat, *whatif)
	}

	return config
}

// applyEnvVars applies environment variable values to flags that weren't explicitly set via command line
func applyEnvVars(envMap map[string]*string) {
	// Track which flags were explicitly set via command line
	providedFlags := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		providedFlags[f.Name] = true
	})

	// Map flag names to environment variable names
	flagToEnv := map[string]string{
		"tenantid":       "MSGRAPHTENANTID",
		"clientid":       "MSGRAPHCLIENTID",
		"secret":         "MSGRAPHSECRET",
		"pfx":            "MSGRAPHPFX",
		"pfxpass":        "MSGRAPHPFXPASS",
		"thumbprint":     "MSGRAPHTHUMBPRINT",
		"mailbox":        "MSGRAPHMAILBOX",
		"subject":        "MSGRAPHSUBJECT",
		"body":           "MSGRAPHBODY",
		"bodyHTML":       "MSGRAPHBODYHTML",
		"body-template":  "MSGRAPHBODYTEMPLATE",
		"invite-subject": "MSGRAPHINVITESUBJECT",
		"start":          "MSGRAPHSTART",
		"end":            "MSGRAPHEND",
		"action":         "MSGRAPHACTION",
		"proxy":          "MSGRAPHPROXY",
		"output":         "MSGRAPHOUTPUT",
	}

	// For each environment variable, if flag wasn't provided, use env value
	for envName, flagPtr := range envMap {
		// Find the flag name for this env var
		var flagName string
		for fn, en := range flagToEnv {
			if en == envName {
				flagName = fn
				break
			}
		}

		// If flag was not provided via command line, check environment variable
		if !providedFlags[flagName] {
			if envValue := os.Getenv(envName); envValue != "" {
				*flagPtr = envValue
			}
		}
	}
}

// applyEnvVarsToSlice applies environment variable values to stringSlice flags
func applyEnvVarsToSlice(flagName string, slice *stringSlice, envName string) {
	// Check if flag was explicitly provided via command line
	flagProvided := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == flagName {
			flagProvided = true
		}
	})

	// If flag was not provided via command line, check environment variable
	if !flagProvided {
		if envValue := os.Getenv(envName); envValue != "" {
			slice.Set(envValue)
		}
	}
}

// validateConfiguration validates all required configuration fields
func validateConfiguration(config *Config) error {
	// Validate required fields with format checking
	if err := validateGUID(config.TenantID, "Tenant ID"); err != nil {
		return err
	}
	if err := validateGUID(config.ClientID, "Client ID"); err != nil {
		return err
	}
	if err := validateEmail(config.Mailbox); err != nil {
		return fmt.Errorf("invalid mailbox: %w", err)
	}

	// Check that at least one authentication method is provided
	authMethodCount := 0
	if config.Secret != "" {
		authMethodCount++
	}
	if config.PfxPath != "" {
		authMethodCount++
	}
	if config.Thumbprint != "" {
		authMethodCount++
	}

	if authMethodCount == 0 {
		return fmt.Errorf("missing authentication: must provide one of -secret, -pfx, or -thumbprint")
	}
	if authMethodCount > 1 {
		return fmt.Errorf("multiple authentication methods provided: use only one of -secret, -pfx, or -thumbprint")
	}

	// Validate PFX file path if provided
	if config.PfxPath != "" {
		if err := validateFilePath(config.PfxPath, "PFX certificate file"); err != nil {
			return err
		}
	}

	// Validate attachment file paths
	for i, attachmentPath := range config.AttachmentFiles {
		fieldName := fmt.Sprintf("Attachment file #%d", i+1)
		if err := validateFilePath(attachmentPath, fieldName); err != nil {
			return err
		}
	}

	// Validate body template file path
	if config.BodyTemplate != "" {
		if err := validateFilePath(config.BodyTemplate, "Body template file"); err != nil {
			return err
		}
	}

	// Validate email lists if provided
	if len(config.To) > 0 {
		if err := validateEmails(config.To, "To recipients"); err != nil {
			return err
		}
	}
	if len(config.Cc) > 0 {
		if err := validateEmails(config.Cc, "CC recipients"); err != nil {
			return err
		}
	}
	if len(config.Bcc) > 0 {
		if err := validateEmails(config.Bcc, "BCC recipients"); err != nil {
			return err
		}
	}

	// Validate RFC3339 times if provided
	if err := validateRFC3339Time(config.StartTime, "Start time"); err != nil {
		return err
	}
	if err := validateRFC3339Time(config.EndTime, "End time"); err != nil {
		return err
	}

	// Validate action
	validActions := map[string]bool{
		ActionGetEvents:       true,
		ActionSendMail:        true,
		ActionSendInvite:      true,
		ActionGetInbox:        true,
		ActionGetSchedule:     true,
		ActionExportInbox:     true,
		ActionSearchAndExport: true,
	}
	if !validActions[config.Action] {
		return fmt.Errorf("invalid action: %s (use: getevents, sendmail, sendinvite, getinbox, getschedule, exportinbox, searchandexport)", config.Action)
	}

	// Validate output format
	if config.OutputFormat != "text" && config.OutputFormat != "json" {
		return fmt.Errorf("invalid output format: %s (use: text, json)", config.OutputFormat)
	}

	// Validate getschedule-specific requirements
	if config.Action == ActionGetSchedule {
		if len(config.To) == 0 {
			return fmt.Errorf("getschedule action requires -to parameter (recipient email address)")
		}
		if len(config.To) > 1 {
			return fmt.Errorf("getschedule action only supports checking one recipient at a time (got %d recipients)", len(config.To))
		}
	}

	// Validate searchandexport-specific requirements
	if config.Action == ActionSearchAndExport {
		if config.MessageID == "" {
			return fmt.Errorf("searchandexport action requires -messageid parameter")
		}

		// SECURITY: Validate Message-ID format to prevent OData injection attacks
		if err := validateMessageID(config.MessageID); err != nil {
			return fmt.Errorf("invalid message ID: %w", err)
		}
	}

	return nil
}

// Print verbose configuration summary
func printVerboseConfig(tenantID, clientID, secret, pfxPath, thumbprint, mailbox, action, proxyURL, to, cc, bcc, subject, body, bodyHTML, attachments, inviteSubject, startTime, endTime, messageID, outputFormat string, whatif bool) {
	fmt.Println("========================================")
	fmt.Println("VERBOSE MODE ENABLED")
	fmt.Println("========================================")
	fmt.Println()

	// Display environment variables
	fmt.Println("Environment Variables (MSGRAPH*):")
	fmt.Println("----------------------------------")
	envVars := getEnvVariables()
	if len(envVars) == 0 {
		fmt.Println("  (no MSGRAPH environment variables set)")
	} else {
		// Sort keys for consistent output
		keys := make([]string, 0, len(envVars))
		for k := range envVars {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, key := range keys {
			value := envVars[key]
			// Mask sensitive values
			displayValue := value
			if key == "MSGRAPHSECRET" || key == "MSGRAPHPFXPASS" {
				displayValue = maskSecret(value)
			}
			fmt.Printf("  %s = %s\n", key, displayValue)
		}
	}
	fmt.Println()

	fmt.Println("Final Configuration (after env vars + flags):")
	fmt.Println("----------------------------------------------")
	fmt.Printf("Version: %s\n", version)
	fmt.Printf("Tenant ID: %s\n", tenantID)
	fmt.Printf("Client ID: %s\n", clientID)
	fmt.Printf("Mailbox: %s\n", mailbox)
	fmt.Printf("Action: %s\n", action)
	fmt.Printf("Output Format: %s\n", outputFormat)
	fmt.Printf("WhatIf (Dry Run): %t\n", whatif)

	// Authentication method
	fmt.Println()
	fmt.Println("Authentication:")
	if secret != "" {
		fmt.Println("  Method: Client Secret")
		// Mask the secret but show length
		fmt.Printf("  Secret: %s (length: %d)\n", maskSecret(secret), len(secret))
	} else if pfxPath != "" {
		fmt.Println("  Method: PFX Certificate")
		fmt.Printf("  PFX Path: %s\n", pfxPath)
		fmt.Println("  PFX Password: ******** (provided)")
	} else if thumbprint != "" {
		fmt.Println("  Method: Windows Certificate Store")
		fmt.Printf("  Thumbprint: %s\n", thumbprint)
	}

	// Network configuration
	if proxyURL != "" {
		fmt.Println()
		fmt.Println("Network Configuration:")
		fmt.Printf("  Proxy: %s\n", proxyURL)
	}

	// Action-specific parameters
	fmt.Println()
	fmt.Println("Action Parameters:")
	switch action {
	case "sendmail":
		fmt.Printf("  To: %s\n", ifEmpty(to, "(defaults to mailbox)"))
		fmt.Printf("  CC: %s\n", ifEmpty(cc, "(none)"))
		fmt.Printf("  BCC: %s\n", ifEmpty(bcc, "(none)"))
		fmt.Printf("  Subject: %s\n", subject)
		fmt.Printf("  Body (Text): %s\n", truncate(body, 60))
		fmt.Printf("  Body (HTML): %s\n", ifEmpty(truncate(bodyHTML, 60), "(none)"))
		fmt.Printf("  Attachments: %s\n", ifEmpty(attachments, "(none)"))
	case "sendinvite":
		fmt.Printf("  Invite Subject: %s\n", inviteSubject)
		fmt.Printf("  Start Time: %s\n", ifEmpty(startTime, "(now)"))
		fmt.Printf("  End Time: %s\n", ifEmpty(endTime, "(start + 1 hour)"))
	case "searchandexport":
		fmt.Printf("  Message ID: %s\n", messageID)
	case "getevents", "getinbox", "exportinbox":
		fmt.Println("  (no additional parameters)")
	}


	fmt.Println()
	fmt.Println("========================================")
	fmt.Println()
}

// Get all MSGRAPH environment variables
func getEnvVariables() map[string]string {
	envVars := make(map[string]string)

	// List of all MSGRAPH environment variables
	msgraphEnvVars := []string{
		"MSGRAPHTENANTID",
		"MSGRAPHCLIENTID",
		"MSGRAPHSECRET",
		"MSGRAPHPFX",
		"MSGRAPHPFXPASS",
		"MSGRAPHTHUMBPRINT",
		"MSGRAPHMAILBOX",
		"MSGRAPHTO",
		"MSGRAPHCC",
		"MSGRAPHBCC",
		"MSGRAPHSUBJECT",
		"MSGRAPHBODY",
		"MSGRAPHBODYHTML",
		"MSGRAPHBODYTEMPLATE",
		"MSGRAPHATTACHMENTS",
		"MSGRAPHINVITESUBJECT",
		"MSGRAPHSTART",
		"MSGRAPHEND",
		"MSGRAPHMESSAGEID",
		"MSGRAPHACTION",
		"MSGRAPHPROXY",
		"MSGRAPHCOUNT",
		"MSGRAPHMAXRETRIES",
		"MSGRAPHRETRYDELAY",
		"MSGRAPHLOGLEVEL",
		"MSGRAPHOUTPUT",
	}

	for _, envVar := range msgraphEnvVars {
		if value := os.Getenv(envVar); value != "" {
			envVars[envVar] = value
		}
	}

	return envVars
}

// stringSlice implements the flag.Value interface for comma-separated string lists.
type stringSlice []string

// String returns the comma-separated string representation of the slice.
func (s *stringSlice) String() string {
	if s == nil {
		return ""
	}
	return strings.Join(*s, ",")
}

// Set parses a comma-separated string into a slice of trimmed strings.
func (s *stringSlice) Set(value string) error {
	if value == "" {
		*s = nil
		return nil
	}
	parts := strings.Split(value, ",")
	var result []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	*s = result
	return nil
}

// validateEmail performs basic email format validation.
func validateEmail(email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return fmt.Errorf("email cannot be empty")
	}
	if !strings.Contains(email, "@") {
		return fmt.Errorf("invalid email format: %s (missing @)", email)
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("invalid email format: %s", email)
	}
	return nil
}

// validateEmails validates a slice of email addresses
func validateEmails(emails []string, fieldName string) error {
	for _, email := range emails {
		if err := validateEmail(email); err != nil {
			return fmt.Errorf("%s contains invalid email: %w", fieldName, err)
		}
	}
	return nil
}

// validateGUID validates that a string matches standard GUID format
func validateGUID(guid, fieldName string) error {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return fmt.Errorf("%s cannot be empty", fieldName)
	}
	// Basic GUID format: 8-4-4-4-12 hex characters
	if len(guid) != 36 {
		return fmt.Errorf("%s should be a GUID (36 characters, format: 12345678-1234-1234-1234-123456789012)", fieldName)
	}
	// Check for proper dash positions
	if guid[8] != '-' || guid[13] != '-' || guid[18] != '-' || guid[23] != '-' {
		return fmt.Errorf("%s has invalid GUID format (dashes at wrong positions)", fieldName)
	}
	return nil
}

// validateRFC3339Time validates that a string matches RFC3339 or PowerShell sortable timestamp format
func validateRFC3339Time(timeStr, fieldName string) error {
	if timeStr == "" {
		return nil // Empty is allowed (defaults are used)
	}
	_, err := parseFlexibleTime(timeStr)
	if err != nil {
		return fmt.Errorf("%s: %w", fieldName, err)
	}
	return nil
}

// validateFilePath validates and sanitizes a file path for security and usability.
func validateFilePath(path, fieldName string) error {
	if path == "" {
		return nil // Empty is allowed for optional fields
	}

	// Clean and normalize path (resolves . and .. elements)
	cleanPath := filepath.Clean(path)

	// Check for path traversal attempts
	// After cleaning, ".." should not remain in the path unless it's at the start (relative path going up)
	// We need to check if the cleaned path tries to escape the current directory context
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return fmt.Errorf("%s: invalid path: %w", fieldName, err)
	}

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		// If we can't get cwd, just verify the file exists
		cwd = ""
	}

	// If we have a cwd, check if the absolute path tries to go outside reasonable bounds
	// For absolute paths, this is allowed
	// For relative paths, we verify they don't traverse outside the working directory tree
	if cwd != "" && !filepath.IsAbs(path) {
		// Check if cleaned path still contains ".." which indicates traversal
		if strings.Contains(cleanPath, "..") {
			return fmt.Errorf("%s: path contains directory traversal (..) which is not allowed", fieldName)
		}
	}

	// Verify file exists and is accessible
	fileInfo, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s: file not found: %s", fieldName, path)
		}
		if os.IsPermission(err) {
			return fmt.Errorf("%s: permission denied: %s", fieldName, path)
		}
		return fmt.Errorf("%s: cannot access file: %w", fieldName, err)
	}

	// Verify it's a regular file (not a directory or special file)
	if !fileInfo.Mode().IsRegular() {
		return fmt.Errorf("%s: not a regular file (is it a directory?): %s", fieldName, path)
	}

	return nil
}

// Action constants
const (
	ActionGetEvents    = "getevents"
	ActionSendMail     = "sendmail"
	ActionSendInvite   = "sendinvite"
	ActionGetInbox     = "getinbox"
	ActionGetSchedule     = "getschedule"
	ActionExportInbox     = "exportinbox"
	ActionSearchAndExport = "searchandexport"
)

// generateBashCompletion generates a bash completion script for the tool
func generateBashCompletion() string {
	return `# msgraphtool bash completion script
# Installation:
#   Linux: Copy to /etc/bash_completion.d/msgraphtool
#   macOS: Copy to /usr/local/etc/bash_completion.d/msgraphtool
#   Manual: source this file in your ~/.bashrc

_msgraphtool_completions() {
    local cur prev opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    # All available flags
    opts="-action -tenantid -clientid -secret -pfx -pfxpass -thumbprint -mailbox
          -to -cc -bcc -subject -body -bodyHTML -attachments
          -invite-subject -start -end -messageid -proxy -count -verbose -version -help
          -maxretries -retrydelay -loglevel -completion"

    # Flag-specific completions
    case "${prev}" in
        -action)
            # Suggest valid actions
            COMPREPLY=( $(compgen -W "getevents sendmail sendinvite getinbox getschedule exportinbox searchandexport" -- ${cur}) )
            return 0
            ;;
        -pfx|-attachments)
            # File path completion
            COMPREPLY=( $(compgen -f -- ${cur}) )
            return 0
            ;;
        -loglevel)
            # Suggest log levels
            COMPREPLY=( $(compgen -W "DEBUG INFO WARN ERROR" -- ${cur}) )
            return 0
            ;;
        -completion)
            # Suggest shell types
            COMPREPLY=( $(compgen -W "bash powershell" -- ${cur}) )
            return 0
            ;;
        -version|-verbose|-help)
            # No completion after boolean flags
            return 0
            ;;
        -maxretries|-retrydelay|-count)
            # Numeric values - no completion
            return 0
            ;;
        -tenantid|-clientid|-secret|-pfxpass|-thumbprint|-mailbox|-to|-cc|-bcc|-subject|-body|-bodyHTML|-invite-subject|-start|-end|-messageid|-proxy)
            # String values - no completion
            return 0
            ;;
    esac

    # Default: complete with flag names
    COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
    return 0
}

# Register the completion function for the tool
complete -F _msgraphtool_completions msgraphtool.exe
complete -F _msgraphtool_completions msgraphtool
complete -F _msgraphtool_completions ./msgraphtool.exe
complete -F _msgraphtool_completions ./msgraphtool
`
}

// generatePowerShellCompletion generates a PowerShell completion script for the tool
func generatePowerShellCompletion() string {
	return `# msgraphtool PowerShell completion script
# Installation:
#   Add to your PowerShell profile: notepad $PROFILE
#   Or run manually: . .\msgraphtool-completion.ps1

Register-ArgumentCompleter -CommandName msgraphtool.exe,msgraphtool,'.\msgraphtool.exe','.\msgraphtool' -ScriptBlock {
    param($commandName, $parameterName, $wordToComplete, $commandAst, $fakeBoundParameters)

    # Define valid actions
    $actions = @('getevents', 'sendmail', 'sendinvite', 'getinbox', 'getschedule', 'exportinbox', 'searchandexport')

    # Define log levels
    $logLevels = @('DEBUG', 'INFO', 'WARN', 'ERROR')

    # Define shell types for completion flag
    $shellTypes = @('bash', 'powershell')

    # All flags that accept values
    $flags = @(
        '-action', '-tenantid', '-clientid', '-secret', '-pfx', '-pfxpass',
        '-thumbprint', '-mailbox', '-to', '-cc', '-bcc', '-subject', '-body',
        '-bodyHTML', '-attachments', '-invite-subject', '-start', '-end',
        '-messageid', '-proxy', '-count', '-maxretries', '-retrydelay', '-loglevel',
        '-completion', '-verbose', '-version', '-help'
    )

    # Get the last word from command line
    $lastWord = ''
    if ($commandAst.CommandElements.Count -gt 1) {
        $lastWord = $commandAst.CommandElements[-2].ToString()
    }

    # Provide context-specific completions based on the previous flag
    switch ($lastWord) {
        '-action' {
            $actions | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', "Action: $_")
            }
            return
        }
        '-loglevel' {
            $logLevels | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', "Log Level: $_")
            }
            return
        }
        '-completion' {
            $shellTypes | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', "Shell: $_")
            }
            return
        }
        '-pfx' {
            # File completion for PFX files
            Get-ChildItem -Path "$wordToComplete*" -File -ErrorAction SilentlyContinue |
                Where-Object { $_.Extension -in @('.pfx', '.p12') -or $wordToComplete -eq '' } |
                ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new(
                        $_.FullName,
                        $_.Name,
                        'ParameterValue',
                        "Certificate: $($_.Name)"
                    )
                }
            return
        }
        '-attachments' {
            # File completion for any file type
            Get-ChildItem -Path "$wordToComplete*" -File -ErrorAction SilentlyContinue |
                ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new(
                        $_.FullName,
                        $_.Name,
                        'ParameterValue',
                        "File: $($_.Name)"
                    )
                }
            return
        }
    }

    # Default: complete with flag names
    $flags | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
        $description = switch ($_) {
            '-action' { 'Operation to perform (getevents, sendmail, sendinvite, getinbox)' }
            '-tenantid' { 'Azure Tenant ID (GUID)' }
            '-clientid' { 'Application (Client) ID (GUID)' }
            '-secret' { 'Client Secret for authentication' }
            '-pfx' { 'Path to .pfx certificate file' }
            '-pfxpass' { 'Password for .pfx certificate' }
            '-thumbprint' { 'Certificate thumbprint (Windows Certificate Store)' }
            '-mailbox' { 'Target user email address' }
            '-to' { 'Comma-separated TO recipients' }
            '-cc' { 'Comma-separated CC recipients' }
            '-bcc' { 'Comma-separated BCC recipients' }
            '-subject' { 'Email subject line' }
            '-body' { 'Email body (text)' }
            '-bodyHTML' { 'Email body (HTML)' }
            '-attachments' { 'Comma-separated file paths to attach' }
            '-invite-subject' { 'Calendar invite subject' }
            '-start' { 'Start time for calendar invite (RFC3339)' }
            '-end' { 'End time for calendar invite (RFC3339)' }
            '-messageid' { 'Internet Message ID for searchandexport' }
            '-proxy' { 'HTTP/HTTPS proxy URL' }
            '-count' { 'Number of items to retrieve (default: 3)' }
            '-maxretries' { 'Maximum retry attempts (default: 3)' }
            '-retrydelay' { 'Retry delay in milliseconds (default: 2000)' }
            '-loglevel' { 'Logging level (DEBUG, INFO, WARN, ERROR)' }
            '-completion' { 'Generate completion script (bash or powershell)' }
            '-verbose' { 'Enable verbose output' }
            '-version' { 'Show version information' }
            '-help' { 'Show help message' }
            default { $_ }
        }
        [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterName', $description)
    }
}

Write-Host "PowerShell completion for msgraphtool loaded successfully!" -ForegroundColor Green
Write-Host "Try typing: msgraphtool.exe -<TAB>" -ForegroundColor Cyan
`
}
