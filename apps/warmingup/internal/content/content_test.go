//go:build !integration

package content

import (
	"bytes"
	"io"
	"mime"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func TestBuild(t *testing.T) {
	long := strings.Repeat("zażółć gęślą jaźń ", 200)
	raw, err := Build(Message{
		From: `"News" <news@example.com>`, To: "a@example.net", MessageID: "x1@example.com", Date: time.Unix(0, 0).UTC(),
		Item: Item{Subject: "Łódź", Text: long, Attribution: "CC BY-SA 4.0"},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	for i, line := range strings.Split(string(raw), "\r\n") {
		if len(line) > 998 || strings.ContainsAny(line, "\r\n") {
			t.Fatalf("line %d is invalid (%d bytes)", i, len(line))
		}
		for _, r := range line {
			if r > 127 {
				t.Fatalf("line %d is not 7-bit: %q", i, line)
			}
		}
	}
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if got, _ := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject")); got != "Łódź" {
		t.Errorf("Subject = %q", got)
	}
	for _, h := range []string{"Auto-Submitted", "X-Auto-Response-Suppress"} {
		if msg.Header.Get(h) != "" {
			t.Errorf("message carries %s", h)
		}
	}
	body, _ := io.ReadAll(msg.Body)
	if !bytes.Contains(body, []byte("text/html")) || !bytes.Contains(body, []byte("quoted-printable")) {
		t.Errorf("unexpected body: %.200s", body)
	}
	if _, err := Build(Message{From: "a@b.pl", To: "c@d.pl\r\nBcc: x@y.pl", MessageID: "1@b.pl"}); err == nil {
		t.Error("Build accepted header injection")
	}
}

func TestExcerpt(t *testing.T) {
	text := strings.Repeat("one two three four\n", 100)
	got := excerpt(text, 200)
	if len(got) > 200 || !strings.HasSuffix(got, "four") {
		t.Errorf("excerpt = %d bytes, %q", len(got), got[len(got)-10:])
	}
	if excerpt("short", 200) != "short" {
		t.Error("short text changed")
	}
}
