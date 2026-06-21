package serve

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/ehlo-pl/gomailtesttool/internal/common/bootstrap"
	"github.com/ehlo-pl/gomailtesttool/internal/protocols/msgraph"
	"github.com/ehlo-pl/gomailtesttool/internal/protocols/smtp"
)

// NewCmd returns the "serve" cobra.Command.
func NewCmd() *cobra.Command {
	v := viper.New()

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start an HTTP server for sending emails via REST API",
		Long: `Start an HTTP REST server that exposes email sending endpoints.

Credentials are loaded from environment variables at startup (SMTP*, MSGRAPH*).
Each request carries only message content — no credentials in request bodies.

Endpoints:
  GET  /health           health check
  POST /smtp/sendmail    send email via SMTP
  POST /msgraph/sendmail send email via Microsoft Graph
  POST /ews/sendmail     not yet implemented (501)

All non-health endpoints require the X-API-Key header.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			configPath := v.GetString("config")
			if err := bootstrap.LoadConfigFile(v, configPath); err != nil {
				return err
			}

			cfg := &Config{
				Port:   v.GetInt("port"),
				Listen: v.GetString("listen"),
				APIKey: v.GetString("api-key"),
			}

			if cfg.APIKey == "" {
				return fmt.Errorf("--api-key (or SERVE_API_KEY env var) is required")
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			slogger, _, _ := bootstrap.InitLoggers("servetool", "serve", false, "INFO", "csv")

			// Load SMTP base config from SMTP* env vars, with defaults from the
			// "smtp" section of --config (if provided).
			smtpViper := viper.New()
			smtp.BindEnvs(smtpViper)
			if err := bootstrap.LoadConfigFileSection(smtpViper, configPath, "smtp"); err != nil {
				return err
			}
			smtpBase := smtp.ConfigFromViper(smtpViper)
			smtpBase.Action = smtp.ActionSendMail
			if smtpBase.Host == "" {
				slogger.Warn("SMTPHOST not set — POST /smtp/sendmail will return 503")
				smtpBase = nil
			}

			// Load MS Graph base config and create client — resolve all values before
			// calling New() so the constructor receives the final state directly.
			// Defaults come from the "msgraph" section of --config (if provided).
			var msgraphBase *msgraph.Config
			var graphClient *msgraphsdk.GraphServiceClient

			msgraphViper := viper.New()
			msgraph.BindEnvs(msgraphViper)
			if err := bootstrap.LoadConfigFileSection(msgraphViper, configPath, "msgraph"); err != nil {
				return err
			}
			loadedBase := msgraph.ConfigFromViper(msgraphViper)
			loadedBase.Action = msgraph.ActionSendMail

			if loadedBase.TenantID == "" || loadedBase.ClientID == "" {
				slogger.Warn("MSGRAPHTENANTID or MSGRAPHCLIENTID not set — POST /msgraph/sendmail will return 503")
			} else {
				if loadedBase.ProxyURL != "" {
					os.Setenv("HTTP_PROXY", loadedBase.ProxyURL)  //nolint:errcheck
					os.Setenv("HTTPS_PROXY", loadedBase.ProxyURL) //nolint:errcheck
				}
				gc, err := msgraph.NewGraphServiceClient(ctx, loadedBase, slogger)
				if err != nil {
					slogger.Warn("MS Graph client init failed — POST /msgraph/sendmail will return 503", "error", err)
				} else {
					msgraphBase = loadedBase
					graphClient = gc
				}
			}

			return New(cfg, smtpBase, msgraphBase, graphClient, slogger).Run(ctx)
		},
	}

	cmd.Flags().Int("port", 8080, "HTTP listen port (env: SERVE_PORT)")
	cmd.Flags().String("listen", "127.0.0.1", "Bind address; use 0.0.0.0 to listen on all interfaces (env: SERVE_LISTEN)")
	cmd.Flags().String("api-key", "", "Required X-API-Key header value (env: SERVE_API_KEY)")

	_ = v.BindEnv("port", "SERVE_PORT")
	_ = v.BindEnv("listen", "SERVE_LISTEN")
	_ = v.BindEnv("api-key", "SERVE_API_KEY")

	return cmd
}
