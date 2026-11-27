// Package inspect extracts facts from received headers and from delivery
// status notifications (bounces).
package inspect

import (
	"bufio"
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"net/netip"
	"net/textproto"
	"regexp"
	"strings"
	"time"
)

// Headers are the facts recorded about a located message.
type Headers struct {
	Arrived time.Time // from the topmost Received header; zero if unknown
	Auth    string    // "spf=pass dkim=pass dmarc=pass"
	Spam    string    // provider spam scores
	// SenderIP is the address the receiving system saw the sending MTA
	// connect from.
	SenderIP string
	// ReturnPath is the envelope sender the receiver recorded.
	ReturnPath string
}

var (
	verdictPattern = regexp.MustCompile(`(?i)\b(spf|dkim|dmarc|arc|compauth)=([a-z]+)`)
	ipPatterns     = []*regexp.Regexp{
		regexp.MustCompile(`(?i)client-ip=([0-9a-f.:]+)`),
		regexp.MustCompile(`(?i)sender ip is ([0-9a-f.:]+)`),
		regexp.MustCompile(`(?i)designates ([0-9a-f.:]+) as`),
		regexp.MustCompile(`(?i)smtp\.remote-ip=([0-9a-f.:]+)`),
	}
	spamHeaders = []string{
		"X-Microsoft-Antispam", "X-Forefront-Antispam-Report", "X-MS-Exchange-Organization-SCL",
		"X-Spam-Status", "X-Spam-Flag", "X-Spam-Score", "X-Spam-Result", "X-YahooFilteredBulk", "X-Apple-Action",
	}
	scfPattern = regexp.MustCompile(`(?i)\b(SCL|SFV|CAT|BCL):[^;]*`)
)

func parse(block []byte) (mail.Header, bool) {
	block = append(bytes.TrimRight(block, "\r\n"), "\r\n\r\n"...)
	msg, err := mail.ReadMessage(bytes.NewReader(block))
	if err != nil {
		return nil, false
	}
	return msg.Header, true
}

// ParseHeaders reads a header block.
func ParseHeaders(block []byte) Headers {
	var out Headers
	h, ok := parse(block)
	if !ok {
		return out
	}
	out.ReturnPath = strings.Trim(strings.TrimSpace(h.Get("Return-Path")), "<>")
	if r := h["Received"]; len(r) > 0 {
		if i := strings.LastIndexByte(r[0], ';'); i >= 0 {
			if t, err := mail.ParseDate(strings.Join(strings.Fields(r[0][i+1:]), " ")); err == nil {
				out.Arrived = t.UTC()
			}
		}
	}

	// Only the topmost Authentication-Results is the receiving system's.
	if a := h["Authentication-Results"]; len(a) > 0 {
		var parts []string
		seen := map[string]bool{}
		for _, m := range verdictPattern.FindAllStringSubmatch(a[0], -1) {
			key := strings.ToLower(m[1])
			if !seen[key] {
				seen[key] = true
				parts = append(parts, key+"="+strings.ToLower(m[2]))
			}
		}
		out.Auth = strings.Join(parts, " ")
	}

	var spam []string
	for _, name := range spamHeaders {
		v := h.Get(name)
		if v == "" {
			continue
		}
		if matches := scfPattern.FindAllString(v, -1); len(matches) > 0 {
			v = strings.Join(matches, ";")
		}
		if len(v) > 200 {
			v = v[:200]
		}
		spam = append(spam, name+": "+v)
	}
	out.Spam = strings.Join(spam, " | ")

	for _, name := range []string{"Received-Spf", "Authentication-Results", "Arc-Authentication-Results"} {
		for _, v := range h[name] {
			for _, p := range ipPatterns {
				if m := p.FindStringSubmatch(v); m != nil && out.SenderIP == "" {
					if ip, err := netip.ParseAddr(strings.TrimSuffix(m[1], ":")); err == nil && ip.IsGlobalUnicast() && !ip.IsPrivate() {
						out.SenderIP = ip.String()
					}
				}
			}
		}
		if out.SenderIP != "" {
			break
		}
	}
	return out
}

// ------------------------------------------------------------------ bounces

// Report is the content of a delivery status notification.
type Report struct {
	Action     string // "failed", "delayed", ...
	Status     string // enhanced status code, e.g. "4.7.28"
	RemoteMTA  string
	Diagnostic string
	// MessageIDs are the Message-IDs quoted in the report, candidates for
	// the original message.
	MessageIDs []string
}

var messageIDPattern = regexp.MustCompile(`(?im)^(?:Message-ID|X-Original-Message-ID|Original-Message-ID):\s*<([^<>\s]+)>`)

// ParseDSN reads an RFC 3464 report. It returns false for other messages.
func ParseDSN(raw []byte) (Report, bool) {
	var r Report
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return r, false
	}
	kind, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || kind != "multipart/report" || !strings.EqualFold(params["report-type"], "delivery-status") {
		return r, false
	}
	mr := multipart.NewReader(msg.Body, params["boundary"])
	for {
		part, err := mr.NextPart()
		if err != nil {
			break
		}
		data, err := io.ReadAll(io.LimitReader(part, 1<<20))
		if err != nil {
			break
		}
		partKind, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		switch partKind {
		case "message/delivery-status", "message/global-delivery-status":
			readStatus(data, &r)
		case "message/rfc822", "text/rfc822-headers", "message/global", "message/global-headers":
			for _, m := range messageIDPattern.FindAllSubmatch(data, -1) {
				r.MessageIDs = append(r.MessageIDs, string(m[1]))
			}
		}
	}
	return r, r.Action != ""
}

// readStatus parses the per-message and per-recipient field groups.
func readStatus(data []byte, r *Report) {
	data = append(bytes.TrimRight(data, "\r\n"), "\r\n\r\n"...)
	tp := textproto.NewReader(bufio.NewReader(bytes.NewReader(data)))
	for {
		h, err := tp.ReadMIMEHeader()
		if a := h.Get("Action"); a != "" && r.Action == "" {
			r.Action = strings.ToLower(strings.TrimSpace(a))
			r.Status = strings.TrimSpace(h.Get("Status"))
			r.RemoteMTA = typed(h.Get("Remote-Mta"))
			r.Diagnostic = typed(h.Get("Diagnostic-Code"))
			if len(r.Diagnostic) > 500 {
				r.Diagnostic = r.Diagnostic[:500]
			}
		}
		if err != nil {
			return
		}
	}
}

// typed strips the "dns;" or "smtp;" type prefix.
func typed(v string) string {
	if _, rest, found := strings.Cut(v, ";"); found {
		v = rest
	}
	return strings.TrimSpace(v)
}
