package main

import (
	"github.com/spf13/cobra"
	"github.com/ziembor/gomailtesttool/internal/common/bootstrap"
	"github.com/ziembor/gomailtesttool/internal/common/version"
	"github.com/ziembor/gomailtesttool/internal/devtools"
	"github.com/ziembor/gomailtesttool/internal/protocols/ews"
	"github.com/ziembor/gomailtesttool/internal/protocols/imap"
	"github.com/ziembor/gomailtesttool/internal/protocols/jmap"
	"github.com/ziembor/gomailtesttool/internal/protocols/msgraph"
	"github.com/ziembor/gomailtesttool/internal/protocols/pop3"
	"github.com/ziembor/gomailtesttool/internal/protocols/smtp"
	"github.com/ziembor/gomailtesttool/internal/serve"
)

var rootCmd = &cobra.Command{
	Use:     "gomailtest",
	Version: version.Get(),
	Short:   "Email and calendar protocol testing tool",
	Long: `gomailtest is a unified CLI for testing email and calendar protocols.

Supports SMTP, IMAP, POP3, JMAP, EWS, and Microsoft Graph (Exchange Online).

Run 'gomailtest <protocol> --help' for protocol-specific usage.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := bootstrap.SetupSignalContext()
		cmd.SetContext(ctx)
		_ = cancel // context lifetime tied to cmd context
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().String("config", "", "Path to a YAML config file providing default flag values (CLI flags and env vars still take precedence)")

	rootCmd.AddCommand(msgraph.NewCmd())
	rootCmd.AddCommand(smtp.NewCmd())
	rootCmd.AddCommand(pop3.NewCmd())
	rootCmd.AddCommand(imap.NewCmd())
	rootCmd.AddCommand(jmap.NewCmd())
	rootCmd.AddCommand(ews.NewCmd())
	rootCmd.AddCommand(devtools.NewCmd())
	rootCmd.AddCommand(serve.NewCmd())
}

// Execute runs the root command and returns any error.
func Execute() error {
	return rootCmd.Execute()
}
