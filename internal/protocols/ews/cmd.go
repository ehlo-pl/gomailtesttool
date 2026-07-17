package ews

import (
	"fmt"

	"github.com/ehlo-pl/gomailtesttool/internal/common/bootstrap"
	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// NewCmd returns the "ews" cobra.Command with all action subcommands.
func NewCmd() *cobra.Command {
	v := viper.New()

	cmd := &cobra.Command{
		Use:   "ews",
		Short: "EWS (Exchange Web Services) connectivity and authentication testing",
		Long: `Test Exchange Web Services (EWS) endpoints on on-premises Exchange Server (2007–2019).

Supports NTLM, Basic, and Bearer (OAuth2) authentication. Useful for diagnosing
on-premises Exchange connectivity where Microsoft Graph is not available.

Environment variables use the EWS prefix (e.g. EWSHOST, EWSUSERNAME, EWSPASSWORD).`,
	}

	RegisterPersistentFlags(cmd)
	BindEnvs(v)

	cmd.AddCommand(
		newTestConnectCmd(v),
		newTestAuthCmd(v),
		newGetFolderCmd(v),
		newAutodiscoverCmd(v),
		newListFoldersCmd(v),
		newListMailCmd(v),
		newSendMailCmd(v),
		newExportMessagesCmd(v),
		newGetEventsCmd(v),
		newSendInviteCmd(v),
		newGetScheduleCmd(v),
		newFindTimeSlotCmd(v),
	)

	return cmd
}

func newGetEventsCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "getevents",
		Short: "List calendar events in a time window (default: next 7 days)",
		Long: `Authenticate to the EWS server and list calendar events using FindItem with a
CalendarView (recurring meetings are expanded). Defaults to the next 7 days.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}

			config := ConfigFromViper(v)
			config.Action = ActionGetEvents

			if err := validateConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w\n\nRun '%s --help' for usage", err, cmd.CommandPath())
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger, csvLogger, logErr := bootstrap.InitLoggers("ewstool", ActionGetEvents, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			logger.LogInfo(slogger, "EWS Testing Tool started", "action", config.Action, "host", config.Host, "port", config.Port)

			if err := getEvents(ctx, config, csvLogger, slogger); err != nil {
				logger.LogError(slogger, "Action failed", "error", err)
				return err
			}

			logger.LogInfo(slogger, "Action completed successfully")
			return nil
		},
	}
	cmd.Flags().String("start", "", "Window start time (RFC3339 or PowerShell sortable format, default: now) (env: EWSSTART)")
	cmd.Flags().String("end", "", "Window end time (RFC3339 or PowerShell sortable format, default: start+7d) (env: EWSEND)")
	cmd.Flags().Int("count", 10, "Maximum number of events to return (env: EWSCOUNT)")
	return cmd
}

func newSendInviteCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sendinvite",
		Short: "Create a calendar meeting and send invitations to attendees",
		Long: `Authenticate to the EWS server and create a calendar meeting via CreateItem with
SendMeetingInvitations="SendToAllAndSaveCopy", inviting the --to attendees.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}

			config := ConfigFromViper(v)
			config.Action = ActionSendInvite

			if err := validateConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w\n\nRun '%s --help' for usage", err, cmd.CommandPath())
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger, csvLogger, logErr := bootstrap.InitLoggers("ewstool", ActionSendInvite, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			logger.LogInfo(slogger, "EWS Testing Tool started", "action", config.Action, "host", config.Host, "port", config.Port)

			if err := sendInvite(ctx, config, csvLogger, slogger); err != nil {
				logger.LogError(slogger, "Action failed", "error", err)
				return err
			}

			logger.LogInfo(slogger, "Action completed successfully")
			return nil
		},
	}
	cmd.Flags().String("to", "", "Comma-separated attendee email addresses (env: EWSTO)")
	cmd.Flags().String("subject", "Automated Tool Notification", "Meeting subject (env: EWSSUBJECT)")
	cmd.Flags().String("body", "It's a test meeting, please ignore", "Meeting body text (env: EWSBODY)")
	cmd.Flags().String("start", "", "Meeting start time (RFC3339 or PowerShell sortable format) (env: EWSSTART)")
	cmd.Flags().String("end", "", "Meeting end time (RFC3339 or PowerShell sortable format) (env: EWSEND)")
	return cmd
}

func newGetScheduleCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "getschedule",
		Short: "Check a recipient's free/busy availability",
		Long: `Authenticate to the EWS server and retrieve the recipient's merged free/busy view
via GetUserAvailability. Defaults to the next 24 hours in hourly slots.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}

			config := ConfigFromViper(v)
			config.Action = ActionGetSchedule

			if err := validateConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w\n\nRun '%s --help' for usage", err, cmd.CommandPath())
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger, csvLogger, logErr := bootstrap.InitLoggers("ewstool", ActionGetSchedule, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			logger.LogInfo(slogger, "EWS Testing Tool started", "action", config.Action, "host", config.Host, "port", config.Port)

			if err := getSchedule(ctx, config, csvLogger, slogger); err != nil {
				logger.LogError(slogger, "Action failed", "error", err)
				return err
			}

			logger.LogInfo(slogger, "Action completed successfully")
			return nil
		},
	}
	cmd.Flags().String("to", "", "Recipient email address to check availability for (env: EWSTO)")
	cmd.Flags().String("start", "", "Window start time (RFC3339 or PowerShell sortable format, default: now) (env: EWSSTART)")
	cmd.Flags().String("end", "", "Window end time (RFC3339 or PowerShell sortable format, default: start+24h) (env: EWSEND)")
	return cmd
}

func newFindTimeSlotCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "findtimeslot",
		Short: "Find free meeting slots in another user's calendar",
		Long: `Authenticate to the EWS server and search a recipient's calendar for free
meeting slots of the given duration. Busy data comes from GetUserAvailability
(detailed FreeBusy view) and free slots are computed client-side, constrained
to working hours (08:00-17:00 UTC, Monday-Friday).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}

			config := ConfigFromViper(v)
			config.Action = ActionFindTimeSlot

			if err := validateConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w\n\nRun '%s --help' for usage", err, cmd.CommandPath())
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger, csvLogger, logErr := bootstrap.InitLoggers("ewstool", ActionFindTimeSlot, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			logger.LogInfo(slogger, "EWS Testing Tool started", "action", config.Action, "host", config.Host, "port", config.Port)

			if err := findTimeSlot(ctx, config, csvLogger, slogger); err != nil {
				logger.LogError(slogger, "Action failed", "error", err)
				return err
			}

			logger.LogInfo(slogger, "Action completed successfully")
			return nil
		},
	}
	cmd.Flags().String("to", "", "Recipient email address whose calendar to search (env: EWSTO)")
	cmd.Flags().Int("duration", 30, "Meeting duration in minutes (env: EWSDURATION)")
	cmd.Flags().String("start", "", "Window start (RFC3339 or PowerShell sortable format); default: now (env: EWSSTART)")
	cmd.Flags().String("end", "", "Window end; default: start + 5 working days (env: EWSEND)")
	cmd.Flags().Int("count", 3, "Maximum number of slots to return (env: EWSCOUNT)")
	return cmd
}

func newTestConnectCmd(v *viper.Viper) *cobra.Command {
	return &cobra.Command{
		Use:   "testconnect",
		Short: "Test HTTP/TLS connectivity to the EWS endpoint",
		Long: `Connect to the EWS endpoint and verify the server responds.
No credentials required — HTTP 401 Unauthorized confirms the server is alive.
Reports TLS version, cipher suite, and certificate details.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}

			config := ConfigFromViper(v)
			config.Action = ActionTestConnect

			if err := validateConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w\n\nRun '%s --help' for usage", err, cmd.CommandPath())
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger, csvLogger, logErr := bootstrap.InitLoggers("ewstool", ActionTestConnect, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			logger.LogInfo(slogger, "EWS Testing Tool started", "action", config.Action, "host", config.Host, "port", config.Port)

			if err := testConnect(ctx, config, csvLogger, slogger); err != nil {
				logger.LogError(slogger, "Action failed", "error", err)
				return err
			}

			logger.LogInfo(slogger, "Action completed successfully")
			return nil
		},
	}
}

func newTestAuthCmd(v *viper.Viper) *cobra.Command {
	return &cobra.Command{
		Use:   "testauth",
		Short: "Test EWS authentication",
		Long: `Authenticate to the EWS server and verify credentials by performing GetFolder(Inbox).
Supports NTLM (on-premises AD), Basic, and Bearer (OAuth2) authentication.
Auth method is auto-detected: NTLM if username contains backslash, Bearer if access token provided.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}

			config := ConfigFromViper(v)
			config.Action = ActionTestAuth

			if err := validateConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w\n\nRun '%s --help' for usage", err, cmd.CommandPath())
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger, csvLogger, logErr := bootstrap.InitLoggers("ewstool", ActionTestAuth, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			logger.LogInfo(slogger, "EWS Testing Tool started", "action", config.Action, "host", config.Host)

			if err := testAuth(ctx, config, csvLogger, slogger); err != nil {
				logger.LogError(slogger, "Action failed", "error", err)
				return err
			}

			logger.LogInfo(slogger, "Action completed successfully")
			return nil
		},
	}
}

