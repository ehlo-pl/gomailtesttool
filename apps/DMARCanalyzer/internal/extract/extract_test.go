package extract

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ehlo-pl/gomailtesttool/apps/DMARCanalyzer/internal/dmarc"
	tu "github.com/ehlo-pl/gomailtesttool/apps/DMARCanalyzer/internal/testutil"
)

var day = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

func TestFromEML(t *testing.T) {
	xml := tu.Sample("Google", "g-1", "example.com", day).XML()
	xml2 := tu.Sample("Yahoo", "y-1", "example.com", day).XML()

	tests := []struct {
		name         string
		eml          []byte
		wantPayloads int
		wantForensic bool
	}{
		{"zip attachment", tu.EML("Report domain: example.com", tu.Attachment{ContentType: "application/zip", Filename: "google.com!example.com!1!2.zip", Data: tu.Zip(map[string][]byte{"r.xml": xml})}), 1, false},
		{"gzip attachment", tu.EML("Report", tu.Attachment{ContentType: "application/gzip", Filename: "outlook.com!example.com!1!2.xml.gz", Data: tu.Gzip(xml)}), 1, false},
		{"plain xml", tu.EML("Report", tu.Attachment{ContentType: "text/xml", Filename: "r.xml", Data: xml}), 1, false},
		{"octet-stream by name", tu.EML("Report", tu.Attachment{ContentType: "application/octet-stream", Filename: "r.xml.gz", Data: tu.Gzip(xml)}), 1, false},
		{"octet-stream unrelated", tu.EML("Hello", tu.Attachment{ContentType: "application/octet-stream", Filename: "photo.jpg", Data: []byte("\xff\xd8")}), 0, false},
		{"multiple attachments", tu.EML("Report", tu.Attachment{ContentType: "application/zip", Filename: "a.zip", Data: tu.Zip(map[string][]byte{"a.xml": xml})}, tu.Attachment{ContentType: "application/gzip", Filename: "b.xml.gz", Data: tu.Gzip(xml2)}), 2, false},
		{"single-part gzip body", tu.SinglePartEML("Report", "application/gzip", "r.xml.gz", tu.Gzip(xml)), 1, false},
		{"zip with two reports", tu.EML("Report", tu.Attachment{ContentType: "application/zip", Filename: "two.zip", Data: tu.Zip(map[string][]byte{"a.xml": xml, "b.xml": xml2, "readme.txt": []byte("hi")})}), 2, false},
		{"forensic report", forensicEML(), 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := FromEML(tt.eml, DefaultLimits)
			if err != nil {
				t.Fatalf("FromEML: %v", err)
			}
			if len(res.Payloads) != tt.wantPayloads {
				t.Fatalf("payloads = %d, want %d (warnings %v)", len(res.Payloads), tt.wantPayloads, res.Warnings)
			}
			if res.Forensic != tt.wantForensic {
				t.Errorf("forensic = %v", res.Forensic)
			}
			for _, p := range res.Payloads {
				if _, err := dmarc.Parse(p.Data); err != nil {
					t.Errorf("payload %s does not parse: %v", p.Name, err)
				}
			}
		})
	}
}

func TestUnpackLimits(t *testing.T) {
	big := bytes.Repeat([]byte("A"), 2<<20)
	lim := Limits{MaxCompressed: 1 << 20, MaxDecompressed: 1 << 20, MaxZipEntries: 2}

	if _, err := Unpack("bomb.xml.gz", tu.Gzip(append([]byte("<?xml?>"), big...)), lim); !errors.Is(err, ErrTooLarge) {
		t.Errorf("gzip bomb: err = %v", err)
	}
	if _, err := Unpack("bomb.zip", tu.Zip(map[string][]byte{"a.xml": append([]byte("<?xml?>"), big...)}), lim); !errors.Is(err, ErrTooLarge) {
		t.Errorf("zip bomb: err = %v", err)
	}
	many := map[string][]byte{"a.xml": nil, "b.xml": nil, "c.xml": nil}
	if _, err := Unpack("many.zip", tu.Zip(many), lim); err == nil || !strings.Contains(err.Error(), "entries") {
		t.Errorf("too many entries: err = %v", err)
	}
	if _, err := Unpack("trav.zip", tu.Zip(map[string][]byte{"../x.xml": []byte("<?xml?>")}), lim); err == nil {
		t.Error("expected path traversal rejection")
	}
	if _, err := Unpack("huge.xml", big, lim); !errors.Is(err, ErrTooLarge) {
		t.Errorf("compressed limit: err = %v", err)
	}
	if p, err := Unpack("notes.txt", []byte("hello"), lim); err != nil || p != nil {
		t.Errorf("unknown: %v %v", p, err)
	}
}

func forensicEML() []byte {
	return []byte("From: a@reporter.example\r\nTo: ruf@example.com\r\nSubject: Fail\r\nMIME-Version: 1.0\r\n" +
		"Content-Type: multipart/report; report-type=feedback-report; boundary=\"b\"\r\n\r\n" +
		"--b\r\nContent-Type: text/plain\r\n\r\nfailure\r\n" +
		"--b\r\nContent-Type: message/feedback-report\r\n\r\nFeedback-Type: auth-failure\r\n\r\n" +
		"--b\r\nContent-Type: text/rfc822-headers\r\n\r\nFrom: x@example.com\r\n\r\n--b--\r\n")
}
