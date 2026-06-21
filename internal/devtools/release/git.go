package release

import (
	"fmt"
	"os/exec"
	"strings"
)

// gitRun executes a git command and returns combined output.
// Returns an error that includes stderr on failure.
func gitRun(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		return trimmed, fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, trimmed)
	}
	return trimmed, nil
}

// Status returns true if the working tree is dirty (has uncommitted changes).
func Status() (dirty bool, output string, err error) {
	out, err := gitRun("status", "--porcelain")
	if err != nil {
		return false, "", err
	}
	return out != "", out, nil
}

// CurrentBranch returns the name of the current git branch.
func CurrentBranch() (string, error) {
	return gitRun("rev-parse", "--abbrev-ref", "HEAD")
}

// Add stages the given file paths.
func Add(paths ...string) error {
	args := append([]string{"add", "--"}, paths...)
	_, err := gitRun(args...)
	return err
}

// Commit creates a commit with the given message.
func Commit(message string) error {
	_, err := gitRun("commit", "-m", message)
	return err
}

// Push pushes the current branch to remote.
func Push(remote, branch string) error {
	_, err := gitRun("push", remote, branch)
	return err
}

// Tag creates a lightweight git tag.
func Tag(name string) error {
	_, err := gitRun("tag", name)
	return err
}

// PushTag pushes a tag to remote.
func PushTag(remote, tag string) error {
	_, err := gitRun("push", remote, tag)
	return err
}

// CheckoutNewBranch creates and checks out a new branch.
func CheckoutNewBranch(name string) error {
	_, err := gitRun("checkout", "-b", name)
	return err
}
