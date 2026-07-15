// Package mime assembles RFC 5322 email messages (plain text or MIME
// multipart) in a transport-agnostic way, shared across protocol providers.
//
// Callers supply already-loaded attachments and decide, via IncludeBccHeader,
// whether Bcc appears in the message headers: SMTP relies on the envelope
// (RCPT TO) and omits it, while APIs such as Gmail derive recipients from the
// message headers and require it.
package mime

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"strings"
	"time"

	"github.com/ehlo-pl/gomailtesttool/internal/common/email"
)

// Message describes an email to assemble into an RFC 5322 byte stream.
type Message struct {
	From, Subject      string
	To, Cc, Bcc        []string
	TextBody, HTMLBody string
	Priority           string         // "high", "low", or "" / "normal"
	Headers            []email.Header // pre-parsed custom headers
	Attachments        []email.Attachment
	Inline             []email.Attachment
	// IncludeBccHeader controls whether a Bcc header is written. SMTP leaves
	// this false (recipients travel in the envelope); Gmail sets it true.
	IncludeBccHeader bool
	// MessageID is the value written inside the angle brackets of the
	// Message-ID header. If empty, one is generated from HostID.
	MessageID string
	// HostID is the "@host" portion used when generating a Message-ID.
	HostID string
	// Date is the Date header value. If zero, time.Now() is used.
	Date time.Time
}

// Build assembles m into a complete RFC 5322 message with CRLF line endings.
// When the message has no HTML body, attachments, inline parts, or custom
// headers, it produces a simple single-part text/plain message; otherwise it
// nests multipart/{mixed,related,alternative} parts as needed.
func Build(m Message) ([]byte, error) {
	messageID := m.MessageID
	if messageID == "" {
		messageID = GenerateMessageID(m.HostID, "gomailtest")
	}
	date := m.Date
	if date.IsZero() {
		date = time.Now()
	}

	hasExtras := m.HTMLBody != "" || len(m.Attachments) > 0 || len(m.Inline) > 0 || len(m.Headers) > 0

	contentType := "text/plain; charset=UTF-8"
	body := []byte(m.TextBody)
	if hasExtras {
		var err error
		contentType, body, err = buildBody(m.TextBody, m.HTMLBody, m.Inline, m.Attachments)
		if err != nil {
			return nil, err
		}
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Message-ID: <%s>\r\n", messageID)
	fmt.Fprintf(&buf, "Date: %s\r\n", date.Format(time.RFC1123Z))
	fmt.Fprintf(&buf, "From: %s\r\n", SanitizeHeader(m.From))
	fmt.Fprintf(&buf, "To: %s\r\n", strings.Join(SanitizeHeaders(m.To), ", "))
	if len(m.Cc) > 0 {
		fmt.Fprintf(&buf, "Cc: %s\r\n", strings.Join(SanitizeHeaders(m.Cc), ", "))
	}
	if m.IncludeBccHeader && len(m.Bcc) > 0 {
		fmt.Fprintf(&buf, "Bcc: %s\r\n", strings.Join(SanitizeHeaders(m.Bcc), ", "))
	}
	fmt.Fprintf(&buf, "Subject: %s\r\n", SanitizeHeader(m.Subject))
	for _, line := range PriorityHeaderLines(m.Priority) {
		buf.WriteString(line + "\r\n")
	}
	for _, h := range m.Headers {
		fmt.Fprintf(&buf, "%s: %s\r\n", h.Name, h.Value)
	}
	buf.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&buf, "Content-Type: %s\r\n", contentType)
	buf.WriteString("\r\n")
	buf.Write(body)
	if !hasExtras {
		// Preserve the historic simple plain-text form, which terminates the
		// body with a trailing CRLF. Multipart bodies are already terminated
		// by their closing boundary.
		buf.WriteString("\r\n")
	}

	return buf.Bytes(), nil
}

// buildBody assembles the MIME body for a message, nesting parts as needed:
//
//	multipart/mixed          (only if file attachments are present)
//	  multipart/related      (only if inline attachments are present)
//	    multipart/alternative (only if both plain text and HTML bodies are present)
//	      text/plain
//	      text/html
//	    inline attachment parts (cid:...)
//	  attachment parts
//
// It returns the Content-Type header value and body bytes for the outermost part.
func buildBody(textBody, htmlBody string, inline, attachments []email.Attachment) (string, []byte, error) {
	contentType, body, err := textOrAlternativePart(textBody, htmlBody)
	if err != nil {
		return "", nil, err
	}

	contentType, body, err = wrapRelatedPart(contentType, body, inline)
	if err != nil {
		return "", nil, err
	}

	return wrapMixedPart(contentType, body, attachments)
}

