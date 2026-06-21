package env

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// NewCmd returns the cobra command for 'gomailtest devtools env'.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Manage MSGRAPH* environment variables for integration testing",
		Long: `Display or clear Microsoft Graph environment variables used for integration tests.

Required variables:
  MSGRAPHTENANTID   Azure Active Directory tenant GUID
  MSGRAPHCLIENTID   Azure app registration client ID (GUID)
  MSGRAPHSECRET     Azure app client secret
  MSGRAPHMAILBOX    Test mailbox email address

Optional variables:
  MSGRAPHPROXY      Proxy server URL (e.g. http://proxy:8080)

To set variables (bash/zsh):
  export MSGRAPHTENANTID=<tenant-id>
  export MSGRAPHCLIENTID=<client-id>
  export MSGRAPHSECRET=<secret>
  export MSGRAPHMAILBOX=<email>

To set variables (PowerShell):
  $env:MSGRAPHTENANTID = "<tenant-id>"
  $env:MSGRAPHCLIENTID = "<client-id>"
  $env:MSGRAPHSECRET   = "<secret>"
  $env:MSGRAPHMAILBOX  = "<email>"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ShowVars(cmd.OutOrStdout())
			if missing := Missing(); len(missing) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "\nmissing required variables: %s\n", strings.Join(missing, ", "))
				return fmt.Errorf("not all required variables are set")
			}
			return nil
		},
	}

	cmd.AddCommand(newShowCmd())
	cmd.AddCommand(newClearCmd())
	cmd.AddCommand(newCheckCmd())

	return cmd
}

func newShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Display masked MSGRAPH* environment variables",
		RunE: func(cmd *cobra.Command, args []string) error {
			ShowVars(cmd.OutOrStdout())
			return nil
		},
	}
}

func newClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Print unset commands for all MSGRAPH* variables",
		Long: `Prints shell unset commands for all MSGRAPH* environment variables.

Since a child process cannot modify its parent shell's environment, run the
output of this command directly in your shell:

  bash/zsh:   eval "$(gomailtest devtools env clear)"
  PowerShell: gomailtest devtools env clear | ForEach-Object { Invoke-Expression $_ }`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ClearCommands(cmd.OutOrStdout())
			return nil
		},
	}
}

func newCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Check that all required MSGRAPH* variables are set",
		RunE: func(cmd *cobra.Command, args []string) error {
			missing := Missing()
			if len(missing) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "All required MSGRAPH* variables are set.")
				return nil
			}
			for _, name := range missing {
				fmt.Fprintf(os.Stderr, "missing: %s\n", name)
			}
			return fmt.Errorf("%d required variable(s) not set", len(missing))
		},
	}
}
