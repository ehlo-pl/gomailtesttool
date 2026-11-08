package dmarc

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxReportSpan is the longest date range accepted without a warning.
// RFC 7489 recommends daily reports; anything above a week is suspicious.
const MaxReportSpan = 7 * 24 * time.Hour

// ErrNotDMARC is returned when the XML document is not a DMARC feedback report.
var ErrNotDMARC = errors.New("not a DMARC aggregate report")

// Raw XML structures. Tags without a namespace match by local name, which
// makes the parser accept both RFC 7489 (no namespace) and DMARCbis documents.
type xmlFeedback struct {
	XMLName         xml.Name           `xml:"feedback"`
	Version         string             `xml:"version"`
	ReportMetadata  xmlReportMetadata  `xml:"report_metadata"`
	PolicyPublished xmlPolicyPublished `xml:"policy_published"`
	Records         []xmlRecord        `xml:"record"`
}

type xmlReportMetadata struct {
	OrgName          string   `xml:"org_name"`
	Email            string   `xml:"email"`
	ExtraContactInfo string   `xml:"extra_contact_info"`
	ReportID         string   `xml:"report_id"`
	Begin            string   `xml:"date_range>begin"`
	End              string   `xml:"date_range>end"`
	Errors           []string `xml:"error"`
}

type xmlPolicyPublished struct {
	Domain string `xml:"domain"`
	ADKIM  string `xml:"adkim"`
	ASPF   string `xml:"aspf"`
	P      string `xml:"p"`
	SP     string `xml:"sp"`
	NP     string `xml:"np"`
	Pct    string `xml:"pct"`
	FO     string `xml:"fo"`
}

type xmlRecord struct {
	Row struct {
		SourceIP        string `xml:"source_ip"`
		Count           string `xml:"count"`
		PolicyEvaluated struct {
			Disposition string `xml:"disposition"`
			DKIM        string `xml:"dkim"`
			SPF         string `xml:"spf"`
			Reasons     []struct {
				Type    string `xml:"type"`
				Comment string `xml:"comment"`
			} `xml:"reason"`
		} `xml:"policy_evaluated"`
	} `xml:"row"`
	Identifiers struct {
		HeaderFrom   string `xml:"header_from"`
		EnvelopeFrom string `xml:"envelope_from"`
		EnvelopeTo   string `xml:"envelope_to"`
	} `xml:"identifiers"`
	AuthResults struct {
		DKIM []struct {
			Domain      string `xml:"domain"`
			Selector    string `xml:"selector"`
			Result      string `xml:"result"`
			HumanResult string `xml:"human_result"`
		} `xml:"dkim"`
		SPF []struct {
			Domain string `xml:"domain"`
			Scope  string `xml:"scope"`
			Result string `xml:"result"`
		} `xml:"spf"`
	} `xml:"auth_results"`
}

// Parse decodes and normalizes a DMARC aggregate report XML document.
func Parse(data []byte) (*Feedback, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = charsetReader
	dec.Strict = true

	var raw xmlFeedback
	if err := dec.Decode(&raw); err != nil {
		var unexpected xml.UnmarshalError
		if errors.As(err, &unexpected) || strings.Contains(err.Error(), "expected element type <feedback>") {
			return nil, fmt.Errorf("%w: %v", ErrNotDMARC, err)
		}
		return nil, fmt.Errorf("parse DMARC XML: %w", err)
	}
	return normalize(&raw)
}

