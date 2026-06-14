// Package export provides shared helpers for exporting messages to disk.
package export

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CreateExportDir creates and returns the export directory:
// <baseDir>/export/<YYYY-MM-DD>/. If baseDir is empty, the OS temp directory
// is used, reproducing the default %TEMP%/export/<date>/ layout.
func CreateExportDir(baseDir string) (string, error) {
	if baseDir == "" {
		baseDir = os.TempDir()
	}
	dateStr := time.Now().Format("2006-01-02")
	dir := filepath.Join(baseDir, "export", dateStr)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create export directory %s: %w", dir, err)
	}
	return dir, nil
}

// SanitizeFilename replaces filesystem-unsafe characters with underscores.
func SanitizeFilename(name string) string {
	invalid := []string{"<", ">", ":", "\"", "/", "\\", "|", "?", "*", "="}
	for _, char := range invalid {
		name = strings.ReplaceAll(name, char, "_")
	}
	return name
}
