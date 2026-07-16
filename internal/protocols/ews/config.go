package ews

import (
	"fmt"
	"strings"
	"time"

	"github.com/ehlo-pl/gomailtesttool/internal/common/network"
	"github.com/ehlo-pl/gomailtesttool/internal/common/validation"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Config holds all ewstool configuration.
type Config struct {
	// Core configuration
	Action string

	// EWS server configuration
	Host             string
	Port             int
	EWSPath          string // default /EWS/Exchange.asmx
	AutodiscoverPath string // default /autodiscover/autodiscover.svc
	Timeout          time.Duration

	// Authentication
	Username    string
	Password    string
	AccessToken string // OAuth2 Bearer token
	AuthMethod  string // NTLM, Basic, Bearer, auto
	Domain      string // AD domain for NTLM (can be extracted from DOMAIN\user)
	Mailbox     string // Target mailbox for impersonation (optional)

	// Email composition (sendmail)
	To      []string
	Cc      []string
	Bcc     []string
	Subject string
	Body    string
	BodyHTML string

	// Calendar (getevents, sendinvite, getschedule)
	StartTime string
	EndTime   string

	// Search / export (exportmessages)
	MessageID string
	ExportDir string

	// Pagination (listmail, exportmessages)
	Count int

	// Meeting slot search (findtimeslot)
	Duration int // slot length in minutes

	// TLS configuration
	SkipVerify bool
	TLSVersion string // 1.2, 1.3

	// Network configuration
	ProxyURL string
	IPv4Only bool // Force resolving --host to an IPv4 (A record) address
	IPv6Only bool // Force resolving --host to an IPv6 (AAAA record) address

	// Runtime configuration
	VerboseMode bool
	LogLevel    string
	LogFormat   string // csv, json
}

// Action constants
const (
	ActionTestConnect    = "testconnect"
	ActionTestAuth       = "testauth"
	ActionGetFolder      = "getfolder"
	ActionAutodiscover   = "autodiscover"
	ActionListFolders    = "listfolders"
	ActionListMail       = "listmail"
	ActionSendMail       = "sendmail"
	ActionExportMessages = "exportmessages"
	ActionFindTimeSlot   = "findtimeslot"
	ActionGetEvents      = "getevents"
	ActionSendInvite     = "sendinvite"
	ActionGetSchedule    = "getschedule"
)

// NewConfig creates a new Config with default values.
func NewConfig() *Config {
	return &Config{
		Port:             443,
		EWSPath:          "/EWS/Exchange.asmx",
		AutodiscoverPath: "/autodiscover/autodiscover.svc",
		Timeout:          30 * time.Second,
		AuthMethod:       "auto",
		SkipVerify:       false,
		TLSVersion:       "1.2",
		VerboseMode:      false,
		LogLevel:         "INFO",
		LogFormat:        "csv",
	}
}

// RegisterPersistentFlags registers flags shared by all ews subcommands.
func RegisterPersistentFlags(cmd *cobra.Command) {
	f := cmd.PersistentFlags()

	// EWS server
	f.String("host", "", "Exchange server hostname or IP address (env: EWSHOST)")
	f.Int("port", 443, "HTTPS port (env: EWSPORT)")
	f.Int("timeout", 30, "Connection timeout in seconds (env: EWSTIMEOUT)")
	f.String("ewspath", "/EWS/Exchange.asmx", "EWS endpoint path (env: EWSPATH)")
	f.String("autodiscoverpath", "/autodiscover/autodiscover.svc", "Autodiscover endpoint path (env: EWSAUTODISCOVERPATH)")

	// Authentication
	f.String("username", "", "Username: DOMAIN\\user for NTLM, email for Basic/Bearer (env: EWSUSERNAME)")
	f.String("password", "", "Password (env: EWSPASSWORD)")
	f.String("accesstoken", "", "OAuth2 Bearer token (env: EWSACCESSTOKEN)")
	f.String("authmethod", "auto", "Auth method: NTLM, Basic, Bearer, auto (env: EWSAUTHMETHOD)")
	f.String("domain", "", "AD domain for NTLM (optional, can be embedded in username as DOMAIN\\user) (env: EWSDOMAIN)")
	f.String("mailbox", "", "Target mailbox SMTP address for impersonation (optional) (env: EWSMAILBOX)")

	// TLS
	f.Bool("skipverify", false, "Skip TLS certificate verification — use for self-signed certs (env: EWSSKIPVERIFY)")
	f.String("tlsversion", "1.2", "Minimum TLS version: 1.2, 1.3 (env: EWSTLSVERSION)")

	// Network
	f.String("proxy", "", "HTTP/HTTPS proxy URL (env: EWSPROXY)")
	f.Bool("ipv4", false, "Force IPv4: resolve --host to an A record and connect over IPv4 (env: EWSIPV4)")
	f.Bool("ipv6", false, "Force IPv6: resolve --host to an AAAA record and connect over IPv6 (env: EWSIPV6)")

	// Output
	f.Bool("verbose", false, "Enable verbose output")
	f.String("loglevel", "INFO", "Logging level: DEBUG, INFO, WARN, ERROR")
	f.String("logformat", "csv", "Log file format: csv, json (env: EWSLOGFORMAT)")
}

// BindEnvs registers Viper environment variable bindings for all ews config keys.
func BindEnvs(v *viper.Viper) {
	bindings := map[string]string{
		"host":             "EWSHOST",
		"port":             "EWSPORT",
		"timeout":          "EWSTIMEOUT",
		"ewspath":          "EWSPATH",
		"autodiscoverpath": "EWSAUTODISCOVERPATH",
		"username":         "EWSUSERNAME",
		"password":         "EWSPASSWORD",
		"accesstoken":      "EWSACCESSTOKEN",
		"authmethod":       "EWSAUTHMETHOD",
		"domain":           "EWSDOMAIN",
		"mailbox":          "EWSMAILBOX",
		"skipverify":       "EWSSKIPVERIFY",
		"tlsversion":       "EWSTLSVERSION",
		"to":               "EWSTO",
		"cc":               "EWSCC",
		"bcc":              "EWSBCC",
		"subject":          "EWSSUBJECT",
		"body":             "EWSBODY",
		"bodyhtml":         "EWSBODYHTML",
		"start":            "EWSSTART",
		"end":              "EWSEND",
		"messageid":        "EWSMESSAGEID",
		"exportdir":        "EWSEXPORTDIR",
		"count":            "EWSCOUNT",
		"duration":         "EWSDURATION",
		"proxy":            "EWSPROXY",
		"ipv4":             "EWSIPV4",
		"ipv6":             "EWSIPV6",
		"logformat":        "EWSLOGFORMAT",
	}
	for key, env := range bindings {
		_ = v.BindEnv(key, env)
	}
}

// ConfigFromViper reads all ews config values from the given Viper instance.
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

	authMethod := v.GetString("authmethod")
	if authMethod == "" {
		authMethod = defaults.AuthMethod
	}

	tlsVersion := v.GetString("tlsversion")
	if tlsVersion == "" {
		tlsVersion = defaults.TLSVersion
	}

	ewsPath := v.GetString("ewspath")
	if ewsPath == "" {
		ewsPath = defaults.EWSPath
	}

	autodiscoverPath := v.GetString("autodiscoverpath")
	if autodiscoverPath == "" {
		autodiscoverPath = defaults.AutodiscoverPath
	}

	logLevel := v.GetString("loglevel")
	if logLevel == "" {
		logLevel = defaults.LogLevel
	}

	logFormat := strings.ToLower(v.GetString("logformat"))
	if logFormat == "" {
		logFormat = defaults.LogFormat
	}

	subject := v.GetString("subject")
	if subject == "" {
		subject = "Automated Tool Notification"
	}

	body := v.GetString("body")
	if body == "" {
		body = "It's a test message, please ignore"
	}

	count := v.GetInt("count")
	if count <= 0 {
		count = 10
	}

	duration := v.GetInt("duration")
	if duration <= 0 {
		duration = 30
	}

	return &Config{
		Host:             v.GetString("host"),
		Port:             port,
		EWSPath:          ewsPath,
		AutodiscoverPath: autodiscoverPath,
		Timeout:          time.Duration(timeoutSec) * time.Second,
		Username:         v.GetString("username"),
		Password:         v.GetString("password"),
		AccessToken:      v.GetString("accesstoken"),
		AuthMethod:       authMethod,
		Domain:           v.GetString("domain"),
		Mailbox:          v.GetString("mailbox"),
		To:               parseStringSlice(v.GetString("to")),
		Cc:               parseStringSlice(v.GetString("cc")),
		Bcc:              parseStringSlice(v.GetString("bcc")),
		Subject:          subject,
		Body:             body,
		BodyHTML:         v.GetString("bodyhtml"),
		StartTime:        v.GetString("start"),
		EndTime:          v.GetString("end"),
		MessageID:        v.GetString("messageid"),
		ExportDir:        v.GetString("exportdir"),
		Count:            count,
		Duration:         duration,
		SkipVerify:       v.GetBool("skipverify"),
		TLSVersion:       tlsVersion,
		ProxyURL:         v.GetString("proxy"),
		IPv4Only:         v.GetBool("ipv4"),
		IPv6Only:         v.GetBool("ipv6"),
		VerboseMode:      v.GetBool("verbose"),
		LogLevel:         logLevel,
		LogFormat:        logFormat,
	}
}

