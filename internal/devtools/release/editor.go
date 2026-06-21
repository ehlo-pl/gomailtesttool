package release

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// OpenInEditor opens the file at path in the user's preferred editor.
// It respects the EDITOR environment variable, falling back to:
//   - notepad on Windows
//   - vi on Unix/macOS
//
// The call blocks until the editor exits.
func OpenInEditor(path string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		if runtime.GOOS == "windows" {
			editor = "notepad"
		} else {
			editor = "vi"
		}
	}

	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("editor %q exited with error: %w", editor, err)
	}
	return nil
}
