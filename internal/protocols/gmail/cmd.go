package gmail

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/ehlo-pl/gomailtesttool/internal/common/bootstrap"
	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
)

// NewCmd returns the "gmail" cobra.Command with all action subcommands. Each
// subcommand shares persistent flags (auth, mailbox, output) and adds its own
// action-specific flags.
func NewCmd() *cobra.Command {
	v := viper.New()

	cmd := &cobra.Command{
		Use:   "gmail",
		Short: "Google Workspace / Gmail API operations",
		Long: `Interact with Google Workspace via the Gmail and Calendar APIs.

Supports sending mail, listing and exporting messages, and basic calendar
operations against a Workspace user.

Authentication methods (pick one): --credentials (service account with
domain-wide delegation, impersonating --mailbox), --bearertoken (a pre-obtained
OAuth2 access token), or --oauth (interactive loopback flow).`,
	}

	RegisterPersistentFlags(cmd)
	BindEnvs(v)

	cmd.AddCommand(
		newSendMailCmd(v),
		newGetInboxCmd(v),
		newExportMessagesCmd(v),
		newGetEventsCmd(v),
		newSendInviteCmd(v),
		newTestAuthCmd(v),
		newExportBearerTokenCmd(v),
	)

	return cmd
}

// setup runs the shared subcommand lifecycle: bind flags, load the config file,
// build and validate the Config, set up the signal context, initialize loggers,
// and apply the proxy environment. Callers must defer cancel() and close the
// CSV logger.
func setup(cmd *cobra.Command, v *viper.Viper, action string) (*Config, context.Context, context.CancelFunc, *slog.Logger, logger.Logger, error) {
	_ = v.BindPFlags(cmd.Flags())
	_ = v.BindPFlags(cmd.InheritedFlags())

	if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
		return nil, nil, nil, nil, nil, err
	}

	config := ConfigFromViper(v)
	config.Action = action

	// "subject" is shared with sendmail/sendinvite, whose default would
	// otherwise narrow the exportmessages search; only use it if provided.
	if action == ActionExportMessages {
		config.Subject = v.GetString("subject")
	}

	if err := validateConfiguration(config); err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("validation failed: %w", err)
	}

	ctx, cancel := bootstrap.SetupSignalContext()

	slogger, csvLogger, logErr := bootstrap.InitLoggers("gmailtool", action, config.VerboseMode, config.LogLevel, config.LogFormat)
	if logErr != nil {
		slogger.Warn("Could not initialize file logging", "error", logErr)
	}

	if config.ProxyURL != "" {
		_ = os.Setenv("HTTP_PROXY", config.ProxyURL)
		_ = os.Setenv("HTTPS_PROXY", config.ProxyURL)
	}

	return config, ctx, cancel, slogger, csvLogger, nil
}

func newSendMailCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sendmail",
		Short: "Send an email via the Gmail API",
		RunE: func(cmd *cobra.Command, args []string) error {
			config, ctx, cancel, slogger, csvLogger, err := setup(cmd, v, ActionSendMail)
			if err != nil {
				return err
			}
			defer cancel()
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			if config.BodyTemplate != "" {
				content, err := os.ReadFile(config.BodyTemplate)
				if err != nil {
					return fmt.Errorf("failed to read body template file: %w", err)
				}
				config.BodyHTML = string(content)
			}

			// Default To to the mailbox if no recipients were specified.
			if len(config.To) == 0 && len(config.Cc) == 0 && len(config.Bcc) == 0 {
				config.To = stringSlice{config.Mailbox}
			}

			svc, err := newGmailService(ctx, config, slogger)
			if err != nil {
				return err
			}
			return SendEmail(ctx, svc, config, csvLogger)
		},
	}
	cmd.Flags().String("to", "", "Comma-separated TO recipients (env: GMAILTO)")
	cmd.Flags().String("cc", "", "Comma-separated CC recipients (env: GMAILCC)")
	cmd.Flags().String("bcc", "", "Comma-separated BCC recipients (env: GMAILBCC)")
	cmd.Flags().String("subject", "Automated Tool Notification", "Email subject (env: GMAILSUBJECT)")
	cmd.Flags().String("body", "It's a test message, please ignore", "Email body text (env: GMAILBODY)")
	cmd.Flags().String("bodyhtml", "", "HTML body content (env: GMAILBODYHTML)")
	cmd.Flags().String("body-template", "", "Path to HTML email body template file (env: GMAILBODYTEMPLATE)")
	cmd.Flags().String("attachments", "", "Comma-separated file paths to attach (env: GMAILATTACHMENTS)")
	cmd.Flags().String("inline-attachments", "", "Comma-separated file paths to embed inline via cid:<filename> (env: GMAILINLINEATTACHMENTS)")
	cmd.Flags().StringArray("header", nil, "Custom header in 'Name: Value' form (repeatable) (env: GMAILHEADER — comma-separated)")
	cmd.Flags().String("priority", "normal", "Email priority: high, normal, low (env: GMAILPRIORITY)")
	return cmd
}

func newGetInboxCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "getinbox",
		Short: "List recent inbox messages for a mailbox",
		RunE: func(cmd *cobra.Command, args []string) error {
			config, ctx, cancel, slogger, csvLogger, err := setup(cmd, v, ActionGetInbox)
			if err != nil {
				return err
			}
			defer cancel()
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			svc, err := newGmailService(ctx, config, slogger)
			if err != nil {
				return err
			}
			return listInbox(ctx, svc, config, csvLogger)
		},
	}
	cmd.Flags().Int("count", 3, "Number of messages to retrieve (env: GMAILCOUNT)")
	return cmd
}

func newExportMessagesCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exportmessages",
		Short: "Search messages by Message-ID and/or Subject and export them as .eml files",
		RunE: func(cmd *cobra.Command, args []string) error {
			config, ctx, cancel, slogger, csvLogger, err := setup(cmd, v, ActionExportMessages)
			if err != nil {
				return err
			}
			defer cancel()
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			svc, err := newGmailService(ctx, config, slogger)
			if err != nil {
				return err
			}
			return exportMessages(ctx, svc, config, csvLogger)
		},
	}
	cmd.Flags().String("messageid", "", "Internet Message-ID to search for (env: GMAILMESSAGEID)")
	cmd.Flags().String("subject", "", "Subject substring to search for (env: GMAILSUBJECT)")
	cmd.Flags().Int("count", 25, "Maximum number of matching messages to export (env: GMAILCOUNT)")
	cmd.Flags().String("exportdir", "", "Directory under which to create the dated export folder; defaults to the OS temp directory (env: GMAILEXPORTDIR)")
	return cmd
}

func newGetEventsCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "getevents",
		Short: "List upcoming calendar events for a mailbox",
		RunE: func(cmd *cobra.Command, args []string) error {
			config, ctx, cancel, slogger, csvLogger, err := setup(cmd, v, ActionGetEvents)
			if err != nil {
				return err
			}
			defer cancel()
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			svc, err := newCalendarService(ctx, config, slogger)
			if err != nil {
				return err
			}
			return listEvents(ctx, svc, config, csvLogger)
		},
	}
	cmd.Flags().Int("count", 3, "Number of events to retrieve (env: GMAILCOUNT)")
	return cmd
}

func newSendInviteCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sendinvite",
		Short: "Create a calendar event and invite attendees",
		RunE: func(cmd *cobra.Command, args []string) error {
			config, ctx, cancel, slogger, csvLogger, err := setup(cmd, v, ActionSendInvite)
			if err != nil {
				return err
			}
			defer cancel()
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			svc, err := newCalendarService(ctx, config, slogger)
			if err != nil {
				return err
			}
			return createInvite(ctx, svc, config, csvLogger)
		},
	}
	cmd.Flags().String("subject", "Automated Tool Notification", "Calendar event subject (env: GMAILSUBJECT)")
	cmd.Flags().String("start", "", "Start time (RFC3339 or PowerShell sortable format) (env: GMAILSTART)")
	cmd.Flags().String("end", "", "End time (RFC3339 or PowerShell sortable format) (env: GMAILEND)")
	cmd.Flags().String("to", "", "Comma-separated attendee email addresses (env: GMAILTO)")
	return cmd
}

func newTestAuthCmd(v *viper.Viper) *cobra.Command {
	return &cobra.Command{
		Use:   "testauth",
		Short: "Verify authentication and impersonation via GetProfile",
		RunE: func(cmd *cobra.Command, args []string) error {
			config, ctx, cancel, slogger, csvLogger, err := setup(cmd, v, ActionTestAuth)
			if err != nil {
				return err
			}
			defer cancel()
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			svc, err := newGmailService(ctx, config, slogger)
			if err != nil {
				return err
			}
			return testAuth(ctx, svc, config, csvLogger)
		},
	}
}

func newExportBearerTokenCmd(v *viper.Viper) *cobra.Command {
	return &cobra.Command{
		Use:   "exportbearertoken",
		Short: "Acquire and print an access token from the configured credential",
		RunE: func(cmd *cobra.Command, args []string) error {
			config, ctx, cancel, slogger, csvLogger, err := setup(cmd, v, ActionExportBearerToken)
			if err != nil {
				return err
			}
			defer cancel()
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			return exportBearerToken(ctx, config, slogger, csvLogger)
		},
	}
}
