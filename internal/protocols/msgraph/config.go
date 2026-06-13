package msgraph

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/ziembor/gomailtesttool/internal/common/email"
	"github.com/ziembor/gomailtesttool/internal/common/validation"
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
	Secret      string // Client Secret for authentication
	PfxPath     string // Path to .pfx certificate file
	PfxPass     string // Password for .pfx certificate file
	Thumbprint  string // SHA1 thumbprint of certificate in Windows Certificate Store
	BearerToken string // Pre-obtained Bearer token for authentication

	// Email recipients (using stringSlice type for comma-separated lists)
	To                    stringSlice // To recipients for email
	Cc                    stringSlice // CC recipients for email
	Bcc                   stringSlice // BCC recipients for email
	AttachmentFiles       stringSlice // File paths to attach to email
	InlineAttachmentFiles stringSlice // File paths to embed inline via cid: (referenced from BodyHTML)
	Headers               []string    // Custom headers in "Name: Value" form

	// Email content
	Subject      string // Email subject line
	Body         string // Email body text content
	BodyHTML     string // Email body HTML content (future use)
	BodyTemplate string // Path to HTML email body template file
	Priority     string // Email priority: high, normal, low (maps to Graph Importance)

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
	LogFormat    string // Log file format: csv, json (default: csv)
	Count        int    // Number of items to retrieve (for getevents and getinbox actions)
}

// NewConfig creates a new Config with sensible default values.
func NewConfig() *Config {
	return &Config{
		Subject:       "Automated Tool Notification",
		Body:          "It's a test message, please ignore",
		Priority:      "normal",
		InviteSubject: "System Sync",
		Action:        ActionGetInbox,
		Count:         3,
		VerboseMode:   false,
		LogLevel:      "INFO",
		OutputFormat:  "text",
		LogFormat:     "csv",
		ShowVersion:   false,
		MaxRetries:    3,
		RetryDelay:    2000 * time.Millisecond,
	}
}

// Action constants
const (
	ActionGetEvents       = "getevents"
	ActionSendMail        = "sendmail"
	ActionSendInvite      = "sendinvite"
	ActionGetInbox        = "getinbox"
	ActionGetSchedule     = "getschedule"
	ActionExportInbox     = "exportinbox"
	ActionSearchAndExport = "searchandexport"
)

// RegisterPersistentFlags registers flags shared by all msgraph subcommands
// on the given parent command (the "msgraph" cobra.Command).
func RegisterPersistentFlags(cmd *cobra.Command) {
	f := cmd.PersistentFlags()

	// Authentication
	f.String("tenantid", "", "The Azure Tenant ID (env: MSGRAPHTENANTID)")
	f.String("clientid", "", "The Application (Client) ID (env: MSGRAPHCLIENTID)")
	f.String("secret", "", "The Client Secret (env: MSGRAPHSECRET)")
	f.String("pfx", "", "Path to the .pfx certificate file (env: MSGRAPHPFX)")
	f.String("pfxpass", "", "Password for the .pfx file (env: MSGRAPHPFXPASS)")
	f.String("thumbprint", "", "Thumbprint of the certificate in the CurrentUser\\My store (env: MSGRAPHTHUMBPRINT)")
	f.String("bearertoken", "", "Pre-obtained Bearer token for authentication (env: MSGRAPHBEARERTOKEN)")

	// Target
	f.String("mailbox", "", "The target EXO mailbox email address (env: MSGRAPHMAILBOX)")

	// Network
	f.String("proxy", "", "HTTP/HTTPS proxy URL (e.g., http://proxy.example.com:8080) (env: MSGRAPHPROXY)")
	f.Int("maxretries", 3, "Maximum retry attempts for transient failures (env: MSGRAPHMAXRETRIES)")
	f.Int("retrydelay", 2000, "Base delay between retries in milliseconds (env: MSGRAPHRETRYDELAY)")

	// Output
	f.Bool("verbose", false, "Enable verbose output (env: -)")
	f.String("loglevel", "INFO", "Logging level: DEBUG, INFO, WARN, ERROR (env: MSGRAPHLOGLEVEL)")
	f.String("output", "text", "Output format: text, json (env: MSGRAPHOUTPUT)")
	f.String("logformat", "csv", "Log file format: csv, json (env: MSGRAPHLOGFORMAT)")
}

