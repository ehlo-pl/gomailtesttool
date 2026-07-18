package main

import (
	"github.com/spf13/cobra"
	"github.com/ehlo-pl/gomailtesttool/internal/common/bootstrap"
	"github.com/ehlo-pl/gomailtesttool/internal/common/version"
	"github.com/ehlo-pl/gomailtesttool/internal/devtools"
	"github.com/ehlo-pl/gomailtesttool/internal/serve"
)

var rootCmd = &cobra.Command{
	Use:     "gomailtest",
	Version: version.Get(),
	Short:   "Email and calendar protocol testing tool",
	Long: `gomailtest is a unified CLI for testing email and calendar protocols.

Supports SMTP, IMAP, POP3, JMAP, EWS, Microsoft Graph (Exchange Online), and
Gmail / Google Workspace. Protocol-subset builds may include only a selection
of these protocols (see BUILD.md for details).

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

	rootCmd.AddCommand(devtools.NewCmd())
	rootCmd.AddCommand(serve.NewCmd())
	// Protocol sub-commands are registered via per-protocol files
	// (protocols_smtp.go, protocols_imap.go, etc.) that use build tags
	// to support protocol-subset builds. See BUILD.md for details.
}

// Execute runs the root command and returns any error.
func Execute() error {
	return rootCmd.Execute()
}
