// Package store persists collected messages, processed files and parsed
// DMARC aggregate reports in a SQLite database (pure Go driver, no CGO).
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"github.com/ehlo-pl/gomailtesttool/apps/DMARCanalyzer/internal/dmarc"
)

//go:embed schema.sql
var schemaV1 string

// schemaVersion is stored in PRAGMA user_version.
const schemaVersion = 1

// Store wraps the SQLite database.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// Open opens (creating if needed) the database at path. Use ":memory:" for tests.
func Open(ctx context.Context, path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
		// Create the file up front with owner-only permissions.
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			return nil, fmt.Errorf("create database file: %w", err)
		}
		_ = f.Close()
	}
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	if path != ":memory:" {
		dsn += "&_pragma=journal_mode(WAL)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1) // single-writer CLI; also keeps :memory: on one connection
	s := &Store{db: db, now: time.Now}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// DB exposes the underlying handle for ad-hoc queries.
func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) migrate(ctx context.Context) error {
	var v int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if v > schemaVersion {
		return fmt.Errorf("database schema version %d is newer than supported %d; upgrade dmarcanalyzer", v, schemaVersion)
	}
	if v < 1 {
		if _, err := s.db.ExecContext(ctx, schemaV1); err != nil {
			return fmt.Errorf("apply schema v1: %w", err)
		}
	}
	// Future migrations: if v < 2 { ... }
	_, err := s.db.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", schemaVersion))
	return err
}

// ---------------------------------------------------------------- sources

// Source identifies a mailbox/folder collected over a protocol.
type Source struct {
	ID            int64
	Protocol      string
	Mailbox       string
	Folder        string
	LastCollectAt time.Time // zero if never collected
}

// EnsureSource returns the source row, creating it if needed.
func (s *Store) EnsureSource(ctx context.Context, protocol, mailbox, folder string) (*Source, error) {
	mailbox, folder = strings.ToLower(mailbox), strings.ToLower(folder)
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO sources(protocol, mailbox, folder) VALUES(?,?,?) ON CONFLICT DO NOTHING`,
		protocol, mailbox, folder); err != nil {
		return nil, fmt.Errorf("insert source: %w", err)
	}
	src := &Source{Protocol: protocol, Mailbox: mailbox, Folder: folder}
	var last sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT id, last_collect_at FROM sources WHERE protocol=? AND mailbox=? AND folder=?`,
		protocol, mailbox, folder).Scan(&src.ID, &last)
	if err != nil {
		return nil, fmt.Errorf("load source: %w", err)
	}
	if last.Valid {
		src.LastCollectAt = time.Unix(last.Int64, 0).UTC()
	}
	return src, nil
}

// SetLastCollect records a successful collection run.
func (s *Store) SetLastCollect(ctx context.Context, sourceID int64, t time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sources SET last_collect_at=? WHERE id=?`, t.Unix(), sourceID)
	return err
}

// IsFetched reports whether msgKey was already downloaded for the source.
func (s *Store) IsFetched(ctx context.Context, sourceID int64, msgKey string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM fetched_messages WHERE source_id=? AND msg_key=?`, sourceID, msgKey).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// FetchedMessage is one downloaded message.
type FetchedMessage struct {
	MsgKey     string
	ReceivedAt time.Time
	EMLPath    string
	SHA256     string
}

// RecordFetched stores a downloaded message key.
func (s *Store) RecordFetched(ctx context.Context, sourceID int64, m FetchedMessage) error {
	var recv any
	if !m.ReceivedAt.IsZero() {
		recv = m.ReceivedAt.Unix()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO fetched_messages(source_id,msg_key,received_at,eml_path,sha256,fetched_at)
		 VALUES(?,?,?,?,?,?) ON CONFLICT(source_id,msg_key) DO UPDATE SET
		 eml_path=excluded.eml_path, sha256=excluded.sha256, fetched_at=excluded.fetched_at`,
		sourceID, m.MsgKey, recv, m.EMLPath, m.SHA256, s.now().Unix())
	return err
}

// MarkDeleted records that the server copy of a message was deleted.
func (s *Store) MarkDeleted(ctx context.Context, sourceID int64, msgKey string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE fetched_messages SET deleted_at=? WHERE source_id=? AND msg_key=?`, s.now().Unix(), sourceID, msgKey)
	return err
}

// ----------------------------------------------------------- processed files

// File status values.
const (
	StatusOK        = "ok"
	StatusDuplicate = "duplicate"
	StatusNoReport  = "no_report"
	StatusForensic  = "forensic_skipped"
	StatusError     = "error"
)

// ProcessedFile is the extraction outcome for one input file.
type ProcessedFile struct {
	Path         string
	SHA256       string
	Status       string
	ReportsFound int
	Error        string
}

// FileSHA returns the stored hash for a processed file, or "" if unknown.
func (s *Store) FileSHA(ctx context.Context, path string) (string, error) {
	var sha string
	err := s.db.QueryRowContext(ctx, `SELECT sha256 FROM processed_files WHERE path=?`, path).Scan(&sha)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return sha, err
}

