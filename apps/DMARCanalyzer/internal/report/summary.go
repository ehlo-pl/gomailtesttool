package report

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ehlo-pl/gomailtesttool/apps/DMARCanalyzer/internal/store"
)

// Options controls aggregation.
type Options struct {
	From, To    time.Time // filter on report begin (To exclusive); zero = open
	Period      Period
	Location    *time.Location
	Top         int  // max rows in source / failing lists (0 = 20)
	FailingOnly bool // restrict source list to sources with failures
	// Resolver, if set, returns a PTR name for an IP ("" if none).
	Resolver func(ctx context.Context, ip string) string
	Now      func() time.Time
}

// Metrics are the counters shown for every aggregation level.
type Metrics struct {
	Messages   int64   `json:"messages"`
	Pass       int64   `json:"dmarc_pass"`
	Fail       int64   `json:"dmarc_fail"`
	PassPct    float64 `json:"dmarc_pass_pct"`
	DKIMPass   int64   `json:"dkim_aligned_pass"`
	DKIMPct    float64 `json:"dkim_aligned_pass_pct"`
	SPFPass    int64   `json:"spf_aligned_pass"`
	SPFPct     float64 `json:"spf_aligned_pass_pct"`
	None       int64   `json:"disposition_none"`
	Quarantine int64   `json:"disposition_quarantine"`
	Reject     int64   `json:"disposition_reject"`
	Reports    int     `json:"reports"`
	Reporters  int     `json:"reporters"`
	SourceIPs  int     `json:"source_ips"`

	reports   map[int64]struct{}
	reporters map[string]struct{}
	ips       map[string]struct{}
}

func (m *Metrics) add(r store.RecordRow) {
	if m.reports == nil {
		m.reports, m.reporters, m.ips = map[int64]struct{}{}, map[string]struct{}{}, map[string]struct{}{}
	}
	m.Messages += r.Count
	if r.DMARCPass {
		m.Pass += r.Count
	} else {
		m.Fail += r.Count
	}
	if r.DKIM == "pass" {
		m.DKIMPass += r.Count
	}
	if r.SPF == "pass" {
		m.SPFPass += r.Count
	}
	switch r.Disposition {
	case "quarantine":
		m.Quarantine += r.Count
	case "reject":
		m.Reject += r.Count
	default:
		m.None += r.Count
	}
	m.reports[r.ReportDBID] = struct{}{}
	m.reporters[r.OrgName] = struct{}{}
	m.ips[r.SourceIP] = struct{}{}
}

func (m *Metrics) finalize() {
	m.PassPct, m.DKIMPct, m.SPFPct = pct(m.Pass, m.Messages), pct(m.DKIMPass, m.Messages), pct(m.SPFPass, m.Messages)
	m.Reports, m.Reporters, m.SourceIPs = len(m.reports), len(m.reporters), len(m.ips)
}

func pct(n, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(int64(float64(n)*10000/float64(total)+0.5)) / 100
}

