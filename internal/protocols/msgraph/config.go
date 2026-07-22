package msgraph

import (
	"fmt"
	"strings"
	"time"

	"github.com/ehlo-pl/gomailtesttool/internal/common/email"
	tmpl "github.com/ehlo-pl/gomailtesttool/internal/common/template"
	"github.com/ehlo-pl/gomailtesttool/internal/common/validation"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
	Secret      string   // Client Secret for authentication
	PfxPath     string   // Path to .pfx certificate file
	PfxPass     string   // Password for .pfx certificate file
	Thumbprint  string   // SHA1 thumbprint of certificate in Windows Certificate Store
	Delegated   bool     // Use delegated permissions flow instead of application permissions
	AuthFlow    string   // Delegated auth flow: devicecode, browser
	RedirectURL string   // Redirect URL for browser auth flow
	Scopes      []string // OAuth scopes to request (delegated mode)
	BearerToken string   // Pre-obtained Bearer token for authentication

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
	Priority     string   // Email priority: high, normal, low (maps to Graph Importance)
	Template     string   // Path to a message template: .eml (fields mapped to the Graph API) or HTML body file
	TemplateVars []string // Template variables in "key=value" form, referenced as {{.key}}
	SaveToSent   bool     // Save a copy in Sent Items (Graph API saveToSentItems)

	// Calendar invite configuration
	InviteSubject string // Subject of calendar meeting invitation
	StartTime     string // Start time in RFC3339 format (e.g., 2026-01-15T14:00:00Z)
	EndTime       string // End time in RFC3339 format

	// Folder selection (listmail)
	Folder string // Well-known or custom folder name for listmail (default: inbox)

	// Search configuration
	MessageID string // Internet Message ID for searchandexport/exportmessages actions

	// Export configuration
	ExportDir string // Directory under which to create the dated export folder (default: OS temp dir)

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
	Duration     int    // Meeting slot length in minutes (findtimeslot)
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
	ActionGetEvents         = "getevents"
	ActionSendMail          = "sendmail"
	ActionSaveDraft         = "draft"
	ActionSendInvite        = "sendinvite"
	ActionGetInbox          = "getinbox"
	ActionListFolders       = "listfolders"
	ActionListMail          = "listmail"
	ActionGetSchedule       = "getschedule"
	ActionFindTimeSlot      = "findtimeslot"
	ActionExportInbox       = "exportinbox"
	ActionSearchAndExport   = "searchandexport"
	ActionExportMessages    = "exportmessages"
	ActionExportBearerToken = "exportbearertoken"
	ActionTestConnect       = "testconnect"
	ActionTestAuth          = "testauth"
)