// MarkFile records the outcome of processing a file.
func (s *Store) MarkFile(ctx context.Context, f ProcessedFile) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO processed_files(path,sha256,status,reports_found,error,processed_at) VALUES(?,?,?,?,?,?)
		 ON CONFLICT(path) DO UPDATE SET sha256=excluded.sha256, status=excluded.status,
		 reports_found=excluded.reports_found, error=excluded.error, processed_at=excluded.processed_at`,
		f.Path, f.SHA256, f.Status, f.ReportsFound, nullable(f.Error), s.now().Unix())
	return err
}

// ------------------------------------------------------------------ reports

// InsertReport stores a parsed report and its records in one transaction.
// It returns false (and no error) when the same report (org, report_id,
// domain) is already stored.
func (s *Store) InsertReport(ctx context.Context, f *dmarc.Feedback, xmlSHA, sourceFile string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	md, pp := f.Metadata, f.PolicyPublished
	res, err := tx.ExecContext(ctx,
		`INSERT INTO reports(org_name,org_email,report_id,begin_ts,end_ts,policy_domain,adkim,aspf,p,sp,np,pct,fo,
		 warnings,xml_sha256,source_file,imported_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(org_name,report_id,policy_domain) DO NOTHING`,
		md.OrgName, nullable(md.Email), md.ReportID, md.Begin.Unix(), md.End.Unix(), pp.Domain,
		nullable(pp.ADKIM), nullable(pp.ASPF), nullable(pp.P), nullable(pp.SP), nullable(pp.NP), pp.Pct, nullable(pp.FO),
		nullable(strings.Join(f.Warnings, "; ")), xmlSHA, nullable(sourceFile), s.now().Unix())
	if err != nil {
		return false, fmt.Errorf("insert report: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, nil
	}
	reportID, err := res.LastInsertId()
	if err != nil {
		return false, err
	}

	recStmt, err := tx.PrepareContext(ctx,
		`INSERT INTO records(report_id,source_ip,count,disposition,dkim_eval,spf_eval,header_from,envelope_from,envelope_to,dmarc_pass)
		 VALUES(?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return false, err
	}
	defer func() { _ = recStmt.Close() }()
	authStmt, err := tx.PrepareContext(ctx,
		`INSERT INTO record_auth(record_id,kind,domain,selector,scope,result) VALUES(?,?,?,?,?,?)`)
	if err != nil {
		return false, err
	}
	defer func() { _ = authStmt.Close() }()
	reasonStmt, err := tx.PrepareContext(ctx, `INSERT INTO record_reasons(record_id,type,comment) VALUES(?,?,?)`)
	if err != nil {
		return false, err
	}
	defer func() { _ = reasonStmt.Close() }()

	for _, r := range f.Records {
		rr, err := recStmt.ExecContext(ctx, reportID, r.SourceIP, r.Count, r.Disposition, nullable(r.DKIM), nullable(r.SPF),
			r.HeaderFrom, nullable(r.EnvelopeFrom), nullable(r.EnvelopeTo), boolInt(r.DMARCPass()))
		if err != nil {
			return false, fmt.Errorf("insert record: %w", err)
		}
		recID, err := rr.LastInsertId()
		if err != nil {
			return false, err
		}
		for _, d := range r.DKIMResults {
			if _, err := authStmt.ExecContext(ctx, recID, "dkim", d.Domain, nullable(d.Selector), nil, d.Result); err != nil {
				return false, err
			}
		}
		for _, sp := range r.SPFResults {
			if _, err := authStmt.ExecContext(ctx, recID, "spf", sp.Domain, nil, nullable(sp.Scope), sp.Result); err != nil {
				return false, err
			}
		}
		for _, rs := range r.Reasons {
			if _, err := reasonStmt.ExecContext(ctx, recID, rs.Type, nullable(rs.Comment)); err != nil {
				return false, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// -------------------------------------------------------------- PTR cache

// PTR returns a cached reverse-DNS name (found=false if not cached or older than maxAge).
func (s *Store) PTR(ctx context.Context, ip string, maxAge time.Duration) (string, bool, error) {
	var ptr sql.NullString
	var at int64
	err := s.db.QueryRowContext(ctx, `SELECT ptr, resolved_at FROM ptr_cache WHERE ip=?`, ip).Scan(&ptr, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if s.now().Sub(time.Unix(at, 0)) > maxAge {
		return "", false, nil
	}
	return ptr.String, true, nil
}

// SetPTR caches a reverse-DNS result ("" for no PTR).
func (s *Store) SetPTR(ctx context.Context, ip, ptr string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO ptr_cache(ip,ptr,resolved_at) VALUES(?,?,?) ON CONFLICT(ip) DO UPDATE SET ptr=excluded.ptr, resolved_at=excluded.resolved_at`,
		ip, ptr, s.now().Unix())
	return err
}

// ----------------------------------------------------------------- helpers

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
