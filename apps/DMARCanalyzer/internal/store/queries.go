package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Stats summarizes database contents.
type Stats struct {
	Sources         int
	FetchedMessages int
	Files           map[string]int // by status
	Reports         int
	Records         int
	Messages        int64 // sum of record counts
	Reporters       int
	Domains         int
	FirstReport     time.Time
	LastReport      time.Time
}

// Stats returns database statistics.
func (s *Store) Stats(ctx context.Context) (*Stats, error) {
	st := &Stats{Files: map[string]int{}}
	counts := []struct {
		q   string
		dst *int
	}{
		{`SELECT COUNT(*) FROM sources`, &st.Sources},
		{`SELECT COUNT(*) FROM fetched_messages`, &st.FetchedMessages},
		{`SELECT COUNT(*) FROM reports`, &st.Reports},
		{`SELECT COUNT(*) FROM records`, &st.Records},
		{`SELECT COUNT(DISTINCT org_name) FROM reports`, &st.Reporters},
		{`SELECT COUNT(DISTINCT policy_domain) FROM reports`, &st.Domains},
	}
	for _, c := range counts {
		if err := s.db.QueryRowContext(ctx, c.q).Scan(c.dst); err != nil {
			return nil, fmt.Errorf("stats: %w", err)
		}
	}
	var msgs sql.NullInt64
	var first, last sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT SUM(count) FROM records`).Scan(&msgs); err != nil {
		return nil, err
	}
	st.Messages = msgs.Int64
	if err := s.db.QueryRowContext(ctx, `SELECT MIN(begin_ts), MAX(end_ts) FROM reports`).Scan(&first, &last); err != nil {
		return nil, err
	}
	if first.Valid {
		st.FirstReport = time.Unix(first.Int64, 0).UTC()
		st.LastReport = time.Unix(last.Int64, 0).UTC()
	}
	rows, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM processed_files GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, err
		}
		st.Files[status] = n
	}
	return st, rows.Err()
}

// RecordFilter selects records for reporting. Zero values mean "no filter".
type RecordFilter struct {
	From    time.Time // report begin_ts >= From
	To      time.Time // report begin_ts <  To
	Domains []string  // policy_domain or header_from (exact or subdomain)
	Orgs    []string  // reporter org_name (case-insensitive)
}

// RecordRow is one record joined with its report metadata.
type RecordRow struct {
	ReportDBID   int64
	OrgName      string
	ReportID     string
	Begin, End   time.Time
	PolicyDomain string
	P, SP        string
	Pct          int
	SourceIP     string
	Count        int64
	Disposition  string
	DKIM, SPF    string
	HeaderFrom   string
	EnvelopeFrom string
	DMARCPass    bool
	DKIMAuth     string // e.g. "example.com/s1=pass, mailer.example=fail"
	SPFAuth      string // e.g. "bounce.example.com=pass"
	Reasons      string // e.g. "forwarded, mailing_list"
}

// Records loads report records matching the filter, ordered by report begin.
func (s *Store) Records(ctx context.Context, f RecordFilter) ([]RecordRow, error) {
	var where []string
	var args []any
	if !f.From.IsZero() {
		where = append(where, "r.begin_ts >= ?")
		args = append(args, f.From.Unix())
	}
	if !f.To.IsZero() {
		where = append(where, "r.begin_ts < ?")
		args = append(args, f.To.Unix())
	}
	if len(f.Domains) > 0 {
		var ors []string
		for _, d := range f.Domains {
			d = strings.ToLower(strings.TrimSpace(d))
			ors = append(ors, "r.policy_domain = ? OR c.header_from = ? OR c.header_from LIKE ? ESCAPE '\\'")
			args = append(args, d, d, "%."+escapeLike(d))
		}
		where = append(where, "("+strings.Join(ors, " OR ")+")")
	}
	if len(f.Orgs) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(f.Orgs)), ",")
		where = append(where, "LOWER(r.org_name) IN ("+ph+")")
		for _, o := range f.Orgs {
			args = append(args, strings.ToLower(o))
		}
	}
	q := `SELECT r.id, r.org_name, r.report_id, r.begin_ts, r.end_ts, r.policy_domain,
	        COALESCE(r.p,''), COALESCE(r.sp,''), COALESCE(r.pct,100),
	        c.source_ip, c.count, c.disposition, COALESCE(c.dkim_eval,''), COALESCE(c.spf_eval,''),
	        c.header_from, COALESCE(c.envelope_from,''), c.dmarc_pass,
	        COALESCE((SELECT GROUP_CONCAT(COALESCE(a.domain,'') || CASE WHEN a.selector IS NOT NULL THEN '/' || a.selector ELSE '' END || '=' || COALESCE(a.result,''), ', ')
	                  FROM record_auth a WHERE a.record_id=c.id AND a.kind='dkim'),''),
	        COALESCE((SELECT GROUP_CONCAT(COALESCE(a.domain,'') || '=' || COALESCE(a.result,''), ', ')
	                  FROM record_auth a WHERE a.record_id=c.id AND a.kind='spf'),''),
	        COALESCE((SELECT GROUP_CONCAT(x.type, ', ') FROM record_reasons x WHERE x.record_id=c.id),'')
	      FROM records c JOIN reports r ON r.id = c.report_id`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY r.begin_ts, r.id, c.id"

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query records: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []RecordRow
	for rows.Next() {
		var r RecordRow
		var begin, end int64
		var pass int
		if err := rows.Scan(&r.ReportDBID, &r.OrgName, &r.ReportID, &begin, &end, &r.PolicyDomain, &r.P, &r.SP, &r.Pct,
			&r.SourceIP, &r.Count, &r.Disposition, &r.DKIM, &r.SPF, &r.HeaderFrom, &r.EnvelopeFrom, &pass,
			&r.DKIMAuth, &r.SPFAuth, &r.Reasons); err != nil {
			return nil, err
		}
		r.Begin, r.End, r.DMARCPass = time.Unix(begin, 0).UTC(), time.Unix(end, 0).UTC(), pass == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
