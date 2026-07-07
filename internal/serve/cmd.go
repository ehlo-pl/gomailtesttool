package serve

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/ehlo-pl/gomailtesttool/internal/common/bootstrap"
	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	"github.com/ehlo-pl/gomailtesttool/internal/protocols/msgraph"
	"github.com/ehlo-pl/gomailtesttool/internal/protocols/smtp"
)

// NewCmd returns the "serve" cobra.Command.
func NewCmd() *cobra.Command {
	v := viper.New()

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start an HTTP and/or MCP server for sending emails",
		Long: `Start a server that exposes email sending endpoints over HTTP REST and MCP.

Credentials are loaded from environment variables at startup (SMTP*, MSGRAPH*).
Each request carries only message content — no credentials in request bodies.

HTTP endpoints:
  GET  /health           health check
  POST /smtp/sendmail    send email via SMTP
  POST /msgraph/sendmail send email via Microsoft Graph
  POST /ews/sendmail     not yet implemented (501)

All non-health endpoints require the X-API-Key header.

The same sendmail capabilities are also exposed over MCP (Model Context Protocol):
  - Streamable HTTP at POST /mcp on the HTTP server (behind X-API-Key), enabled by
    default; disable with --mcp=false.
  - stdio, via --mcp-stdio, for local AI clients (e.g. Claude Desktop/Code) that
    launch this binary as a subprocess. In stdio mode no HTTP server is started
    and no API key is required.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())

			configPath := v.GetString("config")
			if err := bootstrap.LoadConfigFile(v, configPath); err != nil {
				return err
			}

			cfg := &Config{
				Port:      v.GetInt("port"),
				Listen:    v.GetString("listen"),
				APIKey:    v.GetString("api-key"),
				EnableMCP: v.GetBool("mcp"),
			}
			mcpStdio := v.GetBool("mcp-stdio")

			// In stdio MCP mode, stdout is the JSON-RPC channel. Preserve it for
			// the transport, then repoint the process's stdout to stderr so the
			// reused send functions' progress output and the CSV "Logging to:"
			// banner cannot corrupt the protocol. This MUST happen before any
			// logging is initialised (InitLoggers prints that banner).
			var mcpStdout io.WriteCloser
			if mcpStdio {
				mcpStdout = nopWriteCloser{os.Stdout}
				os.Stdout = os.Stderr
			}

			// The API key guards network exposure; stdio mode is a local
			// subprocess with no listening socket, so it is not required there.
			if !mcpStdio && cfg.APIKey == "" {
				return fmt.Errorf("--api-key (or SERVE_API_KEY env var) is required")
			}

			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()

			// slog writes to stderr in both modes. The CSV file logger (whose
			// stdout banner is unsafe for stdio) is only initialised for HTTP mode.
			var slogger *slog.Logger
			if mcpStdio {
				slogger = logger.SetupLogger(false, "INFO")
			} else {
				slogger, _, _ = bootstrap.InitLoggers("servetool", "serve", false, "INFO", "csv")
			}

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

			srv := New(cfg, smtpBase, msgraphBase, graphClient, slogger)
			if mcpStdio {
				return srv.RunMCPStdio(ctx, os.Stdin, mcpStdout)
			}
			return srv.Run(ctx)
		},
	}

	cmd.Flags().Int("port", 8080, "HTTP listen port (env: SERVE_PORT)")
	cmd.Flags().String("listen", "127.0.0.1", "Bind address; use 0.0.0.0 to listen on all interfaces (env: SERVE_LISTEN)")
	cmd.Flags().String("api-key", "", "Required X-API-Key header value (env: SERVE_API_KEY)")
	cmd.Flags().Bool("mcp", true, "Mount the MCP (Streamable HTTP) endpoint at /mcp on the HTTP server (env: SERVE_MCP)")
	cmd.Flags().Bool("mcp-stdio", false, "Run as an MCP server over stdio instead of the HTTP server; no API key required (env: SERVE_MCP_STDIO)")

	_ = v.BindEnv("port", "SERVE_PORT")
	_ = v.BindEnv("listen", "SERVE_LISTEN")
	_ = v.BindEnv("api-key", "SERVE_API_KEY")
	_ = v.BindEnv("mcp", "SERVE_MCP")
	_ = v.BindEnv("mcp-stdio", "SERVE_MCP_STDIO")

	return cmd
}