// textOrAlternativePart returns a single text/plain or text/html part, or
// (if both bodies are provided) a multipart/alternative wrapping both.
func textOrAlternativePart(textBody, htmlBody string) (string, []byte, error) {
	if textBody != "" && htmlBody != "" {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)

		textHeader := textproto.MIMEHeader{"Content-Type": {"text/plain; charset=UTF-8"}}
		pw, err := mw.CreatePart(textHeader)
		if err != nil {
			return "", nil, err
		}
		if _, err := pw.Write([]byte(textBody)); err != nil {
			return "", nil, err
		}

		htmlHeader := textproto.MIMEHeader{"Content-Type": {"text/html; charset=UTF-8"}}
		pw, err = mw.CreatePart(htmlHeader)
		if err != nil {
			return "", nil, err
		}
		if _, err := pw.Write([]byte(htmlBody)); err != nil {
			return "", nil, err
		}

		if err := mw.Close(); err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("multipart/alternative; boundary=%s", mw.Boundary()), buf.Bytes(), nil
	}

	if htmlBody != "" {
		return "text/html; charset=UTF-8", []byte(htmlBody), nil
	}

	return "text/plain; charset=UTF-8", []byte(textBody), nil
}

// wrapRelatedPart wraps the given part in a multipart/related part alongside
// inline attachments. If there are no inline attachments, the part is returned
// unchanged.
func wrapRelatedPart(contentType string, body []byte, inline []email.Attachment) (string, []byte, error) {
	if len(inline) == 0 {
		return contentType, body, nil
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	header := textproto.MIMEHeader{"Content-Type": {contentType}}
	pw, err := mw.CreatePart(header)
	if err != nil {
		return "", nil, err
	}
	if _, err := pw.Write(body); err != nil {
		return "", nil, err
	}

	for _, att := range inline {
		if err := writeAttachmentPart(mw, att); err != nil {
			return "", nil, err
		}
	}

	if err := mw.Close(); err != nil {
		return "", nil, err
	}

	return fmt.Sprintf("multipart/related; boundary=%s", mw.Boundary()), buf.Bytes(), nil
}

// wrapMixedPart wraps the given part in a multipart/mixed part alongside file
// attachments. If there are no attachments, the part is returned unchanged.
func wrapMixedPart(contentType string, body []byte, attachments []email.Attachment) (string, []byte, error) {
	if len(attachments) == 0 {
		return contentType, body, nil
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	header := textproto.MIMEHeader{"Content-Type": {contentType}}
	pw, err := mw.CreatePart(header)
	if err != nil {
		return "", nil, err
	}
	if _, err := pw.Write(body); err != nil {
		return "", nil, err
	}

	for _, att := range attachments {
		if err := writeAttachmentPart(mw, att); err != nil {
			return "", nil, err
		}
	}

	if err := mw.Close(); err != nil {
		return "", nil, err
	}

	return fmt.Sprintf("multipart/mixed; boundary=%s", mw.Boundary()), buf.Bytes(), nil
}

// writeAttachmentPart writes a single base64-encoded attachment part (inline or
// regular) to the given multipart writer.
func writeAttachmentPart(mw *multipart.Writer, att email.Attachment) error {
	header := textproto.MIMEHeader{
		"Content-Type":              {att.ContentType},
		"Content-Transfer-Encoding": {"base64"},
	}
	if att.Inline {
		header.Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", att.Name))
		header.Set("Content-ID", fmt.Sprintf("<%s>", att.ContentID))
	} else {
		header.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", att.Name))
	}

	pw, err := mw.CreatePart(header)
	if err != nil {
		return err
	}

	return writeBase64(pw, att.Data)
}

// writeBase64 writes base64-encoded data in RFC 2045 compliant lines of 76
// characters, terminated with CRLF.
func writeBase64(w io.Writer, data []byte) error {
	encoded := base64.StdEncoding.EncodeToString(data)
	for i := 0; i < len(encoded); i += 76 {
		end := min(i+76, len(encoded))
		if _, err := w.Write([]byte(encoded[i:end])); err != nil {
			return err
		}
		if _, err := w.Write([]byte("\r\n")); err != nil {
			return err
		}
	}
	return nil
}

// SanitizeHeader removes CRLF sequences from email header values to prevent
// header injection attacks. This is a defense-in-depth measure.
func SanitizeHeader(header string) string {
	header = strings.ReplaceAll(header, "\r", "")
	header = strings.ReplaceAll(header, "\n", "")
	return header
}

// SanitizeHeaders applies SanitizeHeader to each address in a list.
func SanitizeHeaders(addrs []string) []string {
	sanitized := make([]string, len(addrs))
	for i, addr := range addrs {
		sanitized[i] = SanitizeHeader(addr)
	}
	return sanitized
}

// PriorityHeaderLines returns the "Name: Value" header lines (without CRLF)
// for the given priority. "normal" (and any other value) adds no headers,
// since it matches default mail client behavior.
func PriorityHeaderLines(priority string) []string {
	switch priority {
	case "high":
		return []string{"X-Priority: 1 (Highest)", "Importance: High", "Priority: urgent"}
	case "low":
		return []string{"X-Priority: 5 (Lowest)", "Importance: Low", "Priority: non-urgent"}
	default:
		return nil
	}
}

// GenerateMessageID creates a unique Message-ID value (without angle brackets)
// of the form "<timestamp>.<tool>@<host>". If host is empty, tool is used as
// the host portion.
func GenerateMessageID(host, tool string) string {
	timestamp := time.Now().UnixNano()
	if host == "" {
		host = tool
	}
	return fmt.Sprintf("%d.%s@%s", timestamp, tool, host)
}
