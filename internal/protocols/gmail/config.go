package gmail

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/ehlo-pl/gomailtesttool/internal/common/email"
	"github.com/ehlo-pl/gomailtesttool/internal/common/validation"
)

// Config holds all configuration for the gmail provider: command-line flags,
// environment variables, and runtime state. It mirrors the msgraph Config,
// adapted to Google Workspace / Gmail authentication and actions.
type Config struct {
	// Target
	Mailbox string // Impersonated user (DWD Subject) / calendar owner
	Action  string // Operation to perform

	// Authentication (mutually exclusive)
	CredentialsFile  string      // Path to service-account JSON key (DWD, primary)
	BearerToken      string      // Pre-obtained OAuth2 access token
	OAuth            bool        // Use the interactive OAuth loopback flow
	OAuthCredentials string      // Path to OAuth client-secret JSON (for --oauth)
	TokenCache       string      // Path to cache the OAuth token (for --oauth)
	Scopes           stringSlice // Explicit scope override (else per-action defaults)

	// Email recipients
	To                    stringSlice
	Cc                    stringSlice
	Bcc                   stringSlice
	AttachmentFiles       stringSlice
	InlineAttachmentFiles stringSlice
	Headers               []string

	// Email content
	Subject      string
	Body         string
	BodyHTML     string
	BodyTemplate string
	Priority     string

	// Calendar
	StartTime string
	EndTime   string

	// Label selection (listmail)
	Label string // Gmail label ID for listmail (default: INBOX)

	// Search / export
	MessageID   string
	SearchQuery string // Raw Gmail search query (overrides messageid/subject search)
	ExportDir   string

	// Network
	ProxyURL   string
	MaxRetries int
	RetryDelay time.Duration

	// Runtime
	VerboseMode  bool
	LogLevel     string
	OutputFormat string
	LogFormat    string
	Count        int
}

// NewConfig creates a new Config with sensible default values.
func NewConfig() *Config {
	return &Config{
		Subject:      "Automated Tool Notification",
		Body:         "It's a test message, please ignore",
		Priority:     "normal",
		Action:       ActionGetInbox,
		Count:        3,
		VerboseMode:  false,
		LogLevel:     "INFO",
		OutputFormat: "text",
		LogFormat:    "csv",
		MaxRetries:   3,
		RetryDelay:   2000 * time.Millisecond,
	}
}

// Action constants
const (
	ActionSendMail          = "sendmail"
	ActionGetInbox          = "getinbox"
	ActionListFolders       = "listfolders"
	ActionListMail          = "listmail"
	ActionExportMessages    = "exportmessages"
	ActionGetEvents         = "getevents"
	ActionSendInvite        = "sendinvite"
	ActionGetSchedule       = "getschedule"
	ActionTestAuth          = "testauth"
	ActionExportBearerToken = "exportbearertoken"
	ActionTestConnect       = "testconnect"
)

// RegisterPersistentFlags registers flags shared by all gmail subcommands on
// the given parent command (the "gmail" cobra.Command).
func RegisterPersistentFlags(cmd *cobra.Command) {
	f := cmd.PersistentFlags()

	// Authentication
	f.String("credentials", "", "Path to the service-account JSON key for domain-wide delegation (env: GMAILCREDENTIALS)")
	f.String("bearertoken", "", "Pre-obtained OAuth2 access token (env: GMAILBEARERTOKEN)")
	f.Bool("oauth", false, "Use the interactive OAuth loopback flow (env: GMAILOAUTH)")
	f.String("oauth-credentials", "", "Path to the OAuth client-secret JSON, used with --oauth (env: GMAILOAUTHCREDENTIALS)")
	f.String("token-cache", "", "Path to cache the OAuth token, used with --oauth (env: GMAILTOKENCACHE)")
	f.String("scope", "", "Comma-separated OAuth scopes overriding the per-action defaults (env: GMAILSCOPE)")

	// Target
	f.String("mailbox", "", "The impersonated Workspace user / calendar owner (env: GMAILMAILBOX)")

	// Network
	f.String("proxy", "", "HTTP/HTTPS proxy URL (e.g., http://proxy.example.com:8080) (env: GMAILPROXY)")
	f.Int("maxretries", 3, "Maximum retry attempts for transient failures (env: GMAILMAXRETRIES)")
	f.Int("retrydelay", 2000, "Base delay between retries in milliseconds (env: GMAILRETRYDELAY)")

	// Output
	f.Bool("verbose", false, "Enable verbose output (env: -)")
	f.String("loglevel", "INFO", "Logging level: DEBUG, INFO, WARN, ERROR (env: GMAILLOGLEVEL)")
	f.String("output", "text", "Output format: text, json (env: GMAILOUTPUT)")
	f.String("logformat", "csv", "Log file format: csv, json (env: GMAILLOGFORMAT)")
}

