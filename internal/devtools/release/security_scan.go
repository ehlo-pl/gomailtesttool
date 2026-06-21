package release

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Finding represents a potential secret detected in a file.
type Finding struct {
	File    string
	Line    int
	Kind    string
	Content string
}

var (
	reGUID   = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	reEmail  = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	reSecret = regexp.MustCompile(`(?i)(secret|password|apikey|api_key)\s*[:=]\s*\S+`)

	// False-positive patterns: lines containing these strings are ignored.
	falsePositives = []string{
		"xxx", "yyy", "zzz",
		"example.com", "example.org",
		"contoso", "fabrikam",
		"your-", "<your",
		"placeholder", "changeme",
		"00000000-0000-0000-0000-000000000000",
		"test@", "@test",
		"noreply@",
		// Go source patterns that are not real secrets
		`MaskSecret`, `MaskGUID`, `MaskEmail`, `MaskPassword`,
		`// `, "/*", " * ",
	}

	// File extensions to scan
	scanExtensions = map[string]bool{
		".go": true,
		".md": true,
	}

	// Directories to skip
	skipDirs = map[string]bool{
		".git":   true,
		"bin":    true,
		"vendor": true,
	}
)

// ScanFiles walks the project root and returns potential secret findings.
func ScanFiles(projectRoot string) ([]Finding, error) {
	var findings []Finding

	err := filepath.WalkDir(projectRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !scanExtensions[strings.ToLower(filepath.Ext(path))] {
			return nil
		}

		ff, err := scanFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: scan %s: %v\n", path, err)
			return nil
		}
		findings = append(findings, ff...)
		return nil
	})

	return findings, err
}

func scanFile(path string) ([]Finding, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var findings []Finding
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if isFalsePositive(line) {
			continue
		}

		if m := reGUID.FindString(line); m != "" {
			findings = append(findings, Finding{File: path, Line: lineNum, Kind: "GUID", Content: truncate(line)})
			continue
		}
		if m := reEmail.FindString(line); m != "" {
			findings = append(findings, Finding{File: path, Line: lineNum, Kind: "email", Content: truncate(line)})
			continue
		}
		if reSecret.MatchString(line) {
			findings = append(findings, Finding{File: path, Line: lineNum, Kind: "secret pattern", Content: truncate(line)})
		}
	}

	return findings, scanner.Err()
}

func isFalsePositive(line string) bool {
	lower := strings.ToLower(line)
	for _, fp := range falsePositives {
		if strings.Contains(lower, strings.ToLower(fp)) {
			return true
		}
	}
	return false
}

func truncate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 120 {
		return s[:117] + "..."
	}
	return s
}
