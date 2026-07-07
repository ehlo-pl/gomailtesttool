package release

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// PromptLine reads a single line from r after printing the prompt to w.
func PromptLine(w io.Writer, r io.Reader, prompt string) (string, error) {
	if _, err := fmt.Fprint(w, prompt); err != nil {
		return "", err
	}
	scanner := bufio.NewScanner(r)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text()), nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", io.EOF
}

// PromptYesNo asks a yes/no question and returns true if the user enters y/Y/yes.
// defaultYes controls what an empty response means.
func PromptYesNo(w io.Writer, r io.Reader, question string, defaultYes bool) (bool, error) {
	hint := "[y/N]"
	if defaultYes {
		hint = "[Y/n]"
	}
	if _, err := fmt.Fprintf(w, "%s %s: ", question, hint); err != nil {
		return false, err
	}
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, err
		}
		return defaultYes, nil
	}
	ans := strings.ToLower(strings.TrimSpace(scanner.Text()))
	if ans == "" {
		return defaultYes, nil
	}
	return ans == "y" || ans == "yes", nil
}

// PromptVersion asks for a new version, showing suggestions.
// Returns the validated version string chosen by the user.
func PromptVersion(w io.Writer, r io.Reader, current string) (string, error) {
	nextPatch, _ := SuggestNextPatch(current)
	nextMinor, _ := SuggestNextMinor(current)

	if _, err := fmt.Fprintf(w, "Current version: %s\n", current); err != nil {
		return "", err
	}
	if _, err := fmt.Fprintf(w, "  [1] Next patch: %s\n", nextPatch); err != nil {
		return "", err
	}
	if _, err := fmt.Fprintf(w, "  [2] Next minor: %s\n", nextMinor); err != nil {
		return "", err
	}
	if _, err := fmt.Fprintf(w, "  [3] Custom version\n"); err != nil {
		return "", err
	}
	if _, err := fmt.Fprint(w, "Choice [1]: "); err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		return nextPatch, nil
	}
	choice := strings.TrimSpace(scanner.Text())

	switch choice {
	case "", "1":
		return nextPatch, nil
	case "2":
		return nextMinor, nil
	case "3":
		if _, err := fmt.Fprint(w, "Enter version: "); err != nil {
			return "", err
		}
		if !scanner.Scan() {
			return "", fmt.Errorf("no version entered")
		}
		v := strings.TrimSpace(scanner.Text())
		if err := ValidateVersion(v); err != nil {
			return "", err
		}
		return v, nil
	default:
		// User may have typed a version directly
		if err := ValidateVersion(choice); err == nil {
			return choice, nil
		}
		return "", fmt.Errorf("invalid choice %q", choice)
	}
}

// PromptLines collects multiple lines for a changelog section until the user enters an empty line.
// Returns nil if no items were entered.
func PromptLines(w io.Writer, r io.Reader, section string) []string {
	_, _ = fmt.Fprintf(w, "%s (one item per line, empty line to finish):\n", section)
	scanner := bufio.NewScanner(r)
	var items []string
	for {
		_, _ = fmt.Fprint(w, "  > ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			break
		}
		items = append(items, line)
	}
	return items
}