// PeriodRow is one time bucket.
type PeriodRow struct {
	Label string    `json:"label"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	Metrics
	// DeltaPassPct is the change in DMARC pass % vs the previous bucket.
	DeltaPassPct *float64 `json:"delta_pass_pct,omitempty"`
}

// DomainRow summarizes one header_from domain.
type DomainRow struct {
	Domain string `json:"domain"`
	Policy string `json:"policy"` // latest published policy seen, e.g. "p=reject sp=reject pct=100"
	Metrics
}

// ReporterRow summarizes one reporting organization.
type ReporterRow struct {
	Org string `json:"org"`
	Metrics
}

// SourceRow summarizes one sending IP.
type SourceRow struct {
	IP          string   `json:"ip"`
	PTR         string   `json:"ptr,omitempty"`
	HeaderFroms []string `json:"header_from"`
	Metrics
}

// FailingRow is an actionable DMARC failure group.
type FailingRow struct {
	IP          string `json:"ip"`
	PTR         string `json:"ptr,omitempty"`
	HeaderFrom  string `json:"header_from"`
	Disposition string `json:"disposition"`
	DKIM        string `json:"dkim_auth"` // raw DKIM results, e.g. "mailer.example/s1=pass"
	SPF         string `json:"spf_auth"`
	Reasons     string `json:"override_reasons,omitempty"`
	Messages    int64  `json:"messages"`
	Reporters   string `json:"reporters"`
	Hint        string `json:"hint"`
}

// ReasonRow counts policy override reasons.
type ReasonRow struct {
	Reason   string `json:"reason"`
	Messages int64  `json:"messages"`
}

// PolicyChange records a change of published policy for a domain.
type PolicyChange struct {
	Domain string    `json:"domain"`
	SeenAt time.Time `json:"seen_at"`
	From   string    `json:"from"`
	To     string    `json:"to"`
}

// Summary is the full report model shared by all renderers.
type Summary struct {
	Generated     time.Time      `json:"generated"`
	From          time.Time      `json:"from,omitempty"`
	To            time.Time      `json:"to,omitempty"`
	Period        Period         `json:"period"`
	Timezone      string         `json:"timezone"`
	Overall       Metrics        `json:"overall"`
	Periods       []PeriodRow    `json:"periods"`
	Domains       []DomainRow    `json:"domains"`
	Reporters     []ReporterRow  `json:"reporters"`
	TopSources    []SourceRow    `json:"top_sources"`
	Failing       []FailingRow   `json:"failing_sources"`
	Reasons       []ReasonRow    `json:"override_reasons"`
	PolicyChanges []PolicyChange `json:"policy_changes"`
}

// Build aggregates records into a Summary.
func Build(ctx context.Context, rows []store.RecordRow, opt Options) *Summary {
	if opt.Location == nil {
		opt.Location = time.UTC
	}
	if opt.Top <= 0 {
		opt.Top = 20
	}
	if opt.Period == "" {
		opt.Period = Month
	}
	now := time.Now
	if opt.Now != nil {
		now = opt.Now
	}
	s := &Summary{
		Generated: now().UTC().Truncate(time.Second), From: opt.From, To: opt.To,
		Period: opt.Period, Timezone: opt.Location.String(),
	}

	periods := map[string]*PeriodRow{}
	domains := map[string]*DomainRow{}
	reporters := map[string]*ReporterRow{}
	sources := map[string]*SourceRow{}
	sourceFroms := map[string]map[string]int64{}
	failing := map[string]*FailingRow{}
	failingOrgs := map[string]map[string]struct{}{}
	reasons := map[string]int64{}
	lastPolicy := map[string]string{} // policy_domain -> latest policy (rows are ordered by begin)

	for _, r := range rows {
		s.Overall.add(r)

		start, end, label := Bucket(r.Begin, opt.Period, opt.Location)
		if opt.Period == Total {
			start, end = opt.From, opt.To
		}
		pr := periods[label]
		if pr == nil {
			pr = &PeriodRow{Label: label, Start: start, End: end}
			periods[label] = pr
		}
		pr.add(r)

		dr := domains[r.HeaderFrom]
		if dr == nil {
			dr = &DomainRow{Domain: r.HeaderFrom}
			domains[r.HeaderFrom] = dr
		}
		dr.add(r)

		polStr := fmt.Sprintf("p=%s sp=%s pct=%d", orDash(r.P), orDash(r.SP), r.Pct)
		if prev, ok := lastPolicy[r.PolicyDomain]; ok && prev != polStr {
			s.PolicyChanges = append(s.PolicyChanges, PolicyChange{Domain: r.PolicyDomain, SeenAt: r.Begin, From: prev, To: polStr})
		}
		lastPolicy[r.PolicyDomain] = polStr

		rr := reporters[r.OrgName]
		if rr == nil {
			rr = &ReporterRow{Org: r.OrgName}
			reporters[r.OrgName] = rr
		}
		rr.add(r)

		sr := sources[r.SourceIP]
		if sr == nil {
			sr = &SourceRow{IP: r.SourceIP}
			sources[r.SourceIP] = sr
			sourceFroms[r.SourceIP] = map[string]int64{}
		}
		sr.add(r)
		sourceFroms[r.SourceIP][r.HeaderFrom] += r.Count

		if r.Reasons != "" {
			for _, reason := range strings.Split(r.Reasons, ", ") {
				reasons[reason] += r.Count
			}
		}

		if !r.DMARCPass {
			key := strings.Join([]string{r.SourceIP, r.HeaderFrom, r.Disposition, r.DKIMAuth, r.SPFAuth, r.Reasons}, "|")
			fr := failing[key]
			if fr == nil {
				fr = &FailingRow{IP: r.SourceIP, HeaderFrom: r.HeaderFrom, Disposition: r.Disposition,
					DKIM: r.DKIMAuth, SPF: r.SPFAuth, Reasons: r.Reasons}
				failing[key] = fr
				failingOrgs[key] = map[string]struct{}{}
			}
			fr.Messages += r.Count
			failingOrgs[key][r.OrgName] = struct{}{}
		}
	}
	s.Overall.finalize()

	// Periods: chronological, with trend.
	for _, p := range periods {
		p.finalize()
		s.Periods = append(s.Periods, *p)
	}
	sort.Slice(s.Periods, func(i, j int) bool { return s.Periods[i].Label < s.Periods[j].Label })
	for i := 1; i < len(s.Periods); i++ {
		d := s.Periods[i].PassPct - s.Periods[i-1].PassPct
		d = float64(int64(d*100+sign(d)*0.5)) / 100
		s.Periods[i].DeltaPassPct = &d
	}

	for name, d := range domains {
		d.finalize()
		d.Policy = policyFor(name, lastPolicy)
		s.Domains = append(s.Domains, *d)
	}
	sort.Slice(s.Domains, func(i, j int) bool {
		return byVolume(s.Domains[i].Metrics, s.Domains[j].Metrics, s.Domains[i].Domain, s.Domains[j].Domain)
	})

	for _, r := range reporters {
		r.finalize()
		s.Reporters = append(s.Reporters, *r)
	}
	sort.Slice(s.Reporters, func(i, j int) bool {
		return byVolume(s.Reporters[i].Metrics, s.Reporters[j].Metrics, s.Reporters[i].Org, s.Reporters[j].Org)
	})

	for ip, src := range sources {
		src.finalize()
		if opt.FailingOnly && src.Fail == 0 {
			continue
		}
		src.HeaderFroms = topKeys(sourceFroms[ip], 3)
		s.TopSources = append(s.TopSources, *src)
	}
	sort.Slice(s.TopSources, func(i, j int) bool {
		a, b := s.TopSources[i], s.TopSources[j]
		if opt.FailingOnly && a.Fail != b.Fail {
			return a.Fail > b.Fail
		}
		return byVolume(a.Metrics, b.Metrics, a.IP, b.IP)
	})
	s.TopSources = limit(s.TopSources, opt.Top)

	for key, f := range failing {
		f.Reporters = strings.Join(sortedKeys(failingOrgs[key]), ", ")
		f.Hint = hint(*f)
		s.Failing = append(s.Failing, *f)
	}
	sort.Slice(s.Failing, func(i, j int) bool {
		if s.Failing[i].Messages != s.Failing[j].Messages {
			return s.Failing[i].Messages > s.Failing[j].Messages
		}
		return s.Failing[i].IP < s.Failing[j].IP
	})
	s.Failing = limit(s.Failing, opt.Top)

	for reason, n := range reasons {
		s.Reasons = append(s.Reasons, ReasonRow{Reason: reason, Messages: n})
	}
	sort.Slice(s.Reasons, func(i, j int) bool {
		if s.Reasons[i].Messages != s.Reasons[j].Messages {
			return s.Reasons[i].Messages > s.Reasons[j].Messages
		}
		return s.Reasons[i].Reason < s.Reasons[j].Reason
	})

	if opt.Resolver != nil {
		ptr := map[string]string{}
		lookup := func(ip string) string {
			if v, ok := ptr[ip]; ok {
				return v
			}
			v := opt.Resolver(ctx, ip)
			ptr[ip] = v
			return v
		}
		for i := range s.TopSources {
			s.TopSources[i].PTR = lookup(s.TopSources[i].IP)
		}
		for i := range s.Failing {
			s.Failing[i].PTR = lookup(s.Failing[i].IP)
		}
	}
	return s
}

// hint gives a short, conservative explanation for a failure group.
func hint(f FailingRow) string {
	switch {
	case strings.Contains(f.Reasons, "forwarded") || strings.Contains(f.Reasons, "mailing_list"):
		return "forwarding / mailing list (reporter override)"
	case strings.Contains(f.DKIM, "=pass") && !strings.Contains(f.DKIM, f.HeaderFrom+"/"):
		return "DKIM passes for another domain: sign with header_from domain (alignment)"
	case strings.Contains(f.SPF, "=pass"):
		return "SPF passes for another domain: envelope sender not aligned"
	case f.DKIM == "" && f.SPF == "":
		return "no authentication results: likely spoofing or unknown sender"
	default:
		return "SPF and DKIM fail: unauthorized sender or missing SPF include / DKIM key"
	}
}

func policyFor(domain string, m map[string]string) string {
	// Walk up the domain tree to the organizational policy domain seen in reports.
	for d := domain; d != ""; {
		if v, ok := m[d]; ok {
			return v
		}
		i := strings.IndexByte(d, '.')
		if i < 0 {
			break
		}
		d = d[i+1:]
	}
	return ""
}

func byVolume(a, b Metrics, ka, kb string) bool {
	if a.Messages != b.Messages {
		return a.Messages > b.Messages
	}
	return ka < kb
}

func topKeys(m map[string]int64, n int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	return limit(keys, n)
}

func sortedKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func limit[T any](s []T, n int) []T {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func sign(f float64) float64 {
	if f < 0 {
		return -1
	}
	return 1
}

// IPs returns the distinct IPs shown in the source and failing sections.
func (s *Summary) IPs() []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(ip string) {
		if _, ok := seen[ip]; !ok {
			seen[ip] = struct{}{}
			out = append(out, ip)
		}
	}
	for _, r := range s.TopSources {
		add(r.IP)
	}
	for _, r := range s.Failing {
		add(r.IP)
	}
	return out
}

// ApplyPTR fills PTR names from a pre-resolved map.
func (s *Summary) ApplyPTR(ptr map[string]string) {
	for i := range s.TopSources {
		s.TopSources[i].PTR = ptr[s.TopSources[i].IP]
	}
	for i := range s.Failing {
		s.Failing[i].PTR = ptr[s.Failing[i].IP]
	}
}