// parseStringSlice splits a comma-separated string into a trimmed slice.
func parseStringSlice(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			result = append(result, t)
		}
	}
	return result
}

// validateConfiguration validates the EWS configuration and resolves auto auth method.
func validateConfiguration(config *Config) error {
	validActions := []string{
		ActionTestConnect, ActionTestAuth, ActionGetFolder, ActionAutodiscover,
		ActionListFolders, ActionListMail, ActionSendMail, ActionExportMessages,
		ActionGetEvents, ActionSendInvite, ActionGetSchedule, ActionFindTimeSlot,
	}
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

	if config.Host == "" {
		return fmt.Errorf("host is required (--host flag)")
	}
	if err := validation.ValidateHostname(config.Host); err != nil {
		return fmt.Errorf("invalid host: %w", err)
	}
	if err := validation.ValidatePort(config.Port); err != nil {
		return fmt.Errorf("invalid port: %w", err)
	}
	if err := validation.ValidateProxyURL(config.ProxyURL); err != nil {
		return fmt.Errorf("invalid proxy URL: %w", err)
	}

	// Validate mutual exclusion: --ipv4 and --ipv6 cannot be used together
	if err := network.ValidateIPVersionFlags(config.IPv4Only, config.IPv6Only); err != nil {
		return err
	}

	// Resolve "auto" auth method
	if strings.EqualFold(config.AuthMethod, "auto") {
		config.AuthMethod = resolveAuthMethod(config)
	}

	// Action-specific validation
	switch config.Action {
	case ActionTestAuth, ActionGetFolder, ActionListFolders, ActionListMail,
		ActionSendMail, ActionExportMessages,
		ActionGetEvents, ActionSendInvite, ActionGetSchedule, ActionFindTimeSlot:
		if config.Username == "" {
			return fmt.Errorf("%s requires --username", config.Action)
		}
		if strings.EqualFold(config.AuthMethod, "Bearer") {
			if config.AccessToken == "" {
				return fmt.Errorf("bearer authentication requires --accesstoken")
			}
		} else if config.Password == "" {
			return fmt.Errorf("%s requires --password (or --accesstoken for Bearer)", config.Action)
		}

	case ActionAutodiscover:
		if config.Username == "" {
			return fmt.Errorf("autodiscover requires --username (email address)")
		}
		if err := validation.ValidateEmail(config.Username); err != nil {
			return fmt.Errorf("autodiscover --username must be an email address: %w", err)
		}
	}

	if config.Action == ActionSendMail {
		if len(config.To) == 0 {
			return fmt.Errorf("sendmail requires at least one recipient (--to)")
		}
	}

	if config.Action == ActionExportMessages {
		if config.MessageID == "" && strings.TrimSpace(config.Subject) == "" {
			return fmt.Errorf("exportmessages requires --messageid and/or --subject")
		}
	}

	if config.Action == ActionSendInvite {
		if len(config.To) == 0 {
			return fmt.Errorf("sendinvite requires at least one attendee (--to)")
		}
		if config.StartTime == "" || config.EndTime == "" {
			return fmt.Errorf("sendinvite requires --start and --end parameters")
		}
	}

	if config.Action == ActionGetSchedule || config.Action == ActionFindTimeSlot {
		if len(config.To) == 0 {
			return fmt.Errorf("%s requires --to parameter (recipient email address)", config.Action)
		}
		if len(config.To) > 1 {
			return fmt.Errorf("%s only supports checking one recipient at a time (got %d recipients)", config.Action, len(config.To))
		}
	}

	if config.Action == ActionFindTimeSlot {
		if config.Duration < 5 || config.Duration > 480 {
			return fmt.Errorf("invalid --duration: %d (must be 5-480 minutes)", config.Duration)
		}
	}

	// Validate time formats when provided (getevents defaults both).
	if config.Action == ActionGetEvents || config.Action == ActionSendInvite ||
		config.Action == ActionGetSchedule || config.Action == ActionFindTimeSlot {
		if config.StartTime != "" {
			if _, err := parseFlexibleTime(config.StartTime); err != nil {
				return fmt.Errorf("invalid --start: %w", err)
			}
		}
		if config.EndTime != "" {
			if _, err := parseFlexibleTime(config.EndTime); err != nil {
				return fmt.Errorf("invalid --end: %w", err)
			}
		}
	}

	return nil
}

// resolveAuthMethod determines the auth method from config when set to "auto".
func resolveAuthMethod(config *Config) string {
	if config.AccessToken != "" {
		return "Bearer"
	}
	if strings.Contains(config.Username, `\`) || config.Domain != "" {
		return "NTLM"
	}
	return "Basic"
}
