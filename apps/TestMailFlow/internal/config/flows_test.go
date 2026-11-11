//go:build !integration

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const sampleYAML = `
testmailflow:
  http: 127.0.0.1:9925
endpoints:
  relay:  { protocol: smtp, host: smtp.example.com, port: 25 }
  exo:    { protocol: msgraph, tenantid: 11111111-1111-1111-1111-111111111111, clientid: 22222222-2222-2222-2222-222222222222, secret: "${TMF_TEST_SECRET}", mailbox: probe@example.com }
  popbox: { protocol: pop3, host: pop.example.net, pop3s: true, username: probe, password: "pa$$word" }
flows:
  - name: onprem-to-exo
    send:  { endpoint: relay, from: probe@onprem.example, to: probe@example.com }
    check: { endpoint: exo, folder: inbox, delete: true }
    interval: 10m
    checks: [1m, 2m30s, 5m]
    warn_after: 90s
  - name: exo-to-pop
    send:  { endpoint: exo, to: [probe@example.net, second@example.net] }
    check: { endpoint: popbox }
`

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tmf.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func TestLoadPlan_Valid(t *testing.T) {
	t.Setenv("TMF_TEST_SECRET", "s3cr3t-value")
	plan, err := LoadPlan(writeConfig(t, sampleYAML))
	if err != nil {
		t.Fatalf("LoadPlan() error = %v", err)
	}
	if len(plan.Endpoints) != 3 || len(plan.Flows) != 2 {
		t.Fatalf("got %d endpoints, %d flows; want 3, 2", len(plan.Endpoints), len(plan.Flows))
	}
	if got := plan.Endpoints["exo"].String("secret"); got != "s3cr3t-value" {
		t.Errorf("secret = %q, want expanded env value", got)
	}
	if got := plan.Endpoints["popbox"].String("password"); got != "pa$$word" {
		t.Errorf("password = %q, want literal with dollar signs", got)
	}

	f := plan.Flow("onprem-to-exo")
	if f == nil {
		t.Fatal("flow onprem-to-exo not found")
	}
	if f.Interval != 10*time.Minute || f.WarnAfter != 90*time.Second || !f.Delete || f.Folder != "inbox" {
		t.Errorf("flow fields = %+v", f)
	}
	if f.CheckOffset(0) != time.Minute || f.CheckOffset(1) != 210*time.Second || f.Deadline() != 510*time.Second {
		t.Errorf("offsets = %v, %v, deadline %v", f.CheckOffset(0), f.CheckOffset(1), f.Deadline())
	}

	g := plan.Flow("exo-to-pop")
	if g.From != "probe@example.com" {
		t.Errorf("From = %q, want the Graph mailbox as default sender", g.From)
	}
	if len(g.To) != 2 || g.Interval != DefaultInterval || len(g.Checks) != len(DefaultChecks) || g.WarnAfter != DefaultChecks[0] {
		t.Errorf("defaults not applied: %+v", g)
	}

	masked := strings.Join(plan.Endpoints["exo"].MaskedSettings(), " ")
	if strings.Contains(masked, "s3cr3t-value") || strings.Contains(masked, "11111111-1111-1111-1111-111111111111") {
		t.Errorf("MaskedSettings() leaks secrets: %s", masked)
	}
}

func TestLoadPlan_Invalid(t *testing.T) {
	base := func(flow string) string {
		return "endpoints:\n" +
			"  relay: { protocol: smtp, host: smtp.example.com }\n" +
			"  box: { protocol: imap, host: imap.example.com, username: u, password: p }\n" +
			"flows:\n" + flow
	}
	tests := []struct {
		name     string
		yaml     string
		errorMsg string
	}{
		{"no endpoints", "flows:\n  - name: a\n", "'endpoints:' must define"},
		{"no flows", "endpoints:\n  relay: { protocol: smtp, host: h }\n", "'flows:' must list"},
		{"unknown protocol", "endpoints:\n  x: { protocol: ews }\nflows:\n  - name: a\n", "unsupported protocol"},
		{"missing env", "endpoints:\n  x: { protocol: smtp, host: h, password: \"${TMF_NOT_SET_ANYWHERE}\" }\nflows:\n  - name: a\n", "TMF_NOT_SET_ANYWHERE is not set"},
		{"bad name", base("  - name: \"bad name\"\n"), "'name' is required"},
		{"unknown send endpoint", base("  - name: a\n    send: { endpoint: nope, from: a@example.com, to: b@example.com }\n    check: { endpoint: box }\n"), "send.endpoint \"nope\" is not defined"},
		{"send over imap", base("  - name: a\n    send: { endpoint: box, from: a@example.com, to: b@example.com }\n    check: { endpoint: box }\n"), "sending supports"},
		{"check over smtp", base("  - name: a\n    send: { endpoint: relay, from: a@example.com, to: b@example.com }\n    check: { endpoint: relay }\n"), "checking supports"},
		{"bad from", base("  - name: a\n    send: { endpoint: relay, from: not-an-address, to: b@example.com }\n    check: { endpoint: box }\n"), "send.from"},
		{"no recipient", base("  - name: a\n    send: { endpoint: relay, from: a@example.com }\n    check: { endpoint: box }\n"), "send.to is required"},
		{"short interval", base("  - name: a\n    send: { endpoint: relay, from: a@example.com, to: b@example.com }\n    check: { endpoint: box }\n    interval: 5s\n"), "at least 30s"},
		{"bad check delay", base("  - name: a\n    send: { endpoint: relay, from: a@example.com, to: b@example.com }\n    check: { endpoint: box }\n    checks: [5m, soon]\n"), "invalid duration"},
		{"duplicate flow", base("  - name: a\n    send: { endpoint: relay, from: a@example.com, to: b@example.com }\n    check: { endpoint: box }\n" +
			"  - name: A\n    send: { endpoint: relay, from: a@example.com, to: b@example.com }\n    check: { endpoint: box }\n"), "duplicate flow name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadPlan(writeConfig(t, tt.yaml))
			if err == nil || !strings.Contains(err.Error(), tt.errorMsg) {
				t.Fatalf("LoadPlan() error = %v, want substring %q", err, tt.errorMsg)
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"2m30s", 150 * time.Second, false},
		{"90d", 90 * 24 * time.Hour, false},
		{"2w", 14 * 24 * time.Hour, false},
		{" 1H ", time.Hour, false},
		{"", 0, true},
		{"5", 0, true},
		{"soon", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseDuration(tt.in)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("ParseDuration(%q) = %v, %v; want %v, error %v", tt.in, got, err, tt.want, tt.wantErr)
			}
		})
	}
}
