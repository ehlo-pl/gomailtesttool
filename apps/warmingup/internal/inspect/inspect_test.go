//go:build !integration

package inspect

import (
	"strings"
	"testing"
)

func TestParseHeaders(t *testing.T) {
	h := ParseHeaders([]byte(strings.Join([]string{
		"Return-Path: <bounces@example.com>",
		"Received: from mail.example.com by mx.google.com; Mon, 7 Dec 2026 10:00:05 +0100",
		"Received-SPF: pass (google.com: domain of a@example.com designates 203.0.113.7 as permitted sender) client-ip=203.0.113.7;",
		"Authentication-Results: mx.google.com; dkim=pass header.i=@example.com; spf=pass; dmarc=pass",
		"Authentication-Results: older; spf=fail",
		"X-Forefront-Antispam-Report: CIP:203.0.113.7;CTRY:PL;SCL:5;SFV:SPM;",
		"", "",
	}, "\r\n")))
	if h.Auth != "dkim=pass spf=pass dmarc=pass" {
		t.Errorf("Auth = %q", h.Auth)
	}
	if h.ReturnPath != "bounces@example.com" {
		t.Errorf("ReturnPath = %q", h.ReturnPath)
	}
	if h.SenderIP != "203.0.113.7" {
		t.Errorf("SenderIP = %q", h.SenderIP)
	}
	if !strings.Contains(h.Spam, "SCL:5;SFV:SPM") {
		t.Errorf("Spam = %q", h.Spam)
	}
	if h.Arrived.Hour() != 9 {
		t.Errorf("Arrived = %v", h.Arrived)
	}
}

func TestParseDSN(t *testing.T) {
	dsn := strings.Join([]string{
		"From: MAILER-DAEMON@example.com",
		"Content-Type: multipart/report; report-type=delivery-status; boundary=b",
		"",
		"--b",
		"Content-Type: text/plain",
		"",
		"delayed",
		"--b",
		"Content-Type: message/delivery-status",
		"",
		"Reporting-MTA: dns; mail.example.com",
		"",
		"Final-Recipient: rfc822; u@gmail.com",
		"Action: delayed",
		"Status: 4.7.28",
		"Remote-MTA: dns; gmail-smtp-in.l.google.com",
		"Diagnostic-Code: smtp; 421-4.7.28 unusual rate of unsolicited mail",
		"",
		"--b",
		"Content-Type: text/rfc822-headers",
		"",
		"Message-ID: <abc@example.com>",
		"Subject: x",
		"",
		"--b--",
		"",
	}, "\r\n")
	r, ok := ParseDSN([]byte(dsn))
	if !ok || r.Action != "delayed" || r.Status != "4.7.28" || r.RemoteMTA != "gmail-smtp-in.l.google.com" {
		t.Fatalf("ParseDSN() = %+v, %v", r, ok)
	}
	if len(r.MessageIDs) != 1 || r.MessageIDs[0] != "abc@example.com" {
		t.Errorf("MessageIDs = %v", r.MessageIDs)
	}
	if _, ok := ParseDSN([]byte("Subject: hi\r\n\r\nbody")); ok {
		t.Error("plain message parsed as DSN")
	}
}