func newGetFolderCmd(v *viper.Viper) *cobra.Command {
	return &cobra.Command{
		Use:   "getfolder",
		Short: "Get Inbox folder properties",
		Long: `Authenticate to EWS and retrieve Inbox folder properties: display name,
total item count, unread count, and folder ID.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}

			config := ConfigFromViper(v)
			config.Action = ActionGetFolder

			if err := validateConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w\n\nRun '%s --help' for usage", err, cmd.CommandPath())
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger, csvLogger, logErr := bootstrap.InitLoggers("ewstool", ActionGetFolder, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			logger.LogInfo(slogger, "EWS Testing Tool started", "action", config.Action, "host", config.Host)

			if err := getFolder(ctx, config, csvLogger, slogger); err != nil {
				logger.LogError(slogger, "Action failed", "error", err)
				return err
			}

			logger.LogInfo(slogger, "Action completed successfully")
			return nil
		},
	}
}

func newListFoldersCmd(v *viper.Viper) *cobra.Command {
	return &cobra.Command{
		Use:   "listfolders",
		Short: "List EWS mail folders",
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
			slogger, csvLogger, logErr := bootstrap.InitLoggers("ewstool", ActionListFolders, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}
			return listFolders(ctx, config, csvLogger, slogger)
		},
	}
}

func newListMailCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "listmail",
		Short: "List recent inbox messages via EWS",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())
			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}
			config := ConfigFromViper(v)
			config.Action = ActionListMail
			if err := validateConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}
			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()
			slogger, csvLogger, logErr := bootstrap.InitLoggers("ewstool", ActionListMail, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}
			return listMail(ctx, config, csvLogger, slogger)
		},
	}
	cmd.Flags().Int("count", 10, "Number of messages to retrieve (env: EWSCOUNT)")
	return cmd
}

func newSendMailCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sendmail",
		Short: "Send an email via EWS",
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
			slogger, csvLogger, logErr := bootstrap.InitLoggers("ewstool", ActionSendMail, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}
			if err := resolveTemplate(config, slogger); err != nil {
				return fmt.Errorf("template failed: %w", err)
			}
			return sendMail(ctx, config, csvLogger, slogger)
		},
	}
	cmd.Flags().String("to", "", "Comma-separated TO recipients (env: EWSTO)")
	cmd.Flags().String("cc", "", "Comma-separated CC recipients (env: EWSCC)")
	cmd.Flags().String("subject", "Automated Tool Notification", "Email subject (env: EWSSUBJECT)")
	cmd.Flags().String("body", "It's a test message, please ignore", "Email body text (env: EWSBODY)")
	cmd.Flags().String("bodyhtml", "", "HTML body content (overrides --body if set) (env: EWSBODYHTML)")
	cmd.Flags().String("template", "", "Message template file with Go text/template variables: a .eml file has its recognised fields (To/Cc/Subject/bodies) mapped to EWS CreateItem; any other extension is used as the HTML body (env: EWSTEMPLATE)")
	cmd.Flags().StringArray("template-vars", nil, "Template variable in 'key=value' form, referenced as {{.key}} in --template (repeatable) (env: EWSTEMPLATEVARS)")
	return cmd
}

func newExportMessagesCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exportmessages",
		Short: "Search messages by Message-ID and/or Subject and export as .eml files",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())
			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}
			config := ConfigFromViper(v)
			config.Action = ActionExportMessages
			// Only use subject if explicitly provided.
			config.Subject = v.GetString("subject")
			if err := validateConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}
			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()
			slogger, csvLogger, logErr := bootstrap.InitLoggers("ewstool", ActionExportMessages, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}
			return exportMessages(ctx, config, csvLogger, slogger)
		},
	}
	cmd.Flags().String("messageid", "", "Internet Message-ID to search for (env: EWSMESSAGEID)")
	cmd.Flags().String("subject", "", "Subject substring to search for (env: EWSSUBJECT)")
	cmd.Flags().Int("count", 25, "Maximum number of messages to export (env: EWSCOUNT)")
	cmd.Flags().String("exportdir", "", "Directory for export folder (defaults to OS temp dir) (env: EWSEXPORTDIR)")
	return cmd
}

func newAutodiscoverCmd(v *viper.Viper) *cobra.Command {
	return &cobra.Command{
		Use:   "autodiscover",
		Short: "Test the EWS Autodiscover SOAP endpoint",
		Long: `POST a GetUserSettings request to the Autodiscover endpoint and report
the resolved EWS URLs (internal and external), user display name, and AD server.
Useful for diagnosing Exchange client configuration issues.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}

			config := ConfigFromViper(v)
			config.Action = ActionAutodiscover

			if err := validateConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w\n\nRun '%s --help' for usage", err, cmd.CommandPath())
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger, csvLogger, logErr := bootstrap.InitLoggers("ewstool", ActionAutodiscover, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			logger.LogInfo(slogger, "EWS Testing Tool started", "action", config.Action, "host", config.Host)

			if err := autodiscover(ctx, config, csvLogger, slogger); err != nil {
				logger.LogError(slogger, "Action failed", "error", err)
				return err
			}

			logger.LogInfo(slogger, "Action completed successfully")
			return nil
		},
	}
}
