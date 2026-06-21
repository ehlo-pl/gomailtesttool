package release

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// NewCmd returns the cobra command for 'gomailtest devtools release'.
func NewCmd() *cobra.Command {
	var (
		skipSecScan bool
		skipPush    bool
		noDevBranch bool
		dryRun      bool
		versionFile string
	)

	cmd := &cobra.Command{
		Use:   "release",
		Short: "Interactive release automation: version bump, changelog, git tag",
		Long: `Interactive release workflow for gomailtesttool.

Steps performed:
  1.  Check git working tree status
  2.  Scan files for accidentally committed secrets
  3.  Read current version from internal/common/version/version.go
  4.  Prompt for new version (patch / minor / custom)
  5.  Update version file
  6.  Collect changelog sections (Added / Changed / Fixed / Security)
  7.  Open changelog entry in editor
  8.  Commit version file + changelog
  9.  Push branch to origin
  10. Create + push git tag vX.Y.Z (triggers GitHub Actions CI/CD)
  11. Optionally create a pull request via 'gh' CLI
  12. Show recent GitHub Actions workflow runs
  13. Create dev branch bX.Y.Z+1 and bump version for next cycle

Flags:
  --dry-run             Print all actions without executing them
  --skip-security-scan  Skip the secret detection scan
  --skip-push           Do not push anything to remote
  --no-dev-branch       Skip creating the next development branch`,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := projectRoot()
			if err != nil {
				return fmt.Errorf("cannot determine project root: %w", err)
			}

			return Run(Options{
				ProjectRoot:      root,
				VersionFilePath:  versionFile,
				SkipSecurityScan: skipSecScan,
				SkipPush:         skipPush,
				NoDevBranch:      noDevBranch,
				DryRun:           dryRun,
				In:               os.Stdin,
				Out:              cmd.OutOrStdout(),
			})
		},
	}

	cmd.Flags().BoolVar(&skipSecScan, "skip-security-scan", false, "Skip secret detection scan")
	cmd.Flags().BoolVar(&skipPush, "skip-push", false, "Do not push to remote")
	cmd.Flags().BoolVar(&noDevBranch, "no-dev-branch", false, "Skip creating next dev branch")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print actions without executing them")
	cmd.Flags().StringVar(&versionFile, "version-file", "",
		"Path to version.go relative to project root (default: internal/common/version/version.go)")

	return cmd
}

// projectRoot returns the repository root by running 'git rev-parse --show-toplevel'.
func projectRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").CombinedOutput()
	if err != nil {
		// Fallback: use the directory of the running binary
		exe, _ := os.Executable()
		return filepath.Dir(exe), nil
	}
	return strings.TrimSpace(string(out)), nil
}
