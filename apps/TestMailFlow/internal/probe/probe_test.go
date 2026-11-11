//go:build !integration

package probe

import (
	"strings"
	"testing"
	"time"
)

func TestBuildAndMatch_RoundTrip(t *testing.T) {
	token, err := NewToken()
	if err != nil {
		t.Fatalf("NewToken() error = %v", err)
	}
	sent := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	raw, msgID, err := Build("onprem-to-exo", token, "probe@onprem.example", []string{"probe@example.com"}, sent)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if msgID != token+".tmf@onprem.example" {
		t.Errorf("Message-ID = %q", msgID)
	}
	for _, want := range []string{"Message-ID: <" + msgID + ">", "Subject: [TMF] onprem-to-exo " + token, "Auto-Submitted: auto-generated"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("message lacks %q", want)
		}
	}

	// Simulate delivery: the receiving system prepends trace headers.
	delivered := "Received: from mx.example.com by mbx.example.com;\r\n Wed, 11 Nov 2026 10:00:42 +0000\r\n" +
		"Received: from relay by mx.example.com; Wed, 11 Nov 2026 11:00:05 +0100\r\n" +
		"Authentication-Results: mx.example.com; spf=pass smtp.mailfrom=onprem.example;\r\n dkim=none; dmarc=Pass action=none\r\n" +
		string(raw)
	f, ok := Match([]byte(delivered))
	if !ok {
		t.Fatal("Match() = false, want true")
	}
	if f.Token != token || f.Flow != "onprem-to-exo" {
		t.Errorf("Token/Flow = %q/%q", f.Token, f.Flow)
	}
	if want := sent.Add(42 * time.Second); !f.Arrived.Equal(want) {
		t.Errorf("Arrived = %v, want %v", f.Arrived, want)
	}
	if f.Hops != 2 {
		t.Errorf("Hops = %d, want 2", f.Hops)
	}
	if f.AuthResults != "spf=pass dkim=none dmarc=pass" {
		t.Errorf("AuthResults = %q", f.AuthResults)
	}
}

func TestMatch(t *testing.T) {
	const token = "0123456789abcdef0123456789abcdef"
	tests := []struct {
		name      string
		raw       string
		wantOK    bool
		wantToken string
		wantFlow  string
	}{
		{"header only", "X-TestMailFlow-Id: " + token + "\r\nSubject: rewritten\r\n\r\nbody", true, token, ""},
		{"subject fallback with prefix", "Subject: [EXTERNAL] [TMF] my.flow " + token + "\r\n\r\nbody", true, token, "my.flow"},
		{"header upper case", "X-TestMailFlow-Id: " + strings.ToUpper(token) + "\r\nX-TestMailFlow-Flow: f1\r\n\r\n", true, token, "f1"},
		{"bare LF message", "Subject: [TMF] f2 " + token + "\n\nbody", true, token, "f2"},
		{"ordinary mail", "Subject: Hello\r\nFrom: a@example.com\r\n\r\n[TMF] f " + token, false, "", ""},
		{"short token", "Subject: [TMF] f 0123abcd\r\n\r\n", false, "", ""},
		{"empty", "", false, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, ok := Match([]byte(tt.raw))
			if ok != tt.wantOK {
				t.Fatalf("Match() ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && (f.Token != tt.wantToken || f.Flow != tt.wantFlow) {
				t.Errorf("Token/Flow = %q/%q, want %q/%q", f.Token, f.Flow, tt.wantToken, tt.wantFlow)
			}
		})
	}
}
