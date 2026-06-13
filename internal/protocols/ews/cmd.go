package ews

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/ziembor/gomailtesttool/internal/common/bootstrap"
	"github.com/ziembor/gomailtesttool/internal/common/logger"
)

// NewCmd returns the "ews" cobra.Command with all 4 action subcommands.
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
	)

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
				defer csvLogger.Close()
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
				defer csvLogger.Close()
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
				defer csvLogger.Close()
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
				defer csvLogger.Close()
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