// BindEnvs registers Viper environment variable bindings for all msgraph config keys.
// Must be called after RegisterPersistentFlags.
func BindEnvs(v *viper.Viper) {
	bindings := map[string]string{
		"tenantid":           "MSGRAPHTENANTID",
		"clientid":           "MSGRAPHCLIENTID",
		"secret":             "MSGRAPHSECRET",
		"pfx":                "MSGRAPHPFX",
		"pfxpass":            "MSGRAPHPFXPASS",
		"thumbprint":         "MSGRAPHTHUMBPRINT",
		"bearertoken":        "MSGRAPHBEARERTOKEN",
		"mailbox":            "MSGRAPHMAILBOX",
		"to":                 "MSGRAPHTO",
		"cc":                 "MSGRAPHCC",
		"bcc":                "MSGRAPHBCC",
		"subject":            "MSGRAPHSUBJECT",
		"body":               "MSGRAPHBODY",
		"bodyhtml":           "MSGRAPHBODYHTML",
		"priority":           "MSGRAPHPRIORITY",
		"body-template":      "MSGRAPHBODYTEMPLATE",
		"attachments":        "MSGRAPHATTACHMENTS",
		"inline-attachments": "MSGRAPHINLINEATTACHMENTS",
		"start":              "MSGRAPHSTART",
		"end":                "MSGRAPHEND",
		"messageid":          "MSGRAPHMESSAGEID",
		"proxy":              "MSGRAPHPROXY",
		"maxretries":         "MSGRAPHMAXRETRIES",
		"retrydelay":         "MSGRAPHRETRYDELAY",
		"loglevel":           "MSGRAPHLOGLEVEL",
		"output":             "MSGRAPHOUTPUT",
		"logformat":          "MSGRAPHLOGFORMAT",
		"count":              "MSGRAPHCOUNT",
	}
	for key, env := range bindings {
		_ = v.BindEnv(key, env)
	}
}

// ConfigFromViper reads all msgraph config values from the given Viper instance.
// The action field must be set by the caller (it comes from which subcommand ran).
func ConfigFromViper(v *viper.Viper) *Config {
	defaults := NewConfig()

	retryDelayMs := v.GetInt("retrydelay")
	if retryDelayMs <= 0 {
		retryDelayMs = 2000
	}

	count := v.GetInt("count")
	if count <= 0 {
		count = defaults.Count
	}

	maxRetries := v.GetInt("maxretries")
	if maxRetries < 0 {
		maxRetries = defaults.MaxRetries
	}

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

	return &Config{
		TenantID:              v.GetString("tenantid"),
		ClientID:              v.GetString("clientid"),
		Mailbox:               v.GetString("mailbox"),
		Secret:                v.GetString("secret"),
		PfxPath:               v.GetString("pfx"),
		PfxPass:               v.GetString("pfxpass"),
		Thumbprint:            v.GetString("thumbprint"),
		BearerToken:           v.GetString("bearertoken"),
		To:                    parseStringSlice(v.GetString("to")),
		Cc:                    parseStringSlice(v.GetString("cc")),
		Bcc:                   parseStringSlice(v.GetString("bcc")),
		AttachmentFiles:       parseStringSlice(v.GetString("attachments")),
		InlineAttachmentFiles: parseStringSlice(v.GetString("inline-attachments")),
		Headers:               v.GetStringSlice("header"),
		Subject:               subject,
		Body:                  body,
		BodyHTML:              v.GetString("bodyhtml"),
		BodyTemplate:          v.GetString("body-template"),
		Priority:              priority,
		InviteSubject:         v.GetString("invite-subject"),
		StartTime:             v.GetString("start"),
		EndTime:               v.GetString("end"),
		MessageID:             v.GetString("messageid"),
		ProxyURL:              v.GetString("proxy"),
		MaxRetries:            maxRetries,
		RetryDelay:            time.Duration(retryDelayMs) * time.Millisecond,
		VerboseMode:           v.GetBool("verbose"),
		LogLevel:              logLevel,
		OutputFormat:          outputFormat,
		LogFormat:             logFormat,
		Count:                 count,
	}
}

