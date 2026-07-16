package jmap

import (
	"fmt"

	"github.com/ehlo-pl/gomailtesttool/internal/common/bootstrap"
	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// NewCmd returns the "jmap" cobra.Command with all action subcommands.
// Each subcommand shares persistent flags (server, auth, TLS, output).
func NewCmd() *cobra.Command {
	v := viper.New()

	cmd := &cobra.Command{
		Use:   "jmap",
		Short: "JMAP server connectivity and authentication testing",
		Long: `Test JMAP server connectivity, authentication, and mailbox listing.

Uses HTTPS with Bearer or Basic authentication. Supports connect-address override
for load balancer testing and JMAP session discovery per RFC 8620.

Environment variables use the JMAP prefix (e.g. JMAPHOST, JMAPPORT, JMAPUSERNAME).`,
	}

	RegisterPersistentFlags(cmd)
	BindEnvs(v)

	cmd.AddCommand(
		newTestConnectCmd(v),
		newTestAuthCmd(v),
		newListFoldersCmd(v),
		newListMailCmd(v),
		newSendMailCmd(v),
		newExportMessagesCmd(v),
	)

	return cmd
}

func newTestConnectCmd(v *viper.Viper) *cobra.Command {
	return &cobra.Command{
		Use:   "testconnect",
		Short: "Test JMAP server connectivity and discover session",
		Long: `Connect to the JMAP server and verify connectivity by fetching and parsing
the JMAP session object from the well-known discovery URL (/.well-known/jmap).`,
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

			slogger, csvLogger, logErr := bootstrap.InitLoggers("jmaptool", ActionTestConnect, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			logger.LogInfo(slogger, "JMAP Testing Tool started", "action", config.Action, "host", config.Host, "port", config.Port)

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
		Short: "Test JMAP authentication",
		Long: `Authenticate to the JMAP server using the configured credentials and auth method.
Supports Bearer token (--accesstoken) and Basic auth (--username + --password).`,
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

			slogger, csvLogger, logErr := bootstrap.InitLoggers("jmaptool", ActionTestAuth, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			logger.LogInfo(slogger, "JMAP Testing Tool started", "action", config.Action, "host", config.Host, "port", config.Port)

			if err := testAuth(ctx, config, csvLogger, slogger); err != nil {
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
		Use:     "listfolders",
		Aliases: []string{"getmailboxes"},
		Short:   "List JMAP mailboxes (folders) with message counts",
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

			slogger, csvLogger, logErr := bootstrap.InitLoggers("jmaptool", ActionListFolders, config.VerboseMode, config.LogLevel, config.LogFormat)
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
		Short: "List recent inbox messages",
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

			slogger, csvLogger, logErr := bootstrap.InitLoggers("jmaptool", ActionListMail, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			return listMail(ctx, config, csvLogger, slogger)
		},
	}
	cmd.Flags().Int("count", 3, "Number of messages to retrieve (env: JMAPCOUNT)")
	return cmd
}

func newSendMailCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sendmail",
		Short: "Send an email via JMAP",
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

			slogger, csvLogger, logErr := bootstrap.InitLoggers("jmaptool", ActionSendMail, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			return sendMail(ctx, config, csvLogger, slogger)
		},
	}
	cmd.Flags().String("to", "", "Comma-separated TO recipients (env: JMAPTO)")
	cmd.Flags().String("cc", "", "Comma-separated CC recipients (env: JMAPCC)")
	cmd.Flags().String("bcc", "", "Comma-separated BCC recipients (env: JMAPBCC)")
	cmd.Flags().String("subject", "Automated Tool Notification", "Email subject (env: JMAPSUBJECT)")
	cmd.Flags().String("body", "It's a test message, please ignore", "Email body text (env: JMAPBODY)")
	cmd.Flags().String("bodyhtml", "", "HTML body content (env: JMAPBODYHTML)")
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
			// Only use subject if explicitly provided (avoid default value narrowing search).
			config.Subject = v.GetString("subject")

			if err := validateConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger, csvLogger, logErr := bootstrap.InitLoggers("jmaptool", ActionExportMessages, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			return exportMessages(ctx, config, csvLogger, slogger)
		},
	}
	cmd.Flags().String("messageid", "", "Internet Message-ID to search for (env: JMAPMESSAGEID)")
	cmd.Flags().String("subject", "", "Subject substring to search for (env: JMAPSUBJECT)")
	cmd.Flags().Int("count", 25, "Maximum number of messages to export (env: JMAPCOUNT)")
	cmd.Flags().String("exportdir", "", "Directory for export folder (defaults to OS temp dir) (env: JMAPEXPORTDIR)")
	return cmd
}
