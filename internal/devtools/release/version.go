package release

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var versionLineRe = regexp.MustCompile(`const Version = "[^"]+"`)

// ReadVersion reads the version string from the Go version file.
// The file must contain a line of the form: const Version = "x.y.z"
func ReadVersion(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read version file: %w", err)
	}
	match := versionLineRe.Find(data)
	if match == nil {
		return "", fmt.Errorf("no version line found in %s", path)
	}
	// Extract the quoted value
	line := string(match)
	start := strings.Index(line, `"`) + 1
	end := strings.LastIndex(line, `"`)
	return line[start:end], nil
}

// WriteVersion updates the version string in the Go version file in-place.
// Only the quoted version value on the `const Version = "..."` line is changed.
func WriteVersion(path, newVersion string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read version file: %w", err)
	}
	replacement := fmt.Sprintf(`const Version = "%s"`, newVersion)
	updated := versionLineRe.ReplaceAll(data, []byte(replacement))
	if string(updated) == string(data) {
		return fmt.Errorf("version line not found or unchanged in %s", path)
	}
	return os.WriteFile(path, updated, 0644)
}

// SuggestNextPatch returns the version with the patch component incremented by 1.
// Input format: "major.minor.patch"
func SuggestNextPatch(current string) (string, error) {
	parts := strings.Split(current, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid version format %q (expected major.minor.patch)", current)
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", fmt.Errorf("invalid patch number %q: %w", parts[2], err)
	}
	return fmt.Sprintf("%s.%s.%d", parts[0], parts[1], patch+1), nil
}

// SuggestNextMinor returns the version with the minor component incremented and patch reset to 0.
func SuggestNextMinor(current string) (string, error) {
	parts := strings.Split(current, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid version format %q (expected major.minor.patch)", current)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", fmt.Errorf("invalid minor number %q: %w", parts[1], err)
	}
	return fmt.Sprintf("%s.%d.0", parts[0], minor+1), nil
}

// ValidateVersion checks that v follows the major.minor.patch format.
func ValidateVersion(v string) error {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return fmt.Errorf("version %q must be in major.minor.patch format", v)
	}
	for _, p := range parts {
		if _, err := strconv.Atoi(p); err != nil {
			return fmt.Errorf("version component %q is not a number", p)
		}
	}
	return nil
}
