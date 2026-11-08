// Package testutil builds synthetic DMARC reports and report e-mails for tests.
// All data is fabricated (documentation IP ranges, example.* domains).
package testutil

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

// Row describes one synthetic record.
type Row struct {
	IP          string
	Count       int
	Disposition string
	DKIM, SPF   string
	HeaderFrom  string
}

// Report describes one synthetic aggregate report.
type Report struct {
	Org, ReportID, Domain, Policy string
	Begin                         time.Time
	Rows                          []Row
	Namespace                     string // optional xmlns
}

// XML renders the report as RFC 7489 XML.
func (r Report) XML() []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	if r.Namespace != "" {
		fmt.Fprintf(&b, `<feedback xmlns="%s">`, r.Namespace)
	} else {
		b.WriteString(`<feedback>`)
	}
	policy := r.Policy
	if policy == "" {
		policy = "none"
	}
	fmt.Fprintf(&b, `<version>1.0</version><report_metadata><org_name>%s</org_name><email>noreply-dmarc@%s</email>`+
		`<report_id>%s</report_id><date_range><begin>%d</begin><end>%d</end></date_range></report_metadata>`,
		r.Org, strings.ToLower(strings.ReplaceAll(r.Org, " ", ""))+".example", r.ReportID,
		r.Begin.Unix(), r.Begin.Add(24*time.Hour-time.Second).Unix())
	fmt.Fprintf(&b, `<policy_published><domain>%s</domain><adkim>r</adkim><aspf>r</aspf><p>%s</p><sp>%s</sp><pct>100</pct></policy_published>`,
		r.Domain, policy, policy)
	for _, row := range r.Rows {
		hf := row.HeaderFrom
		if hf == "" {
			hf = r.Domain
		}
		disp := row.Disposition
		if disp == "" {
			disp = "none"
		}
		fmt.Fprintf(&b, `<record><row><source_ip>%s</source_ip><count>%d</count><policy_evaluated>`+
			`<disposition>%s</disposition><dkim>%s</dkim><spf>%s</spf></policy_evaluated></row>`+
			`<identifiers><header_from>%s</header_from></identifiers><auth_results>`+
			`<dkim><domain>%s</domain><selector>s1</selector><result>%s</result></dkim>`+
			`<spf><domain>%s</domain><scope>mfrom</scope><result>%s</result></spf></auth_results></record>`,
			row.IP, row.Count, disp, row.DKIM, row.SPF, hf, hf, row.DKIM, hf, row.SPF)
	}
	b.WriteString(`</feedback>`)
	return []byte(b.String())
}

// Gzip compresses data.
func Gzip(data []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write(data)
	_ = w.Close()
	return buf.Bytes()
}

// Zip stores files (name -> content) in a zip archive.
func Zip(files map[string][]byte) []byte {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, data := range files {
		f, _ := w.Create(name)
		_, _ = f.Write(data)
	}
	_ = w.Close()
	return buf.Bytes()
}

// Attachment is one MIME attachment for EML.
type Attachment struct {
	ContentType string
	Filename    string
	Data        []byte
}

// EML builds a multipart/mixed message with a text part and the attachments.
func EML(subject string, atts ...Attachment) []byte {
	const boundary = "BOUNDARY-dmarc-test"
	var b strings.Builder
	b.WriteString("From: noreply-dmarc@reporter.example\r\n")
	b.WriteString("To: dmarc@example.com\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("Date: Mon, 02 Nov 2026 10:00:00 +0000\r\n")
	b.WriteString("Message-ID: <" + strings.ReplaceAll(subject, " ", "") + "@reporter.example>\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: multipart/mixed; boundary=\"" + boundary + "\"\r\n\r\n")
	b.WriteString("--" + boundary + "\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nThis is a DMARC aggregate report.\r\n")
	for _, a := range atts {
		b.WriteString("--" + boundary + "\r\n")
		b.WriteString("Content-Type: " + a.ContentType + "; name=\"" + a.Filename + "\"\r\n")
		b.WriteString("Content-Disposition: attachment; filename=\"" + a.Filename + "\"\r\n")
		b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		enc := base64.StdEncoding.EncodeToString(a.Data)
		for len(enc) > 76 {
			b.WriteString(enc[:76] + "\r\n")
			enc = enc[76:]
		}
		b.WriteString(enc + "\r\n")
	}
	b.WriteString("--" + boundary + "--\r\n")
	return []byte(b.String())
}

// SinglePartEML builds a message whose whole body is the (base64) attachment.
func SinglePartEML(subject, contentType, filename string, data []byte) []byte {
	var b strings.Builder
	b.WriteString("From: noreply-dmarc@reporter.example\r\nTo: dmarc@example.com\r\n")
	b.WriteString("Subject: " + subject + "\r\nMIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: " + contentType + "; name=\"" + filename + "\"\r\n")
	b.WriteString("Content-Disposition: attachment; filename=\"" + filename + "\"\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	b.WriteString(base64.StdEncoding.EncodeToString(data) + "\r\n")
	return []byte(b.String())
}

// Sample returns a small standard report used across tests.
func Sample(org, id, domain string, begin time.Time) Report {
	return Report{
		Org: org, ReportID: id, Domain: domain, Policy: "quarantine", Begin: begin,
		Rows: []Row{
			{IP: "192.0.2.10", Count: 10, DKIM: "pass", SPF: "pass"},
			{IP: "198.51.100.7", Count: 3, Disposition: "quarantine", DKIM: "fail", SPF: "fail"},
			{IP: "2001:db8::1", Count: 2, DKIM: "fail", SPF: "pass"},
		},
	}
}