func normalize(raw *xmlFeedback) (*Feedback, error) {
	f := &Feedback{Version: strings.TrimSpace(raw.Version)}
	md := raw.ReportMetadata

	f.Metadata = ReportMetadata{
		OrgName:          clean(md.OrgName),
		Email:            strings.ToLower(clean(md.Email)),
		ExtraContactInfo: clean(md.ExtraContactInfo),
		ReportID:         clean(md.ReportID),
	}
	for _, e := range md.Errors {
		if e = clean(e); e != "" {
			f.Metadata.Errors = append(f.Metadata.Errors, e)
		}
	}
	if f.Metadata.ReportID == "" {
		return nil, fmt.Errorf("%w: missing report_id", ErrNotDMARC)
	}
	if f.Metadata.OrgName == "" {
		f.Metadata.OrgName = "unknown"
		f.Warnings = append(f.Warnings, "missing org_name")
	}

	begin, err := parseEpoch(md.Begin)
	if err != nil {
		return nil, fmt.Errorf("invalid date_range begin: %w", err)
	}
	end, err := parseEpoch(md.End)
	if err != nil {
		return nil, fmt.Errorf("invalid date_range end: %w", err)
	}
	if end.Before(begin) {
		f.Warnings = append(f.Warnings, "date_range end before begin; swapped")
		begin, end = end, begin
	}
	if end.Sub(begin) > MaxReportSpan {
		f.Warnings = append(f.Warnings, fmt.Sprintf("date_range spans %s (> 7 days)", end.Sub(begin)))
	}
	f.Metadata.Begin, f.Metadata.End = begin, end

	pp := raw.PolicyPublished
	f.PolicyPublished = PolicyPublished{
		Domain: domain(pp.Domain),
		ADKIM:  lower(pp.ADKIM),
		ASPF:   lower(pp.ASPF),
		P:      lower(pp.P),
		SP:     lower(pp.SP),
		NP:     lower(pp.NP),
		FO:     clean(pp.FO),
		Pct:    100,
	}
	if f.PolicyPublished.Domain == "" {
		return nil, fmt.Errorf("%w: missing policy_published domain", ErrNotDMARC)
	}
	if s := clean(pp.Pct); s != "" {
		if pct, err := strconv.Atoi(s); err == nil && pct >= 0 && pct <= 100 {
			f.PolicyPublished.Pct = pct
		} else {
			f.Warnings = append(f.Warnings, fmt.Sprintf("invalid pct %q", s))
		}
	}

	for i, xr := range raw.Records {
		r := Record{
			Disposition:  lower(xr.Row.PolicyEvaluated.Disposition),
			DKIM:         lower(xr.Row.PolicyEvaluated.DKIM),
			SPF:          lower(xr.Row.PolicyEvaluated.SPF),
			HeaderFrom:   domain(xr.Identifiers.HeaderFrom),
			EnvelopeFrom: domain(xr.Identifiers.EnvelopeFrom),
			EnvelopeTo:   domain(xr.Identifiers.EnvelopeTo),
		}
		ipStr := clean(xr.Row.SourceIP)
		if addr, err := netip.ParseAddr(ipStr); err == nil {
			r.SourceIP = addr.Unmap().String()
		} else {
			r.SourceIP = ipStr
			f.Warnings = append(f.Warnings, fmt.Sprintf("record %d: invalid source_ip %q", i, ipStr))
		}
		if s := clean(xr.Row.Count); s != "" {
			n, err := strconv.ParseInt(s, 10, 64)
			if err != nil || n < 0 {
				f.Warnings = append(f.Warnings, fmt.Sprintf("record %d: invalid count %q", i, s))
			} else {
				r.Count = n
			}
		}
		if r.Disposition == "" {
			r.Disposition = "none"
		}
		if r.HeaderFrom == "" {
			r.HeaderFrom = f.PolicyPublished.Domain
		}
		for _, rs := range xr.Row.PolicyEvaluated.Reasons {
			if t := lower(rs.Type); t != "" {
				r.Reasons = append(r.Reasons, Reason{Type: t, Comment: clean(rs.Comment)})
			}
		}
		for _, d := range xr.AuthResults.DKIM {
			r.DKIMResults = append(r.DKIMResults, DKIMResult{
				Domain: domain(d.Domain), Selector: clean(d.Selector),
				Result: lower(d.Result), HumanResult: clean(d.HumanResult),
			})
		}
		for _, s := range xr.AuthResults.SPF {
			r.SPFResults = append(r.SPFResults, SPFResult{
				Domain: domain(s.Domain), Scope: lower(s.Scope), Result: lower(s.Result),
			})
		}
		f.Records = append(f.Records, r)
	}
	return f, nil
}

func parseEpoch(s string) (time.Time, error) {
	s = clean(s)
	if s == "" {
		return time.Time{}, errors.New("empty")
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(n, 0).UTC(), nil
}

func clean(s string) string { return strings.TrimSpace(s) }
func lower(s string) string { return strings.ToLower(clean(s)) }
func domain(s string) string {
	return strings.TrimSuffix(lower(s), ".")
}

// charsetReader supports the encodings seen in real-world DMARC reports
// without pulling in golang.org/x/text: UTF-8/ASCII pass through, Latin-1
// and Windows-1252 are mapped byte-for-byte (sufficient for report XML which
// is effectively ASCII).
func charsetReader(label string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "utf-8", "utf8", "us-ascii", "ascii", "":
		return input, nil
	case "iso-8859-1", "latin1", "latin-1", "iso_8859-1", "windows-1252", "cp1252":
		data, err := io.ReadAll(input)
		if err != nil {
			return nil, err
		}
		if utf8.Valid(data) && isASCII(data) {
			return bytes.NewReader(data), nil
		}
		var b strings.Builder
		b.Grow(len(data))
		for _, c := range data {
			b.WriteRune(rune(c))
		}
		return strings.NewReader(b.String()), nil
	default:
		return nil, fmt.Errorf("unsupported charset %q", label)
	}
}

func isASCII(b []byte) bool {
	for _, c := range b {
		if c >= 0x80 {
			return false
		}
	}
	return true
}
