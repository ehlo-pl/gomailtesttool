package dmarc

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ehlo-pl/gomailtesttool/apps/DMARCanalyzer/internal/testutil"
)

var day = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

func TestParseSample(t *testing.T) {
	f, err := Parse(testutil.Sample("Google Inc", "r-1", "Example.COM.", day).XML())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if f.Metadata.OrgName != "Google Inc" || f.Metadata.ReportID != "r-1" {
		t.Errorf("metadata = %+v", f.Metadata)
	}
	if !f.Metadata.Begin.Equal(day) {
		t.Errorf("begin = %v, want %v", f.Metadata.Begin, day)
	}
	if f.PolicyPublished.Domain != "example.com" || f.PolicyPublished.P != "quarantine" || f.PolicyPublished.Pct != 100 {
		t.Errorf("policy = %+v", f.PolicyPublished)
	}
	if len(f.Records) != 3 {
		t.Fatalf("records = %d", len(f.Records))
	}
	if got := f.TotalCount(); got != 15 {
		t.Errorf("TotalCount = %d, want 15", got)
	}
	passes := 0
	for _, r := range f.Records {
		if r.DMARCPass() {
			passes++
		}
	}
	if passes != 2 {
		t.Errorf("passing records = %d, want 2", passes)
	}
	if f.Records[2].SourceIP != "2001:db8::1" {
		t.Errorf("ipv6 = %q", f.Records[2].SourceIP)
	}
	if len(f.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", f.Warnings)
	}
}

func TestParseVariants(t *testing.T) {
	tests := []struct {
		name    string
		xml     string
		wantErr error
		anyErr  bool
		check   func(t *testing.T, f *Feedback)
	}{
		{
			name: "dmarcbis namespace",
			xml: string(testutil.Report{Org: "X", ReportID: "ns", Domain: "example.org", Begin: day,
				Namespace: "urn:ietf:params:xml:ns:dmarc-2.0",
				Rows:      []testutil.Row{{IP: "192.0.2.1", Count: 1, DKIM: "pass", SPF: "fail"}}}.XML()),
			check: func(t *testing.T, f *Feedback) {
				if len(f.Records) != 1 || f.PolicyPublished.Domain != "example.org" {
					t.Errorf("got %+v", f)
				}
			},
		},
		{
			name: "missing optional fields and whitespace",
			xml: `<feedback><report_metadata><org_name> Acme </org_name><report_id> 42 </report_id>
<date_range><begin> 1793491200 </begin><end>1793577599</end></date_range></report_metadata>
<policy_published><domain>example.net</domain><p>REJECT</p></policy_published>
<record><row><source_ip>192.0.2.5</source_ip><count>7</count><policy_evaluated><dkim>pass</dkim><spf>fail</spf></policy_evaluated></row>
<identifiers></identifiers></record></feedback>`,
			check: func(t *testing.T, f *Feedback) {
				r := f.Records[0]
				if r.Disposition != "none" || r.HeaderFrom != "example.net" || r.Count != 7 {
					t.Errorf("record = %+v", r)
				}
				if f.PolicyPublished.P != "reject" || f.Metadata.ReportID != "42" {
					t.Errorf("policy/meta = %+v %+v", f.PolicyPublished, f.Metadata)
				}
			},
		},
		{
			name: "huge count and bad ip warning",
			xml: `<feedback><report_metadata><org_name>A</org_name><report_id>1</report_id><date_range><begin>0</begin><end>86399</end></date_range></report_metadata>
<policy_published><domain>example.com</domain><p>none</p></policy_published>
<record><row><source_ip>not-an-ip</source_ip><count>9000000000</count><policy_evaluated><disposition>none</disposition><dkim>pass</dkim><spf>pass</spf></policy_evaluated></row><identifiers><header_from>example.com</header_from></identifiers></record></feedback>`,
			check: func(t *testing.T, f *Feedback) {
				if f.Records[0].Count != 9000000000 {
					t.Errorf("count = %d", f.Records[0].Count)
				}
				if len(f.Warnings) != 1 || !strings.Contains(f.Warnings[0], "source_ip") {
					t.Errorf("warnings = %v", f.Warnings)
				}
			},
		},
		{
			name: "latin1 charset",
			xml:  "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?><feedback><report_metadata><org_name>Caf\xe9</org_name><report_id>1</report_id><date_range><begin>0</begin><end>1</end></date_range></report_metadata><policy_published><domain>example.com</domain><p>none</p></policy_published></feedback>",
			check: func(t *testing.T, f *Feedback) {
				if f.Metadata.OrgName != "Café" {
					t.Errorf("org = %q", f.Metadata.OrgName)
				}
			},
		},
		{
			name: "long span warning",
			xml:  `<feedback><report_metadata><org_name>A</org_name><report_id>1</report_id><date_range><begin>0</begin><end>2000000</end></date_range></report_metadata><policy_published><domain>example.com</domain><p>none</p></policy_published></feedback>`,
			check: func(t *testing.T, f *Feedback) {
				if len(f.Warnings) != 1 {
					t.Errorf("warnings = %v", f.Warnings)
				}
			},
		},
		{name: "not dmarc root", xml: `<html><body/></html>`, wantErr: ErrNotDMARC},
		{name: "missing report id", xml: `<feedback><report_metadata><org_name>A</org_name></report_metadata></feedback>`, wantErr: ErrNotDMARC},
		{name: "broken xml", xml: `<feedback><report_metadata>`, anyErr: true},
		{name: "bad begin", xml: `<feedback><report_metadata><org_name>A</org_name><report_id>1</report_id><date_range><begin>x</begin><end>1</end></date_range></report_metadata></feedback>`, anyErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Parse([]byte(tt.xml))
			switch {
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			case tt.anyErr:
				if err == nil {
					t.Fatal("expected error")
				}
				return
			case err != nil:
				t.Fatalf("Parse: %v", err)
			}
			tt.check(t, f)
		})
	}
}
