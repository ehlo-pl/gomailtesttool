package pop3

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/ziembor/gomailtesttool/internal/common/bootstrap"
	"github.com/ziembor/gomailtesttool/internal/common/logger"
)

// NewCmd returns the "pop3" cobra.Command with all 3 action subcommands.
// Each subcommand shares persistent flags (server, auth, TLS, output) and adds
// its own action-specific flags.
func NewCmd() *cobra.Command {
	v := viper.New()

	cmd := &cobra.Command{
		Use:   "pop3",
		Short: "POP3 server connectivity and authentication testing",
		Long: `Test POP3 server connectivity, TLS configuration, authentication, and mailbox listing.

Supports STLS and POP3S (implicit TLS) modes, USER/PASS, APOP, and XOAUTH2 authentication,
and connect-address override for load balancer testing.

Environment variables use the POP3 prefix (e.g. POP3HOST, POP3PORT, POP3USERNAME).`,
	}

	RegisterPersistentFlags(cmd)
	BindEnvs(v)

	cmd.AddCommand(
		newTestConnectCmd(v),
		newTestAuthCmd(v),
		newListMailCmd(v),
	)

	return cmd
}

func newTestConnectCmd(v *viper.Viper) *cobra.Command {
	return &cobra.Command{
		Use:   "testconnect",
		Short: "Test TCP connection and server capabilities",
		Long: `Connect to the POP3 server and verify the connection greeting, CAPA capabilities,
and (for POP3S) TLS state.`,
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

			slogger, csvLogger, logErr := bootstrap.InitLoggers("pop3tool", ActionTestConnect, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer csvLogger.Close()
			}

			logger.LogInfo(slogger, "POP3 Connectivity Testing Tool started", "action", config.Action, "host", config.Host, "port", config.Port)

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
		Short: "Test POP3 authentication",
		Long: `Authenticate to the POP3 server using the configured credentials and auth method.
Supports USER/PASS, APOP, and XOAUTH2 (OAuth2 bearer token).
Automatically upgrades to TLS via STLS when --starttls is set.`,
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

			slogger, csvLogger, logErr := bootstrap.InitLoggers("pop3tool", ActionTestAuth, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer csvLogger.Close()
			}

			logger.LogInfo(slogger, "POP3 Connectivity Testing Tool started", "action", config.Action, "host", config.Host, "port", config.Port)

			if err := testAuth(ctx, config, csvLogger, slogger); err != nil {
				logger.LogError(slogger, "Action failed", "error", err)
				return err
			}

			logger.LogInfo(slogger, "Action completed successfully")
			return nil
		},
	}
}

func newListMailCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "listmail",
		Short: "List messages in the mailbox",
		Long: `Authenticate to the POP3 server and list messages using STAT, LIST, and UIDL commands.
Shows message count, total size, and per-message size and UIDL (if supported).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}

			config := ConfigFromViper(v)
			config.Action = ActionListMail

			if err := validateConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w\n\nRun '%s --help' for usage", err, cmd.CommandPath())
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger, csvLogger, logErr := bootstrap.InitLoggers("pop3tool", ActionListMail, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer csvLogger.Close()
			}

			logger.LogInfo(slogger, "POP3 Connectivity Testing Tool started", "action", config.Action, "host", config.Host, "port", config.Port)

			if err := listMail(ctx, config, csvLogger, slogger); err != nil {
				logger.LogError(slogger, "Action failed", "error", err)
				return err
			}

			logger.LogInfo(slogger, "Action completed successfully")
			return nil
		},
	}

	cmd.Flags().Int("maxmessages", 100, "Maximum messages to list (env: POP3MAXMESSAGES)")

	return cmd
}