// parseStringSlice parses a comma-separated string into a stringSlice.
// Returns nil for an empty string.
func parseStringSlice(s string) stringSlice {
	if s == "" {
		return nil
	}
	var result stringSlice
	_ = result.Set(s)
	return result
}

// validateConfiguration validates all required configuration fields
func validateConfiguration(config *Config) error {
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
	if config.BearerToken != "" {
		authMethodCount++
	}

	if authMethodCount == 0 {
		return fmt.Errorf("missing authentication: must provide one of --secret, --pfx, --thumbprint, or --bearertoken")
	}
	if authMethodCount > 1 {
		return fmt.Errorf("multiple authentication methods provided: use only one of --secret, --pfx, --thumbprint, or --bearertoken")
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

	// Validate inline attachment file paths
	for i, attachmentPath := range config.InlineAttachmentFiles {
		fieldName := fmt.Sprintf("Inline attachment file #%d", i+1)
		if err := validateFilePath(attachmentPath, fieldName); err != nil {
			return err
		}
	}

	// Validate custom headers
	if _, err := email.ParseHeaders(config.Headers); err != nil {
		return fmt.Errorf("invalid -header: %w", err)
	}

	// Validate body template file path
	if config.BodyTemplate != "" {
		if err := validateFilePath(config.BodyTemplate, "Body template file"); err != nil {
			return err
		}
	}

	// Validate email priority
	switch config.Priority {
	case "high", "normal", "low":
	default:
		return fmt.Errorf("invalid -priority: %s (must be one of: high, normal, low)", config.Priority)
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

	// Validate output format
	if config.OutputFormat != "text" && config.OutputFormat != "json" {
		return fmt.Errorf("invalid output format: %s (use: text, json)", config.OutputFormat)
	}

	// Validate getschedule-specific requirements
	if config.Action == ActionGetSchedule {
		if len(config.To) == 0 {
			return fmt.Errorf("getschedule action requires --to parameter (recipient email address)")
		}
		if len(config.To) > 1 {
			return fmt.Errorf("getschedule action only supports checking one recipient at a time (got %d recipients)", len(config.To))
		}
	}

	// Validate searchandexport-specific requirements
	if config.Action == ActionSearchAndExport {
		if config.MessageID == "" {
			return fmt.Errorf("searchandexport action requires --messageid parameter")
		}

		// SECURITY: Validate Message-ID format to prevent OData injection attacks
		if err := validateMessageID(config.MessageID); err != nil {
			return fmt.Errorf("invalid message ID: %w", err)
		}
	}

	return nil
}

// stringSlice implements the pflag.Value interface for comma-separated string lists.
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

// Type returns the type name for pflag usage.
func (s *stringSlice) Type() string {
	return "stringSlice"
}

// validateEmail wraps the common validation package.
func validateEmail(email string) error {
	return validation.ValidateEmail(email)
}

// validateEmails wraps the common validation package.
func validateEmails(emails []string, fieldName string) error {
	return validation.ValidateEmails(emails, fieldName)
}

// validateGUID wraps the common validation package.
func validateGUID(guid, fieldName string) error {
	return validation.ValidateGUID(guid, fieldName)
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

// validateFilePath wraps the common validation package.
func validateFilePath(path, fieldName string) error {
	return validation.ValidateFilePath(path, fieldName)
}