// BindEnvs registers Viper environment variable bindings for all gmail config
// keys. Must be called after RegisterPersistentFlags.
func BindEnvs(v *viper.Viper) {
	bindings := map[string]string{
		"credentials":        "GMAILCREDENTIALS",
		"bearertoken":        "GMAILBEARERTOKEN",
		"oauth":              "GMAILOAUTH",
		"oauth-credentials":  "GMAILOAUTHCREDENTIALS",
		"token-cache":        "GMAILTOKENCACHE",
		"scope":              "GMAILSCOPE",
		"mailbox":            "GMAILMAILBOX",
		"to":                 "GMAILTO",
		"cc":                 "GMAILCC",
		"bcc":                "GMAILBCC",
		"subject":            "GMAILSUBJECT",
		"body":               "GMAILBODY",
		"bodyhtml":           "GMAILBODYHTML",
		"priority":           "GMAILPRIORITY",
		"body-template":      "GMAILBODYTEMPLATE",
		"attachments":        "GMAILATTACHMENTS",
		"inline-attachments": "GMAILINLINEATTACHMENTS",
		"start":              "GMAILSTART",
		"end":                "GMAILEND",
		"label":              "GMAILLABEL",
		"messageid":          "GMAILMESSAGEID",
		"search":             "GMAILSEARCH",
		"exportdir":          "GMAILEXPORTDIR",
		"proxy":              "GMAILPROXY",
		"maxretries":         "GMAILMAXRETRIES",
		"retrydelay":         "GMAILRETRYDELAY",
		"loglevel":           "GMAILLOGLEVEL",
		"output":             "GMAILOUTPUT",
		"logformat":          "GMAILLOGFORMAT",
		"count":              "GMAILCOUNT",
		"header":             "GMAILHEADER",
	}
	for key, env := range bindings {
		_ = v.BindEnv(key, env)
	}
}

// ConfigFromViper reads all gmail config values from the given Viper instance.
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
		Mailbox:               v.GetString("mailbox"),
		CredentialsFile:       v.GetString("credentials"),
		BearerToken:           v.GetString("bearertoken"),
		OAuth:                 v.GetBool("oauth"),
		OAuthCredentials:      v.GetString("oauth-credentials"),
		TokenCache:            v.GetString("token-cache"),
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
		BodyTemplate:          v.GetString("body-template"),
		Priority:              priority,
		StartTime:             v.GetString("start"),
		EndTime:               v.GetString("end"),
		Label:                 v.GetString("label"),
		MessageID:             v.GetString("messageid"),
		SearchQuery:           v.GetString("search"),
		ExportDir:             v.GetString("exportdir"),
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

// authMethodCount returns how many mutually-exclusive auth methods are set.
func (c *Config) authMethodCount() int {
	n := 0
	if c.CredentialsFile != "" {
		n++
	}
	if c.BearerToken != "" {
		n++
	}
	if c.OAuth {
		n++
	}
	return n
}

