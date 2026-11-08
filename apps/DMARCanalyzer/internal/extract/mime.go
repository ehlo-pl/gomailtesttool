package extract

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strings"
)

const maxMultipartDepth = 10

// Result is what was found in one e-mail message.
type Result struct {
	Subject  string
	Forensic bool      // multipart/report; report-type=feedback-report (ruf)
	Payloads []Payload // candidate aggregate report XML documents
	Warnings []string  // per-part problems that did not abort processing
}

var wordDecoder = &mime.WordDecoder{}

// reportTypes are attachment media types that can carry an aggregate report.
var reportTypes = map[string]bool{
	"application/zip":              true,
	"application/x-zip-compressed": true,
	"application/x-zip":            true,
	"application/gzip":             true,
	"application/x-gzip":           true,
	"application/xml":              true,
	"text/xml":                     true,
}

// FromEML parses a raw RFC 5322 message and returns any DMARC report payloads.
func FromEML(raw []byte, lim Limits) (*Result, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("parse message: %w", err)
	}
	res := &Result{}
	if s, err := wordDecoder.DecodeHeader(msg.Header.Get("Subject")); err == nil {
		res.Subject = s
	} else {
		res.Subject = msg.Header.Get("Subject")
	}
	err = walk(textproto.MIMEHeader(msg.Header), msg.Body, res, lim, 0)
	return res, err
}

func walk(h textproto.MIMEHeader, body io.Reader, res *Result, lim Limits, depth int) error {
	if depth > maxMultipartDepth {
		return fmt.Errorf("multipart nesting exceeds %d levels", maxMultipartDepth)
	}
	mediaType, params := "text/plain", map[string]string{}
	if ct := h.Get("Content-Type"); ct != "" {
		mt, p, err := mime.ParseMediaType(ct)
		if err != nil {
			// Some reporters send sloppy headers; fall back to the bare type.
			mt = strings.ToLower(strings.TrimSpace(strings.SplitN(ct, ";", 2)[0]))
		}
		mediaType, params = mt, p
		if params == nil {
			params = map[string]string{}
		}
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		if mediaType == "multipart/report" && strings.EqualFold(params["report-type"], "feedback-report") {
			res.Forensic = true
		}
		boundary := params["boundary"]
		if boundary == "" {
			return fmt.Errorf("multipart %s without boundary", mediaType)
		}
		mr := multipart.NewReader(body, boundary)
		for {
			part, err := mr.NextRawPart()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return fmt.Errorf("read multipart: %w", err)
			}
			if err := walk(part.Header, part, res, lim, depth+1); err != nil {
				res.Warnings = append(res.Warnings, err.Error())
			}
		}
	}
	if mediaType == "message/feedback-report" {
		res.Forensic = true
		return nil
	}

	name := filename(h, params)
	if !isCandidate(mediaType, name) {
		return nil
	}
	data, err := decode(h.Get("Content-Transfer-Encoding"), body, lim.MaxCompressed)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if name == "" {
		name = "attachment"
	}
	payloads, err := Unpack(name, data, lim)
	if err != nil {
		return err
	}
	res.Payloads = append(res.Payloads, payloads...)
	return nil
}

func isCandidate(mediaType, name string) bool {
	if reportTypes[mediaType] {
		return true
	}
	lower := strings.ToLower(name)
	for _, ext := range []string{".xml", ".xml.gz", ".gz", ".zip", ".gzip"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func filename(h textproto.MIMEHeader, ctParams map[string]string) string {
	if cd := h.Get("Content-Disposition"); cd != "" {
		if _, p, err := mime.ParseMediaType(cd); err == nil && p["filename"] != "" {
			return decodeWord(p["filename"])
		}
	}
	return decodeWord(ctParams["name"])
}

func decodeWord(s string) string {
	if d, err := wordDecoder.DecodeHeader(s); err == nil {
		return d
	}
	return s
}

func decode(encoding string, r io.Reader, limit int64) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		r = base64.NewDecoder(base64.StdEncoding, &stripWhitespace{r: r})
	case "quoted-printable":
		r = quotedprintable.NewReader(r)
	case "", "7bit", "8bit", "binary":
	default:
		return nil, fmt.Errorf("unsupported Content-Transfer-Encoding %q", encoding)
	}
	return readLimited(r, limit)
}

// stripWhitespace removes spaces and tabs that some senders leave in base64
// bodies (base64.NewDecoder only skips CR and LF).
type stripWhitespace struct{ r io.Reader }

func (s *stripWhitespace) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	j := 0
	for i := 0; i < n; i++ {
		if p[i] != ' ' && p[i] != '\t' {
			p[j] = p[i]
			j++
		}
	}
	return j, err
}
