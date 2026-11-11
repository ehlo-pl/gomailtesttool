// Package probe builds the probe messages testmailflow sends and recognizes
// them again in a checked mailbox.
package probe

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/ehlo-pl/gomailtesttool/internal/common/email"
	mimebuilder "github.com/ehlo-pl/gomailtesttool/internal/common/mime"
)

// Header names and the subject marker of probe messages.
const (
	HeaderID      = "X-TestMailFlow-Id"
	HeaderFlow    = "X-TestMailFlow-Flow"
	SubjectMarker = "[TMF]"
)

// NewToken returns a random 128-bit token as 32 hex characters.
func NewToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate probe token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Subject is the probe subject: "[TMF] <flow> <token>".
func Subject(flow, token string) string {
	return SubjectMarker + " " + flow + " " + token
}

// MessageID is the probe Message-ID (without angle brackets).
func MessageID(token, from string) string {
	domain := "testmailflow.invalid"
	if i := strings.LastIndexByte(from, '@'); i >= 0 && i < len(from)-1 {
		domain = from[i+1:]
	}
	return token + ".tmf@" + domain
}

// Build assembles the probe message. The token is carried in a header, the
// subject and the body so it survives gateways that rewrite one of them.
func Build(flow, token, from string, to []string, sentAt time.Time) ([]byte, string, error) {
	msgID := MessageID(token, from)
	body := fmt.Sprintf("This is an automated mail flow probe sent by testmailflow.\r\n\r\n"+
		"Flow:  %s\r\nToken: %s\r\nSent:  %s\r\n\r\nNo action is required. The message can be deleted.\r\n",
		flow, token, sentAt.UTC().Format(time.RFC3339))
	raw, err := mimebuilder.Build(mimebuilder.Message{
		From:      from,
		To:        to,
		Subject:   Subject(flow, token),
		TextBody:  body,
		MessageID: msgID,
		Date:      sentAt,
		Headers: []email.Header{
			{Name: HeaderID, Value: token},
			{Name: HeaderFlow, Value: flow},
			{Name: "Auto-Submitted", Value: "auto-generated"},
			{Name: "X-Auto-Response-Suppress", Value: "All"},
		},
	})
	return raw, msgID, err
}

// Found describes a probe message recognized in a mailbox.
type Found struct {
	Token       string
	Flow        string
	Arrived     time.Time // from the topmost Received header; zero if unknown
	Hops        int       // number of Received headers
	AuthResults string    // compact spf/dkim/dmarc verdicts, e.g. "spf=pass dkim=pass dmarc=pass"
}

var (
	tokenPattern   = regexp.MustCompile(`^[0-9a-f]{32}$`)
	subjectPattern = regexp.MustCompile(`\[TMF\]\s+(\S+)\s+([0-9a-f]{32})`)
	verdictPattern = regexp.MustCompile(`(?i)\b(spf|dkim|dmarc|arc|compauth)=([a-z]+)`)
)

// Match parses the headers of raw and reports whether it is a probe message.
// The token is taken from the X-TestMailFlow-Id header, falling back to the
// subject when a gateway stripped the header.
func Match(raw []byte) (Found, bool) {
	var f Found
	msg, err := mail.ReadMessage(bytes.NewReader(headerBlock(raw)))
	if err != nil {
		return f, false
	}
	h := msg.Header
	f.Token = strings.ToLower(strings.TrimSpace(h.Get(HeaderID)))
	f.Flow = strings.TrimSpace(h.Get(HeaderFlow))
	if !tokenPattern.MatchString(f.Token) {
		m := subjectPattern.FindStringSubmatch(h.Get("Subject"))
		if m == nil {
			return f, false
		}
		f.Flow, f.Token = m[1], m[2]
	} else if f.Flow == "" {
		if m := subjectPattern.FindStringSubmatch(h.Get("Subject")); m != nil {
			f.Flow = m[1]
		}
	}

	received := h["Received"]
	f.Hops = len(received)
	if len(received) > 0 {
		f.Arrived = receivedDate(received[0])
	}
	f.AuthResults = verdicts(h["Authentication-Results"])
	return f, true
}

// headerBlock returns the header section of raw followed by an empty body.
func headerBlock(raw []byte) []byte {
	end := bytes.Index(raw, []byte("\r\n\r\n"))
	if end < 0 {
		end = bytes.Index(raw, []byte("\n\n"))
	}
	if end < 0 {
		end = len(raw)
	}
	return append(append([]byte{}, raw[:end]...), "\r\n\r\n"...)
}

// receivedDate parses the date after the last ';' of a Received header.
func receivedDate(v string) time.Time {
	i := strings.LastIndexByte(v, ';')
	if i < 0 {
		return time.Time{}
	}
	t, err := mail.ParseDate(strings.Join(strings.Fields(v[i+1:]), " "))
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

// verdicts extracts "method=result" pairs from the first (topmost,
// i.e. receiving system's) Authentication-Results header.
func verdicts(headers []string) string {
	if len(headers) == 0 {
		return ""
	}
	var out []string
	seen := map[string]bool{}
	for _, m := range verdictPattern.FindAllStringSubmatch(headers[0], -1) {
		key := strings.ToLower(m[1])
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key+"="+strings.ToLower(m[2]))
	}
	return strings.Join(out, " ")
}