// validateConfiguration validates all required configuration fields.
func validateConfiguration(config *Config) error {
	// Exactly one authentication method.
	switch config.authMethodCount() {
	case 0:
		return fmt.Errorf("missing authentication: must provide one of --credentials, --bearertoken, or --oauth")
	case 1:
		// ok
	default:
		return fmt.Errorf("multiple authentication methods provided: use only one of --credentials, --bearertoken, or --oauth")
	}

	// Validate auth file paths.
	if config.CredentialsFile != "" {
		if err := validateFilePath(config.CredentialsFile, "service-account credentials file"); err != nil {
			return err
		}
	}
	if config.OAuth {
		if config.OAuthCredentials == "" {
			return fmt.Errorf("--oauth requires --oauth-credentials (the OAuth client-secret JSON)")
		}
		if err := validateFilePath(config.OAuthCredentials, "OAuth client-secret file"); err != nil {
			return err
		}
	}

	// The mailbox is the mandatory DWD Subject for service accounts and the
	// impersonated user for the interactive flow. A pre-obtained bearer token
	// already encodes its user ("me"), so the mailbox is optional there.
	if config.CredentialsFile != "" || config.OAuth {
		if config.Mailbox == "" {
			return fmt.Errorf("--mailbox is required: it is the impersonated user (domain-wide delegation Subject)")
		}
	}
	if config.Mailbox != "" {
		if err := validateEmail(config.Mailbox); err != nil {
			return fmt.Errorf("invalid mailbox: %w", err)
		}
	}

	// Content validation shared with sendmail.
	for i, attachmentPath := range config.AttachmentFiles {
		if err := validateFilePath(attachmentPath, fmt.Sprintf("Attachment file #%d", i+1)); err != nil {
			return err
		}
	}
	for i, attachmentPath := range config.InlineAttachmentFiles {
		if err := validateFilePath(attachmentPath, fmt.Sprintf("Inline attachment file #%d", i+1)); err != nil {
			return err
		}
	}
	if _, err := email.ParseHeaders(config.Headers); err != nil {
		return fmt.Errorf("invalid --header: %w", err)
	}
	if config.BodyTemplate != "" {
		if err := validateFilePath(config.BodyTemplate, "Body template file"); err != nil {
			return err
		}
	}
	switch config.Priority {
	case "high", "normal", "low":
	default:
		return fmt.Errorf("invalid --priority: %s (must be one of: high, normal, low)", config.Priority)
	}
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
	if err := validateRFC3339Time(config.StartTime, "Start time"); err != nil {
		return err
	}
	if err := validateRFC3339Time(config.EndTime, "End time"); err != nil {
		return err
	}
	if config.OutputFormat != "text" && config.OutputFormat != "json" {
		return fmt.Errorf("invalid output format: %s (use: text, json)", config.OutputFormat)
	}

	// Per-action requirements.
	switch config.Action {
	case ActionExportMessages:
		if config.MessageID == "" && strings.TrimSpace(config.Subject) == "" && strings.TrimSpace(config.SearchQuery) == "" {
			return fmt.Errorf("exportmessages action requires --messageid, --subject and/or --search parameter")
		}
		// SECURITY: validate the search inputs before they are placed into a
		// Gmail search query (rfc822msgid:/subject:) to block operator injection.
		if config.MessageID != "" {
			if err := validateMessageID(config.MessageID); err != nil {
				return fmt.Errorf("invalid message ID: %w", err)
			}
		}
		if config.Subject != "" {
			if err := validateSearchSubject(config.Subject); err != nil {
				return fmt.Errorf("invalid subject: %w", err)
			}
		}
	case ActionSendInvite:
		if config.StartTime == "" || config.EndTime == "" {
			return fmt.Errorf("sendinvite action requires --start and --end parameters")
		}
	case ActionGetSchedule:
		if len(config.To) == 0 {
			return fmt.Errorf("getschedule action requires --to parameter (recipient email address)")
		}
		if len(config.To) > 1 {
			return fmt.Errorf("getschedule action only supports checking one recipient at a time (got %d recipients)", len(config.To))
		}
	}

	return nil
}

// validateTestConnectConfiguration validates config for the testconnect action.
// testconnect is an unauthenticated network/TLS probe against the Gmail API
// endpoint, so it needs no credentials, mailbox, or tenant — only a valid output
// format (any proxy is validated at dial time).
func validateTestConnectConfiguration(config *Config) error {
	if config.OutputFormat != "text" && config.OutputFormat != "json" {
		return fmt.Errorf("invalid output format: %s (use: text, json)", config.OutputFormat)
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
func validateEmail(addr string) error {
	return validation.ValidateEmail(addr)
}

// validateEmails wraps the common validation package.
func validateEmails(emails []string, fieldName string) error {
	return validation.ValidateEmails(emails, fieldName)
}

// validateFilePath wraps the common validation package.
func validateFilePath(path, fieldName string) error {
	return validation.ValidateFilePath(path, fieldName)
}

// validateRFC3339Time validates that a string matches RFC3339 or the PowerShell
// sortable timestamp format. An empty string is allowed (defaults are used).
func validateRFC3339Time(timeStr, fieldName string) error {
	if timeStr == "" {
		return nil
	}
	if _, err := parseFlexibleTime(timeStr); err != nil {
		return fmt.Errorf("%s: %w", fieldName, err)
	}
	return nil
}
