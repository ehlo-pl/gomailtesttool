package smtp

import (
	"fmt"

	"github.com/ehlo-pl/gomailtesttool/internal/common/bootstrap"
	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// NewCmd returns the "smtp" cobra.Command with all 4 action subcommands.
// Each subcommand shares persistent flags (server, auth, TLS, output) and adds
// its own action-specific flags.
func NewCmd() *cobra.Command {
	v := viper.New()

	cmd := &cobra.Command{
		Use:   "smtp",
		Short: "SMTP server connectivity and authentication testing",
		Long: `Test SMTP server connectivity, TLS configuration, authentication, and email sending.

Supports STARTTLS and SMTPS (implicit TLS) modes, all standard AUTH mechanisms
(PLAIN, LOGIN, CRAM-MD5, XOAUTH2), and connect-address override for load balancer testing.

Environment variables use the SMTP prefix (e.g. SMTPHOST, SMTPPORT, SMTPUSERNAME).`,
	}

	RegisterPersistentFlags(cmd)
	BindEnvs(v)

	cmd.AddCommand(
		newTestConnectCmd(v),
		newTestStartTLSCmd(v),
		newTestAuthCmd(v),
		newSendMailCmd(v),
	)

	return cmd
}

func newTestConnectCmd(v *viper.Viper) *cobra.Command {
	return &cobra.Command{
		Use:   "testconnect",
		Short: "Test TCP connection and server capabilities",
		Long: `Connect to the SMTP server and verify the connection banner, EHLO capabilities,
and (for SMTPS) TLS state. Detects Exchange Online servers.`,
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

			slogger, csvLogger, logErr := bootstrap.InitLoggers("smtptool", ActionTestConnect, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			logger.LogInfo(slogger, "SMTP Connectivity Testing Tool started", "action", config.Action, "host", config.Host, "port", config.Port)

			if err := testConnect(ctx, config, csvLogger, slogger); err != nil {
				logger.LogError(slogger, "Action failed", "error", err)
				return err
			}

			logger.LogInfo(slogger, "Action completed successfully")
			return nil
		},
	}
}

func newTestStartTLSCmd(v *viper.Viper) *cobra.Command {
	return &cobra.Command{
		Use:   "teststarttls",
		Short: "Test TLS/SSL with comprehensive diagnostics",
		Long: `Perform STARTTLS or SMTPS handshake and analyze the TLS connection in detail:
protocol version, cipher suite, certificate chain, SANs, expiry, and security warnings.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}

			config := ConfigFromViper(v)
			config.Action = ActionTestStartTLS

			if err := validateConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w\n\nRun '%s --help' for usage", err, cmd.CommandPath())
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger, csvLogger, logErr := bootstrap.InitLoggers("smtptool", ActionTestStartTLS, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			logger.LogInfo(slogger, "SMTP Connectivity Testing Tool started", "action", config.Action, "host", config.Host, "port", config.Port)

			if err := testStartTLS(ctx, config, csvLogger, slogger); err != nil {
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
		Short: "Test SMTP authentication",
		Long: `Authenticate to the SMTP server using the configured credentials and auth method.
Supports PLAIN, LOGIN, CRAM-MD5, and XOAUTH2 (OAuth2 bearer token).
Automatically upgrades to TLS via STARTTLS when available on ports 25/587.`,
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

			slogger, csvLogger, logErr := bootstrap.InitLoggers("smtptool", ActionTestAuth, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			logger.LogInfo(slogger, "SMTP Connectivity Testing Tool started", "action", config.Action, "host", config.Host, "port", config.Port)

			if err := testAuth(ctx, config, csvLogger, slogger); err != nil {
				logger.LogError(slogger, "Action failed", "error", err)
				return err
			}

			logger.LogInfo(slogger, "Action completed successfully")
			return nil
		},
	}
}

func newSendMailCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sendmail",
		Short: "Send a test email",
		Long: `Send a test email through the SMTP server. Authenticates if credentials are provided,
upgrades to TLS automatically, and logs the result (including TLS details) to CSV.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			if err := bootstrap.LoadConfigFile(v, v.GetString("config")); err != nil {
				return err
			}

			config := ConfigFromViper(v)
			config.Action = ActionSendMail

			if err := validateConfiguration(config); err != nil {
				return fmt.Errorf("validation failed: %w\n\nRun '%s --help' for usage", err, cmd.CommandPath())
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger, csvLogger, logErr := bootstrap.InitLoggers("smtptool", ActionSendMail, config.VerboseMode, config.LogLevel, config.LogFormat)
			if logErr != nil {
				slogger.Warn("Could not initialize file logging", "error", logErr)
			}
			if csvLogger != nil {
				defer func() { _ = csvLogger.Close() }()
			}

			logger.LogInfo(slogger, "SMTP Connectivity Testing Tool started", "action", config.Action, "host", config.Host, "port", config.Port)

			if err := SendMail(ctx, config, csvLogger, slogger); err != nil {
				logger.LogError(slogger, "Action failed", "error", err)
				return err
			}

			logger.LogInfo(slogger, "Action completed successfully")
			return nil
		},
	}

	cmd.Flags().String("from", "", "Sender email address (env: SMTPFROM)")
	cmd.Flags().String("to", "", "Comma-separated recipient email addresses (env: SMTPTO)")
	cmd.Flags().String("cc", "", "Comma-separated CC recipient email addresses; included in the Cc: header and the SMTP envelope (env: SMTPCC)")
	cmd.Flags().String("bcc", "", "Comma-separated BCC recipient email addresses; included in the SMTP envelope only, never in message headers (env: SMTPBCC)")
	cmd.Flags().String("subject", "SMTP Test", "Email subject (env: SMTPSUBJECT)")
	cmd.Flags().String("body", "This is a test message from smtptool", "Email body text (env: SMTPBODY)")
	cmd.Flags().String("bodyhtml", "", "HTML body content; combine with --body for multipart/alternative (env: SMTPBODYHTML)")
	cmd.Flags().String("attachments", "", "Comma-separated file paths to attach (env: SMTPATTACHMENTS)")
	cmd.Flags().String("inline-attachments", "", "Comma-separated file paths to embed inline via cid:<filename> (env: SMTPINLINEATTACHMENTS)")
	cmd.Flags().StringArray("header", nil, "Custom header in 'Name: Value' form (repeatable) (env: SMTPHEADER — comma-separated; avoid commas in header values)")
	cmd.Flags().String("priority", "normal", "Email priority: high, normal, low; high/low add X-Priority, Importance, and Priority headers (env: SMTPPRIORITY)")

	return cmd
}
