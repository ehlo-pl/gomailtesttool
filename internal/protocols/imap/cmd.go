package imap

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/ziembor/gomailtesttool/internal/common/bootstrap"
	"github.com/ziembor/gomailtesttool/internal/common/logger"
)

// NewCmd returns the "imap" cobra.Command with all 3 action subcommands.
// Each subcommand shares persistent flags (server, auth, TLS, output).
func NewCmd() *cobra.Command {
	v := viper.New()

	cmd := &cobra.Command{
		Use:   "imap",
		Short: "IMAP server connectivity and authentication testing",
		Long: `Test IMAP server connectivity, TLS configuration, authentication, and folder listing.

Supports STARTTLS and IMAPS (implicit TLS) modes, PLAIN, LOGIN, and XOAUTH2 authentication,
and connect-address override for load balancer testing.

Environment variables use the IMAP prefix (e.g. IMAPHOST, IMAPPORT, IMAPUSERNAME).`,
	}

	RegisterPersistentFlags(cmd)
	BindEnvs(v)

	cmd.AddCommand(
		newTestConnectCmd(v),
		newTestAuthCmd(v),
		newListFoldersCmd(v),
		newExportMessagesCmd(v),
	)

	return cmd
}

func newTestConnectCmd(v *viper.Viper) *cobra.Command {
	return &cobra.Command{
		Use:   "testconnect",
		Short: "Test TCP connection and server capabilities",
		Long: `Connect to the IMAP server and verify the connection, CAPABILITY response,
and (for IMAPS/STARTTLS) TLS state.`,
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

			slogger, csvLogger, logErr := bootstrap.InitLoggers("imaptool", ActionTestConnect, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer csvLogger.Close()
			}

			logger.LogInfo(slogger, "IMAP Connectivity Testing Tool started", "action", config.Action, "host", config.Host, "port", config.Port)

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
		Short: "Test IMAP authentication",
		Long: `Authenticate to the IMAP server using the configured credentials and auth method.
Supports PLAIN, LOGIN, and XOAUTH2 (OAuth2 bearer token).
Use --starttls or --imaps to establish TLS before authenticating.`,
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

			slogger, csvLogger, logErr := bootstrap.InitLoggers("imaptool", ActionTestAuth, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer csvLogger.Close()
			}

			logger.LogInfo(slogger, "IMAP Connectivity Testing Tool started", "action", config.Action, "host", config.Host, "port", config.Port)

			if err := testAuth(ctx, config, csvLogger, slogger); err != nil {
				logger.LogError(slogger, "Action failed", "error", err)
				return err
			}

			logger.LogInfo(slogger, "Action completed successfully")
			return nil
		},
	}
}

func newExportMessagesCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exportmessages",
		Short: "Search messages by Message-ID and/or Subject and export them as .eml files",
		Long: `Authenticate to the IMAP server, SEARCH the selected mailbox for messages
matching the given Message-ID and/or Subject, and export each match's full
raw RFC822 message as a .eml file.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}

			config := ConfigFromViper(v)
			config.Action = ActionExportMessages

			if err := validateConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w\n\nRun '%s --help' for usage", err, cmd.CommandPath())
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger, csvLogger, logErr := bootstrap.InitLoggers("imaptool", ActionExportMessages, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer csvLogger.Close()
			}

			logger.LogInfo(slogger, "IMAP Connectivity Testing Tool started", "action", config.Action, "host", config.Host, "port", config.Port)

			if err := exportMessages(ctx, config, csvLogger, slogger); err != nil {
				logger.LogError(slogger, "Action failed", "error", err)
				return err
			}

			logger.LogInfo(slogger, "Action completed successfully")
			return nil
		},
	}
	cmd.Flags().String("messageid", "", "Message-ID header value to search for (env: IMAPMESSAGEID)")
	cmd.Flags().String("subject", "", "Subject substring to search for (env: IMAPSUBJECT)")
	cmd.Flags().String("mailbox", "INBOX", "Mailbox to search (env: IMAPMAILBOX)")
	cmd.Flags().Int("count", 25, "Maximum number of matching messages to export (env: IMAPCOUNT)")
	cmd.Flags().String("exportdir", "", "Directory under which to create the dated export folder; defaults to the OS temp directory (env: IMAPEXPORTDIR)")
	return cmd
}

func newListFoldersCmd(v *viper.Viper) *cobra.Command {
	return &cobra.Command{
		Use:   "listfolders",
		Short: "List mailbox folders",
		Long: `Authenticate to the IMAP server and list all mailbox folders using the LIST command.
Shows folder name, attributes, message count, and unseen count for each folder.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}

			config := ConfigFromViper(v)
			config.Action = ActionListFolders

			if err := validateConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w\n\nRun '%s --help' for usage", err, cmd.CommandPath())
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger, csvLogger, logErr := bootstrap.InitLoggers("imaptool", ActionListFolders, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer csvLogger.Close()
			}

			logger.LogInfo(slogger, "IMAP Connectivity Testing Tool started", "action", config.Action, "host", config.Host, "port", config.Port)

			if err := listFolders(ctx, config, csvLogger, slogger); err != nil {
				logger.LogError(slogger, "Action failed", "error", err)
				return err
			}

			logger.LogInfo(slogger, "Action completed successfully")
			return nil
		},
	}
}
