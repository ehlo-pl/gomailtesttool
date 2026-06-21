package release

import (
	"fmt"
	"os/exec"
	"strings"
)

// ghAvailable returns true if the 'gh' CLI is installed.
func ghAvailable() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// CreatePR creates a GitHub pull request using the 'gh' CLI.
func CreatePR(title, body, base string) (string, error) {
	if !ghAvailable() {
		return "", fmt.Errorf("'gh' CLI not found — install from https://cli.github.com")
	}
	out, err := exec.Command("gh", "pr", "create",
		"--title", title,
		"--body", body,
		"--base", base,
	).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh pr create: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// ListRuns lists the most recent GitHub Actions workflow runs.
func ListRuns(limit int) (string, error) {
	if !ghAvailable() {
		return "", fmt.Errorf("'gh' CLI not found")
	}
	out, err := exec.Command("gh", "run", "list", "--limit", fmt.Sprintf("%d", limit)).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh run list: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
