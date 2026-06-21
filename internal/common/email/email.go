// Package email provides helpers shared across protocol packages for
// loading file attachments (regular and inline) and parsing custom
// email headers supplied via CLI flags.
package email

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

// Attachment represents a file to be attached to an email message, either
// as a regular attachment or as an inline part referenced via "cid:<ContentID>"
// from an HTML body.
type Attachment struct {
	Name        string
	ContentType string
	Data        []byte
	ContentID   string
	Inline      bool
}

// LoadAttachments reads files from disk as regular (non-inline) attachments.
// Files that cannot be read are skipped; onSkip (if non-nil) is called for each.
// Returns an error only if every requested path failed to load.
func LoadAttachments(paths []string, onSkip func(path string, err error)) ([]Attachment, error) {
	return loadAttachments(paths, false, onSkip)
}

// LoadInlineAttachments reads files from disk as inline attachments. Each
// attachment's ContentID is set to its base filename, so it can be referenced
// from an HTML body as "cid:<filename>".
// Files that cannot be read are skipped; onSkip (if non-nil) is called for each.
// Returns an error only if every requested path failed to load.
func LoadInlineAttachments(paths []string, onSkip func(path string, err error)) ([]Attachment, error) {
	return loadAttachments(paths, true, onSkip)
}

func loadAttachments(paths []string, inline bool, onSkip func(path string, err error)) ([]Attachment, error) {
	var attachments []Attachment
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			if onSkip != nil {
				onSkip(path, err)
			}
			continue
		}

		name := filepath.Base(path)
		contentType := mime.TypeByExtension(filepath.Ext(path))
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		a := Attachment{Name: name, ContentType: contentType, Data: data}
		if inline {
			a.Inline = true
			a.ContentID = name
		}
		attachments = append(attachments, a)
	}

	if len(attachments) == 0 && len(paths) > 0 {
		return nil, fmt.Errorf("no valid attachments could be processed")
	}

	return attachments, nil
}

// Header represents a custom email header supplied via "--header" flags.
type Header struct {
	Name  string
	Value string
}

// protectedHeaders cannot be overridden via custom --header flags because
// they are managed by the message builder itself.
var protectedHeaders = map[string]bool{
	"from":                      true,
	"to":                        true,
	"cc":                        true,
	"bcc":                       true,
	"subject":                   true,
	"date":                      true,
	"message-id":                true,
	"mime-version":              true,
	"content-type":              true,
	"content-transfer-encoding": true,
	"content-disposition":       true,
	"x-priority":                true,
	"importance":                true,
	"priority":                  true,
}

// ParseHeaders parses entries of the form "Name: Value" into Headers.
// Header values are sanitized to strip CR/LF (defense-in-depth against
// header injection). Returns an error for malformed entries, empty names,
// or names that collide with protected/managed headers.
func ParseHeaders(raw []string) ([]Header, error) {
	headers := make([]Header, 0, len(raw))
	for _, entry := range raw {
		idx := strings.Index(entry, ":")
		if idx <= 0 {
			return nil, fmt.Errorf("invalid header %q: expected \"Name: Value\" format", entry)
		}

		name := strings.TrimSpace(entry[:idx])
		value := strings.TrimSpace(entry[idx+1:])
		if name == "" {
			return nil, fmt.Errorf("invalid header %q: empty header name", entry)
		}
		if protectedHeaders[strings.ToLower(name)] {
			return nil, fmt.Errorf("header %q cannot be set via --header (managed automatically)", name)
		}

		headers = append(headers, Header{Name: name, Value: SanitizeHeaderValue(value)})
	}
	return headers, nil
}

// SanitizeHeaderValue removes CR/LF sequences from a header value to prevent
// header injection attacks. This is a defense-in-depth measure.
func SanitizeHeaderValue(value string) string {
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, "\n", "")
	return value
}
