// Package dmarc contains the data model and parser for DMARC aggregate (rua)
// reports as defined by RFC 7489 Appendix C, tolerant of the DMARCbis
// aggregate-report namespace (urn:ietf:params:xml:ns:dmarc-2.0).
package dmarc

import "time"

// Feedback is a parsed and normalized DMARC aggregate report.
type Feedback struct {
	Version         string          `json:"version,omitempty"`
	Metadata        ReportMetadata  `json:"report_metadata"`
	PolicyPublished PolicyPublished `json:"policy_published"`
	Records         []Record        `json:"records"`
	// Warnings collects non-fatal normalization issues (bad IP, odd date range, ...).
	Warnings []string `json:"warnings,omitempty"`
}

// ReportMetadata describes who sent the report and which period it covers.
type ReportMetadata struct {
	OrgName          string    `json:"org_name"`
	Email            string    `json:"email,omitempty"`
	ExtraContactInfo string    `json:"extra_contact_info,omitempty"`
	ReportID         string    `json:"report_id"`
	Begin            time.Time `json:"begin"`
	End              time.Time `json:"end"`
	Errors           []string  `json:"errors,omitempty"`
}

// PolicyPublished is the DMARC policy the reporter saw in DNS.
type PolicyPublished struct {
	Domain string `json:"domain"`
	ADKIM  string `json:"adkim,omitempty"`
	ASPF   string `json:"aspf,omitempty"`
	P      string `json:"p"`
	SP     string `json:"sp,omitempty"`
	NP     string `json:"np,omitempty"`
	Pct    int    `json:"pct"`
	FO     string `json:"fo,omitempty"`
}

// Record is one row of the report: a source IP plus identifiers and results.
type Record struct {
	SourceIP     string       `json:"source_ip"`
	Count        int64        `json:"count"`
	Disposition  string       `json:"disposition"`
	DKIM         string       `json:"dkim"` // policy_evaluated (aligned) DKIM result
	SPF          string       `json:"spf"`  // policy_evaluated (aligned) SPF result
	Reasons      []Reason     `json:"reasons,omitempty"`
	HeaderFrom   string       `json:"header_from"`
	EnvelopeFrom string       `json:"envelope_from,omitempty"`
	EnvelopeTo   string       `json:"envelope_to,omitempty"`
	DKIMResults  []DKIMResult `json:"dkim_results,omitempty"`
	SPFResults   []SPFResult  `json:"spf_results,omitempty"`
}

// DMARCPass reports whether the record passed DMARC (aligned DKIM or SPF pass).
func (r Record) DMARCPass() bool {
	return r.DKIM == "pass" || r.SPF == "pass"
}

// Reason is a policy override reason (forwarded, mailing_list, ...).
type Reason struct {
	Type    string `json:"type"`
	Comment string `json:"comment,omitempty"`
}

// DKIMResult is one raw (unaligned) DKIM authentication result.
type DKIMResult struct {
	Domain      string `json:"domain"`
	Selector    string `json:"selector,omitempty"`
	Result      string `json:"result"`
	HumanResult string `json:"human_result,omitempty"`
}

// SPFResult is one raw (unaligned) SPF authentication result.
type SPFResult struct {
	Domain string `json:"domain"`
	Scope  string `json:"scope,omitempty"`
	Result string `json:"result"`
}

// TotalCount returns the sum of message counts over all records.
func (f *Feedback) TotalCount() int64 {
	var n int64
	for _, r := range f.Records {
		n += r.Count
	}
	return n
}