// Delegated auth flows accepted by --authflow; an empty AuthFlow means device code.
const (
	AuthFlowDeviceCode = "devicecode"
	AuthFlowBrowser    = "browser"
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
	f.Bool("delegated", false, "Use delegated permissions auth flow (env: MSGRAPHDELEGATED)")
	_ = f.MarkDeprecated("delegated", "delegated mode flags are deprecated; see docs/protocols/msgraph.md")
	f.String("authflow", AuthFlowDeviceCode, "Delegated auth flow: devicecode, browser (env: MSGRAPHAUTHFLOW)")
	_ = f.MarkDeprecated("authflow", "delegated mode flags are deprecated; see docs/protocols/msgraph.md")
	f.String("redirecturl", "", "Redirect URL for browser auth flow (env: MSGRAPHREDIRECTURL)")
	_ = f.MarkDeprecated("redirecturl", "delegated mode flags are deprecated; see docs/protocols/msgraph.md")
	f.String("scope", "", "Comma-separated delegated scopes; default covers mail/calendar operations (env: MSGRAPHSCOPE)")
	_ = f.MarkDeprecated("scope", "delegated mode flags are deprecated; see docs/protocols/msgraph.md")
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
		"delegated":          "MSGRAPHDELEGATED",
		"authflow":           "MSGRAPHAUTHFLOW",
		"redirecturl":        "MSGRAPHREDIRECTURL",
		"scope":              "MSGRAPHSCOPE",
		"bearertoken":        "MSGRAPHBEARERTOKEN",
		"mailbox":            "MSGRAPHMAILBOX",
		"to":                 "MSGRAPHTO",
		"cc":                 "MSGRAPHCC",
		"bcc":                "MSGRAPHBCC",
		"subject":            "MSGRAPHSUBJECT",
		"body":               "MSGRAPHBODY",
		"bodyhtml":           "MSGRAPHBODYHTML",
		"priority":           "MSGRAPHPRIORITY",
		"template":           "MSGRAPHTEMPLATE",
		"template-vars":      "MSGRAPHTEMPLATEVARS",
		"attachments":        "MSGRAPHATTACHMENTS",
		"inline-attachments": "MSGRAPHINLINEATTACHMENTS",
		"start":              "MSGRAPHSTART",
		"end":                "MSGRAPHEND",
		"duration":           "MSGRAPHDURATION",
		"folder":             "MSGRAPHFOLDER",
		"messageid":          "MSGRAPHMESSAGEID",
		"exportdir":          "MSGRAPHEXPORTDIR",
		"proxy":              "MSGRAPHPROXY",
		"maxretries":         "MSGRAPHMAXRETRIES",
		"retrydelay":         "MSGRAPHRETRYDELAY",
		"loglevel":           "MSGRAPHLOGLEVEL",
		"output":             "MSGRAPHOUTPUT",
		"logformat":          "MSGRAPHLOGFORMAT",
		"count":              "MSGRAPHCOUNT",
		"header":             "MSGRAPHHEADER",
		"save-to-sent":       "MSGRAPHSAVETOSENT",
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

	duration := v.GetInt("duration")
	if duration <= 0 {
		duration = 30
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

	authFlow := strings.ToLower(v.GetString("authflow"))
	// --authflow devicecode|browser implies delegated mode; --delegated is deprecated.
	delegated := v.GetBool("delegated") || authFlow == AuthFlowDeviceCode || authFlow == AuthFlowBrowser

	return &Config{
		TenantID:              v.GetString("tenantid"),
		ClientID:              v.GetString("clientid"),
		Mailbox:               v.GetString("mailbox"),
		Secret:                v.GetString("secret"),
		PfxPath:               v.GetString("pfx"),
		PfxPass:               v.GetString("pfxpass"),
		Thumbprint:            v.GetString("thumbprint"),
		BearerToken:           v.GetString("bearertoken"),
		Delegated:             delegated,
		AuthFlow:              authFlow,
		RedirectURL:           strings.TrimSpace(v.GetString("redirecturl")),
		Scopes:                parseStringSlice(v.GetString("scope")),
		To:                    parseStringSlice(v.GetString("to")),
		Cc:                    parseStringSlice(v.GetString("cc")),
		Bcc:                   parseStringSlice(v.GetString("bcc")),
		AttachmentFiles:       parseStringSlice(v.GetString("attachments")),
		InlineAttachmentFiles: parseStringSlice(v.GetString("inline-attachments")),
		Headers:               v.GetStringSlice("header"),
		Subject:               subject,
		Body:                  body,
		BodyHTML:              v.GetString("bodyhtml"),
		Priority:              priority,
		Template:              v.GetString("template"),
		TemplateVars:          v.GetStringSlice("template-vars"),
		SaveToSent:            v.GetBool("save-to-sent"),
		InviteSubject:         v.GetString("invite-subject"),
		StartTime:             v.GetString("start"),
		EndTime:               v.GetString("end"),
		Folder:                v.GetString("folder"),
		MessageID:             v.GetString("messageid"),
		ExportDir:             v.GetString("exportdir"),
		ProxyURL:              v.GetString("proxy"),
		MaxRetries:            maxRetries,
		RetryDelay:            time.Duration(retryDelayMs) * time.Millisecond,
		VerboseMode:           v.GetBool("verbose"),
		LogLevel:              logLevel,
		OutputFormat:          outputFormat,
		LogFormat:             logFormat,
		Count:                 count,
		Duration:              duration,
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

	if err := validateAuthConfiguration(config); err != nil {
		return err
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

	// Validate --template/--template-vars. Unlike SMTP/gmail, an .eml
	// template is parsed and its recognised fields mapped onto the Graph
	// send API, so attachment/header/priority flags still apply.
	if len(config.TemplateVars) > 0 && config.Template == "" {
		return fmt.Errorf("--template-vars requires --template")
	}
	if config.Template != "" {
		if err := validateFilePath(config.Template, "Template file"); err != nil {
			return err
		}
		if config.BodyHTML != "" {
			return fmt.Errorf("cannot use both --template and --bodyhtml simultaneously")
		}
		if config.Body != NewConfig().Body {
			return fmt.Errorf("cannot use both --template and --body simultaneously")
		}
		if _, err := tmpl.ParseVars(config.TemplateVars); err != nil {
			return fmt.Errorf("invalid --template-vars: %w", err)
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

	// Validate findtimeslot-specific requirements
	if config.Action == ActionFindTimeSlot {
		if len(config.To) == 0 {
			return fmt.Errorf("findtimeslot action requires --to parameter (recipient email address)")
		}
		if len(config.To) > 1 {
			return fmt.Errorf("findtimeslot action only supports checking one recipient at a time (got %d recipients)", len(config.To))
		}
		if config.Duration < 5 || config.Duration > 480 {
			return fmt.Errorf("invalid --duration: %d (must be 5-480 minutes)", config.Duration)
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

	// Validate exportmessages-specific requirements
	if config.Action == ActionExportMessages {
		if config.MessageID == "" && strings.TrimSpace(config.Subject) == "" && strings.TrimSpace(config.Folder) == "" {
			return fmt.Errorf("exportmessages action requires --messageid, --subject and/or --folder parameter")
		}

		if config.MessageID != "" {
			// SECURITY: Validate Message-ID format to prevent OData injection attacks
			if err := validateMessageID(config.MessageID); err != nil {
				return fmt.Errorf("invalid message ID: %w", err)
			}
		}

		if config.Subject != "" {
			if err := validateSearchSubject(config.Subject); err != nil {
				return fmt.Errorf("invalid subject: %w", err)
			}
		}
	}

	return nil
}

// validateExportBearerTokenConfiguration validates config for exportbearertoken action.
func validateExportBearerTokenConfiguration(config *Config) error {
	if config.BearerToken == "" {
		if err := validateGUID(config.TenantID, "Tenant ID"); err != nil {
			return err
		}
		if err := validateGUID(config.ClientID, "Client ID"); err != nil {
			return err
		}
	}

	if err := validateAuthConfiguration(config); err != nil {
		return err
	}

	if config.OutputFormat != "text" && config.OutputFormat != "json" {
		return fmt.Errorf("invalid output format: %s (use: text, json)", config.OutputFormat)
	}

	return nil
}

// validateTestConnectConfiguration validates config for the testconnect action.
// testconnect is an unauthenticated network/TLS probe against the Graph endpoint,
// so it needs neither credentials, tenant/client IDs, nor a mailbox — only a
// valid output format (and any proxy is validated at dial time).
func validateTestConnectConfiguration(config *Config) error {
	if config.OutputFormat != "text" && config.OutputFormat != "json" {
		return fmt.Errorf("invalid output format: %s (use: text, json)", config.OutputFormat)
	}
	return nil
}

// validateTestAuthConfiguration validates config for the testauth action. Like
// exportbearertoken it verifies a credential rather than operating on a mailbox,
// so the mailbox is optional (validated only when provided for the supplementary
// Graph check). Tenant/Client IDs are required unless a pre-obtained bearer token
// is supplied.
func validateTestAuthConfiguration(config *Config) error {
	if config.BearerToken == "" {
		if err := validateGUID(config.TenantID, "Tenant ID"); err != nil {
			return err
		}
		if err := validateGUID(config.ClientID, "Client ID"); err != nil {
			return err
		}
	}

	if err := validateAuthConfiguration(config); err != nil {
		return err
	}

	if config.Mailbox != "" {
		if err := validateEmail(config.Mailbox); err != nil {
			return fmt.Errorf("invalid mailbox: %w", err)
		}
	}

	if config.OutputFormat != "text" && config.OutputFormat != "json" {
		return fmt.Errorf("invalid output format: %s (use: text, json)", config.OutputFormat)
	}

	return nil
}

func validateAuthConfiguration(config *Config) error {
	if config.Delegated {
		switch config.AuthFlow {
		case "", AuthFlowDeviceCode:
		case AuthFlowBrowser:
			if config.RedirectURL == "" {
				return fmt.Errorf("browser delegated flow requires --redirecturl")
			}
		default:
			return fmt.Errorf("invalid -authflow: %s (must be one of: %s, %s)", config.AuthFlow, AuthFlowDeviceCode, AuthFlowBrowser)
		}

		if config.Secret != "" || config.PfxPath != "" || config.Thumbprint != "" {
			if config.BearerToken != "" {
				return fmt.Errorf("multiple authentication methods provided: in delegated mode with --bearertoken, do not combine with --secret, --pfx, or --thumbprint")
			}
			return fmt.Errorf("delegated mode without --bearertoken does not support --secret, --pfx, or --thumbprint")
		}
		return nil
	}

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

	if config.PfxPath != "" {
		if err := validateFilePath(config.PfxPath, "PFX certificate file"); err != nil {
			return err
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
