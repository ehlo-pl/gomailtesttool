package msgraph

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/ehlo-pl/gomailtesttool/internal/common/bootstrap"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// NewCmd returns the "msgraph" cobra.Command with all action subcommands.
// Each subcommand shares persistent flags (auth, mailbox, output) and adds
// its own action-specific flags.
func NewCmd() *cobra.Command {
	v := viper.New()

	cmd := &cobra.Command{
		Use:   "msgraph",
		Short: "Microsoft Graph API (Exchange Online) operations",
		Long: `Interact with Microsoft Exchange Online via the Microsoft Graph API.

Supports sending emails, listing calendar events, checking availability,
exporting inbox messages, and searching by Message-ID.

Authentication methods: --secret (client secret), --pfx (certificate file),
--thumbprint (Windows certificate store), --bearertoken (pre-obtained token).
Delegated permissions: use --delegated with --authflow devicecode|browser
(browser flow requires --redirecturl).`,
	}

	RegisterPersistentFlags(cmd)
	BindEnvs(v)

	cmd.AddCommand(
		newGetEventsCmd(v),
		newSendMailCmd(v),
		newSendInviteCmd(v),
		newGetInboxCmd(v),
		newListFoldersCmd(v),
		newListMailCmd(v),
		newGetScheduleCmd(v),
		newExportInboxCmd(v),
		newSearchAndExportCmd(v),
		newExportMessagesCmd(v),
		newExportBearerTokenCmd(v),
		newTestConnectCmd(v),
		newTestAuthCmd(v),
	)

	return cmd
}

func newGetEventsCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "getevents",
		Short: "List upcoming calendar events for a mailbox",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}

			config := ConfigFromViper(v)
			config.Action = ActionGetEvents

			if err := validateConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger, csvLogger, logErr := bootstrap.InitLoggers("msgraphtool", ActionGetEvents, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			if config.ProxyURL != "" {
				_ = os.Setenv("HTTP_PROXY", config.ProxyURL)
				_ = os.Setenv("HTTPS_PROXY", config.ProxyURL)
			}

			client, err := NewGraphServiceClient(ctx, config, slogger)
			if err != nil {
				return err
			}

			return listEvents(ctx, client, config.Mailbox, config.Count, config, csvLogger)
		},
	}
	cmd.Flags().Int("count", 3, "Number of events to retrieve (env: MSGRAPHCOUNT)")
	return cmd
}

func newSendMailCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sendmail",
		Short: "Send an email via Microsoft Graph",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}

			config := ConfigFromViper(v)
			config.Action = ActionSendMail

			if err := validateConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger, csvLogger, logErr := bootstrap.InitLoggers("msgraphtool", ActionSendMail, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			if config.ProxyURL != "" {
				_ = os.Setenv("HTTP_PROXY", config.ProxyURL)
				_ = os.Setenv("HTTPS_PROXY", config.ProxyURL)
			}

			// Load body template if provided
			if config.BodyTemplate != "" {
				content, err := os.ReadFile(config.BodyTemplate)
				if err != nil {
					return fmt.Errorf("failed to read body template file: %w", err)
				}
				config.BodyHTML = string(content)
			}

			client, err := NewGraphServiceClient(ctx, config, slogger)
			if err != nil {
				return err
			}

			// Default To to mailbox if no recipients specified
			if len(config.To) == 0 && len(config.Cc) == 0 && len(config.Bcc) == 0 {
				config.To = stringSlice{config.Mailbox}
			}

			return SendEmail(ctx, client, config.Mailbox, config.To, config.Cc, config.Bcc, config.Subject, config.Body, config.BodyHTML, config.AttachmentFiles, config, csvLogger)
		},
	}
	cmd.Flags().String("to", "", "Comma-separated TO recipients (env: MSGRAPHTO)")
	cmd.Flags().String("cc", "", "Comma-separated CC recipients (env: MSGRAPHCC)")
	cmd.Flags().String("bcc", "", "Comma-separated BCC recipients (env: MSGRAPHBCC)")
	cmd.Flags().String("subject", "Automated Tool Notification", "Email subject (env: MSGRAPHSUBJECT)")
	cmd.Flags().String("body", "It's a test message, please ignore", "Email body text (env: MSGRAPHBODY)")
	cmd.Flags().String("bodyhtml", "", "HTML body content (env: MSGRAPHBODYHTML)")
	cmd.Flags().String("body-template", "", "Path to HTML email body template file (env: MSGRAPHBODYTEMPLATE)")
	cmd.Flags().String("attachments", "", "Comma-separated file paths to attach (env: MSGRAPHATTACHMENTS)")
	cmd.Flags().String("inline-attachments", "", "Comma-separated file paths to embed inline via cid:<filename> (env: MSGRAPHINLINEATTACHMENTS)")
	cmd.Flags().StringArray("header", nil, "Custom header in 'Name: Value' form (repeatable) (env: MSGRAPHHEADER — comma-separated; avoid commas in header values)")
	cmd.Flags().String("priority", "normal", "Email priority/importance: high, normal, low (env: MSGRAPHPRIORITY)")
	return cmd
}

func newSendInviteCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sendinvite",
		Short: "Create a calendar meeting invitation",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}

			config := ConfigFromViper(v)
			config.Action = ActionSendInvite

			if err := validateConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger, csvLogger, logErr := bootstrap.InitLoggers("msgraphtool", ActionSendInvite, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			if config.ProxyURL != "" {
				_ = os.Setenv("HTTP_PROXY", config.ProxyURL)
				_ = os.Setenv("HTTPS_PROXY", config.ProxyURL)
			}

			client, err := NewGraphServiceClient(ctx, config, slogger)
			if err != nil {
				return err
			}

			// Resolve invite subject (backward compat: --invite-subject overrides --subject)
			inviteSubject := config.Subject
			if config.InviteSubject != "" {
				inviteSubject = config.InviteSubject
			}
			if inviteSubject == "Automated Tool Notification" {
				inviteSubject = "It's testing event"
			}

			createInvite(ctx, client, config.Mailbox, inviteSubject, config.StartTime, config.EndTime, config, csvLogger)
			return nil
		},
	}
	cmd.Flags().String("subject", "Automated Tool Notification", "Calendar invite subject (env: MSGRAPHSUBJECT)")
	cmd.Flags().String("invite-subject", "", "Deprecated: use --subject instead (env: MSGRAPHINVITESUBJECT)")
	cmd.Flags().String("start", "", "Start time (RFC3339 or PowerShell sortable format) (env: MSGRAPHSTART)")
	cmd.Flags().String("end", "", "End time (RFC3339 or PowerShell sortable format) (env: MSGRAPHEND)")
	return cmd
}

func newGetInboxCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:        "getinbox",
		Short:      "List recent inbox messages for a mailbox",
		Deprecated: "use 'listmail --folder inbox' instead",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}

			config := ConfigFromViper(v)
			config.Action = ActionGetInbox

			if err := validateConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger, csvLogger, logErr := bootstrap.InitLoggers("msgraphtool", ActionGetInbox, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			if config.ProxyURL != "" {
				_ = os.Setenv("HTTP_PROXY", config.ProxyURL)
				_ = os.Setenv("HTTPS_PROXY", config.ProxyURL)
			}

			client, err := NewGraphServiceClient(ctx, config, slogger)
			if err != nil {
				return err
			}

			return listMailInFolder(ctx, client, config.Mailbox, "inbox", config.Count, config, csvLogger)
		},
	}
	cmd.Flags().Int("count", 3, "Number of messages to retrieve (env: MSGRAPHCOUNT)")
	return cmd
}

func newListFoldersCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "listfolders",
		Short: "List mail folders for a mailbox",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}

			config := ConfigFromViper(v)
			config.Action = ActionListFolders

			if err := validateConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger, csvLogger, logErr := bootstrap.InitLoggers("msgraphtool", ActionListFolders, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			if config.ProxyURL != "" {
				_ = os.Setenv("HTTP_PROXY", config.ProxyURL)
				_ = os.Setenv("HTTPS_PROXY", config.ProxyURL)
			}

			client, err := NewGraphServiceClient(ctx, config, slogger)
			if err != nil {
				return err
			}

			return listMailFolders(ctx, client, config.Mailbox, config, csvLogger)
		},
	}
	return cmd
}

func newListMailCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "listmail",
		Short: "List messages from a mailbox folder (default: inbox)",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}

			config := ConfigFromViper(v)
			config.Action = ActionListMail

			if config.Folder == "" {
				config.Folder = "inbox"
			}

			if err := validateConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger, csvLogger, logErr := bootstrap.InitLoggers("msgraphtool", ActionListMail, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			if config.ProxyURL != "" {
				_ = os.Setenv("HTTP_PROXY", config.ProxyURL)
				_ = os.Setenv("HTTPS_PROXY", config.ProxyURL)
			}

			client, err := NewGraphServiceClient(ctx, config, slogger)
			if err != nil {
				return err
			}

			return listMailInFolder(ctx, client, config.Mailbox, config.Folder, config.Count, config, csvLogger)
		},
	}
	cmd.Flags().String("folder", "inbox", "Mail folder to list messages from (well-known names: inbox, sentitems, drafts, deleteditems, junkemail) (env: MSGRAPHFOLDER)")
	cmd.Flags().Int("count", 3, "Number of messages to retrieve (env: MSGRAPHCOUNT)")
	return cmd
}

func newGetScheduleCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "getschedule",
		Short: "Check a recipient's availability for the next working day",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}

			config := ConfigFromViper(v)
			config.Action = ActionGetSchedule

			if err := validateConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger, csvLogger, logErr := bootstrap.InitLoggers("msgraphtool", ActionGetSchedule, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			if config.ProxyURL != "" {
				_ = os.Setenv("HTTP_PROXY", config.ProxyURL)
				_ = os.Setenv("HTTPS_PROXY", config.ProxyURL)
			}

			client, err := NewGraphServiceClient(ctx, config, slogger)
			if err != nil {
				return err
			}

			return checkAvailability(ctx, client, config.Mailbox, config.To[0], config, csvLogger)
		},
	}
	cmd.Flags().String("to", "", "Recipient email address to check availability for (env: MSGRAPHTO)")
	return cmd
}

func newExportInboxCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:        "exportinbox",
		Short:      "Export inbox messages to JSON files",
		Deprecated: "use 'exportmessages --folder inbox' instead (note: exportmessages writes .eml, exportinbox writes JSON)",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}

			config := ConfigFromViper(v)
			config.Action = ActionExportInbox

			if err := validateConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger, csvLogger, logErr := bootstrap.InitLoggers("msgraphtool", ActionExportInbox, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			if config.ProxyURL != "" {
				_ = os.Setenv("HTTP_PROXY", config.ProxyURL)
				_ = os.Setenv("HTTPS_PROXY", config.ProxyURL)
			}

			client, err := NewGraphServiceClient(ctx, config, slogger)
			if err != nil {
				return err
			}

			return exportInbox(ctx, client, config.Mailbox, config.Count, config, csvLogger)
		},
	}
	cmd.Flags().Int("count", 3, "Number of messages to export (env: MSGRAPHCOUNT)")
	cmd.Flags().String("exportdir", "", "Directory under which to create the dated export folder; defaults to the OS temp directory (env: MSGRAPHEXPORTDIR)")
	return cmd
}

func newSearchAndExportCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:        "searchandexport",
		Short:      "Search for a message by Internet Message-ID and export it",
		Deprecated: "use 'exportmessages --messageid' instead (note: exportmessages writes .eml, searchandexport writes JSON)",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}

			config := ConfigFromViper(v)
			config.Action = ActionSearchAndExport

			if err := validateConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger, csvLogger, logErr := bootstrap.InitLoggers("msgraphtool", ActionSearchAndExport, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			if config.ProxyURL != "" {
				_ = os.Setenv("HTTP_PROXY", config.ProxyURL)
				_ = os.Setenv("HTTPS_PROXY", config.ProxyURL)
			}

			client, err := NewGraphServiceClient(ctx, config, slogger)
			if err != nil {
				return err
			}

			return searchAndExport(ctx, client, config.Mailbox, config.MessageID, config, csvLogger)
		},
	}
	cmd.Flags().String("messageid", "", "Internet Message-ID to search for (env: MSGRAPHMESSAGEID)")
	cmd.Flags().String("exportdir", "", "Directory under which to create the dated export folder; defaults to the OS temp directory (env: MSGRAPHEXPORTDIR)")
	return cmd
}

func newExportMessagesCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exportmessages",
		Short: "Search messages by Message-ID and/or Subject and export them as .eml files",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}

			config := ConfigFromViper(v)
			config.Action = ActionExportMessages
			// "subject" is shared with sendmail/sendinvite, whose default
			// ("Automated Tool Notification") would otherwise narrow the
			// search; only use it here if the user actually provided it.
			config.Subject = v.GetString("subject")

			if err := validateConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger, csvLogger, logErr := bootstrap.InitLoggers("msgraphtool", ActionExportMessages, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			if config.ProxyURL != "" {
				_ = os.Setenv("HTTP_PROXY", config.ProxyURL)
				_ = os.Setenv("HTTPS_PROXY", config.ProxyURL)
			}

			client, err := NewGraphServiceClient(ctx, config, slogger)
			if err != nil {
				return err
			}

			return exportMessages(ctx, client, config.Mailbox, config.MessageID, config.Subject, config.Folder, config.Count, config, csvLogger)
		},
	}
	cmd.Flags().String("messageid", "", "Internet Message-ID to search for (env: MSGRAPHMESSAGEID)")
	cmd.Flags().String("subject", "", "Subject substring to search for, used with OData contains() (env: MSGRAPHSUBJECT)")
	cmd.Flags().String("folder", "", "Mail folder to scope the search to (well-known names: inbox, sentitems, drafts, deleteditems, junkemail); with no other criteria exports the newest messages of the folder (env: MSGRAPHFOLDER)")
	cmd.Flags().Int("count", 25, "Maximum number of matching messages to export (env: MSGRAPHCOUNT)")
	cmd.Flags().String("exportdir", "", "Directory under which to create the dated export folder; defaults to the OS temp directory (env: MSGRAPHEXPORTDIR)")
	return cmd
}

func newExportBearerTokenCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exportbearertoken",
		Short: "Acquire and print a Microsoft Graph bearer token",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}

			config := ConfigFromViper(v)
			config.Action = ActionExportBearerToken

			if err := validateExportBearerTokenConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger := slog.Default()

			if config.ProxyURL != "" {
				_ = os.Setenv("HTTP_PROXY", config.ProxyURL)
				_ = os.Setenv("HTTPS_PROXY", config.ProxyURL)
			}

			cred, err := getCredential(config, slogger)
			if err != nil {
				return err
			}

			token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
				Scopes:    effectiveScopes(config),
				EnableCAE: true,
			})
			if err != nil {
				return fmt.Errorf("failed to acquire bearer token: %w", err)
			}

			if config.OutputFormat == "json" {
				printJSON(map[string]string{"bearertoken": token.Token})
			} else {
				fmt.Println(token.Token)
			}

			return nil
		},
	}

	return cmd
}

func newTestConnectCmd(v *viper.Viper) *cobra.Command {
	return &cobra.Command{
		Use:   "testconnect",
		Short: "Probe network/TLS reachability to the Microsoft Graph endpoint (no credentials)",
		Long: `Perform an unauthenticated HTTP/TLS probe against https://graph.microsoft.com.

No credentials, tenant/client IDs, or mailbox are required. Any HTTP response
(401 is expected) confirms the endpoint is reachable. Because Graph is a global
service, this test cannot verify authentication or permissions — its value is
catching proxy/firewall blocks and TLS interception (an unexpected certificate
Issuer indicates a TLS-intercepting proxy). Use --proxy to route through a proxy.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}

			config := ConfigFromViper(v)
			config.Action = ActionTestConnect

			if err := validateTestConnectConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger, csvLogger, logErr := bootstrap.InitLoggers("msgraphtool", ActionTestConnect, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			return testConnect(ctx, config, csvLogger, slogger)
		},
	}
}

func newTestAuthCmd(v *viper.Viper) *cobra.Command {
	return &cobra.Command{
		Use:   "testauth",
		Short: "Verify the configured credential by acquiring a Graph token",
		Long: `Acquire an access token from Microsoft Entra ID using the configured
credential (--secret, --pfx, --thumbprint, or --bearertoken). Successful token
acquisition is the authentication verdict; the token's claims (application name,
assigned roles) are displayed so you can see the granted application permissions.

No mailbox is required. If --mailbox is supplied, one lightweight authenticated
Graph call is made as end-to-end verification: a 403 there is reported as success
(the token was accepted; the app simply lacks User.Read.All), while a 401 is a
genuine authentication failure.

Note: with --bearertoken the token is not validated by acquisition (it is used
as-is, without contacting Entra ID), so pass --mailbox to verify it end to end.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}

			config := ConfigFromViper(v)
			config.Action = ActionTestAuth

			if err := validateTestAuthConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger, csvLogger, logErr := bootstrap.InitLoggers("msgraphtool", ActionTestAuth, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			if config.ProxyURL != "" {
				_ = os.Setenv("HTTP_PROXY", config.ProxyURL)
				_ = os.Setenv("HTTPS_PROXY", config.ProxyURL)
			}

			return testAuth(ctx, config, csvLogger, slogger)
		},
	}
}
