package release

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Sections holds the content for each changelog section.
type Sections struct {
	Added    []string
	Changed  []string
	Fixed    []string
	Security []string
}

// EntryPath returns the path for a version's changelog entry.
// e.g. "ChangeLog/3.2.0.md"
func EntryPath(projectRoot, version string) string {
	return filepath.Join(projectRoot, "ChangeLog", version+".md")
}

// CreateEntry writes a new changelog entry file.
// If the file already exists it is not overwritten; returns the path.
func CreateEntry(path, version string, s Sections) error {
	if _, err := os.Stat(path); err == nil {
		// File already exists — skip creation, user will edit it
		return nil
	}

	date := time.Now().Format("2006-01-02")
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s — %s\n\n", version, date)

	if len(s.Added) > 0 {
		sb.WriteString("## Added\n")
		for _, item := range s.Added {
			fmt.Fprintf(&sb, "- %s\n", item)
		}
		sb.WriteString("\n")
	}
	if len(s.Changed) > 0 {
		sb.WriteString("## Changed\n")
		for _, item := range s.Changed {
			fmt.Fprintf(&sb, "- %s\n", item)
		}
		sb.WriteString("\n")
	}
	if len(s.Fixed) > 0 {
		sb.WriteString("## Fixed\n")
		for _, item := range s.Fixed {
			fmt.Fprintf(&sb, "- %s\n", item)
		}
		sb.WriteString("\n")
	}
	if len(s.Security) > 0 {
		sb.WriteString("## Security\n")
		for _, item := range s.Security {
			fmt.Fprintf(&sb, "- %s\n", item)
		}
		sb.WriteString("\n")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create ChangeLog dir: %w", err)
	}
	return os.WriteFile(path, []byte(sb.String()), 0644)
}
