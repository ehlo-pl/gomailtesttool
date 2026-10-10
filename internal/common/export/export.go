// Package export provides shared helpers for exporting messages to disk.
package export

import (
	"fmt"
	"io"
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

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("failed to create export directory %s: %w", dir, err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", fmt.Errorf("failed to secure export directory %s: %w", dir, err)
	}
	return dir, nil
}

// WriteFile writes data to path with owner-only permissions.
func WriteFile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}

	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Truncate(0); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Seek(0, 0); err != nil {
		_ = file.Close()
		return err
	}
	if n, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	} else if n != len(data) {
		_ = file.Close()
		return io.ErrShortWrite
	}
	return file.Close()
}

// SanitizeFilename replaces filesystem-unsafe characters with underscores.
func SanitizeFilename(name string) string {
	invalid := []string{"<", ">", ":", "\"", "/", "\\", "|", "?", "*", "="}
	for _, char := range invalid {
		name = strings.ReplaceAll(name, char, "_")
	}
	return name
}
