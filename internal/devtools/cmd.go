// Package devtools provides developer-facing CLI subcommands for managing
// the project: environment configuration, release automation, etc.
package devtools

import (
	"github.com/spf13/cobra"
	"github.com/ehlo-pl/gomailtesttool/internal/devtools/env"
	"github.com/ehlo-pl/gomailtesttool/internal/devtools/release"
)

// NewCmd returns the cobra command for 'gomailtest devtools'.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "devtools",
		Short: "Developer tools: release management and environment configuration",
		Long: `Developer tools for managing the gomailtesttool project.

Subcommands:
  env      — manage MSGRAPH* environment variables for integration testing
  release  — interactive release automation (version bump, changelog, git tag)`,
	}

	cmd.AddCommand(env.NewCmd())
	cmd.AddCommand(release.NewCmd())

	return cmd
}
