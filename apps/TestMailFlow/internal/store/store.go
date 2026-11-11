// Package store persists probes, examined message keys and state-change
// events in a SQLite database (pure Go driver, no CGO).
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
)

//go:embed schema.sql
var schemaV1 string

const schemaVersion = 1

// Probe states.
const (
	StatePending    = "pending"
	StateDelivered  = "delivered"
	StateLate       = "late"
	StateLost       = "lost"
	StateSendFailed = "send_failed"
)

// Flow health derived from the latest finished probe.
const (
	HealthUnknown  = "unknown"
	HealthOK       = "ok"
	HealthDegraded = "degraded"
	HealthFailing  = "failing"
)

// HealthOf maps a finished probe state to a flow health.
func HealthOf(state string) string {
	switch state {
	case StateDelivered:
		return HealthOK
	case StateLate:
		return HealthDegraded
	case StateLost, StateSendFailed:
		return HealthFailing
	}
	return HealthUnknown
}

// Probe is one probe message and its outcome.
type Probe struct {
	ID           int64
	Flow         string
	Token        string
	MessageID    string
	CreatedAt    time.Time
	SentAt       time.Time // zero until accepted
	SendError    string
	SendResponse string
	SendHost     string
	SendTLS      string
	State        string
	Attempts     int
	NextCheckAt  time.Time
	ArrivedAt    time.Time
	Latency      time.Duration
	Hops         int
	AuthResults  string
	CheckError   string
	FinishedAt   time.Time
}

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the database at path. Use ":memory:" for tests.
func Open(ctx context.Context, path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			return nil, fmt.Errorf("create database file: %w", err)
		}
		_ = f.Close()
	}
	dsn := path + "?_pragma=busy_timeout(5000)"
	if path != ":memory:" {
		// WAL lets "status"/"report" read while the service writes.
		dsn += "&_pragma=journal_mode(WAL)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	var v int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if v > schemaVersion {
		return fmt.Errorf("database schema version %d is newer than supported %d; upgrade testmailflow", v, schemaVersion)
	}
	if v < 1 {
		if _, err := s.db.ExecContext(ctx, schemaV1); err != nil {
			return fmt.Errorf("apply schema v1: %w", err)
		}
	}
	_, err := s.db.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", schemaVersion))
	return err
}

// ------------------------------------------------------------------ probes

// InsertProbe stores a new pending probe before it is sent and sets p.ID.
func (s *Store) InsertProbe(ctx context.Context, p *Probe) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO probes(flow, token, message_id, created_at, state) VALUES(?,?,?,?,?)`,
		p.Flow, p.Token, p.MessageID, p.CreatedAt.Unix(), StatePending)
	if err != nil {
		return fmt.Errorf("insert probe: %w", err)
	}
	p.State = StatePending
	p.ID, err = res.LastInsertId()
	return err
}

// MarkSent records an accepted submission and schedules the first check.
func (s *Store) MarkSent(ctx context.Context, id int64, sentAt, nextCheck time.Time, response, host, tls string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE probes SET sent_at=?, next_check_at=?, send_response=?, send_host=?, send_tls=? WHERE id=?`,
		sentAt.Unix(), nextCheck.Unix(), nullable(response), nullable(host), nullable(tls), id)
	return err
}

// MarkSendFailed finishes a probe that could not be submitted.
func (s *Store) MarkSendFailed(ctx context.Context, id int64, at time.Time, sendErr string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE probes SET state=?, send_error=?, finished_at=?, next_check_at=NULL WHERE id=?`,
		StateSendFailed, sendErr, at.Unix(), id)
	return err
}

// MarkArrived finishes a probe that was found in the checked mailbox.
func (s *Store) MarkArrived(ctx context.Context, id int64, state string, arrivedAt, finishedAt time.Time,
	latency time.Duration, hops int, authResults string, attempts int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE probes SET state=?, arrived_at=?, latency_ms=?, hops=?, auth_results=?, attempts=?,
		 finished_at=?, next_check_at=NULL, check_error=NULL WHERE id=?`,
		state, arrivedAt.Unix(), latency.Milliseconds(), hops, nullable(authResults), attempts, finishedAt.Unix(), id)
	return err
}

// MarkNotFound records an unsuccessful check attempt and schedules the next one.
func (s *Store) MarkNotFound(ctx context.Context, id int64, attempts int, nextCheck time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE probes SET attempts=?, next_check_at=?, check_error=NULL WHERE id=?`, attempts, nextCheck.Unix(), id)
	return err
}

// MarkLost finishes a probe that did not arrive within its schedule.
func (s *Store) MarkLost(ctx context.Context, id int64, attempts int, at time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE probes SET state=?, attempts=?, finished_at=?, next_check_at=NULL, check_error=NULL WHERE id=?`,
		StateLost, attempts, at.Unix(), id)
	return err
}

// MarkCheckError records that the mailbox could not be checked; the attempt
// is not consumed and is retried at nextCheck.
func (s *Store) MarkCheckError(ctx context.Context, id int64, checkErr string, nextCheck time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE probes SET check_error=?, next_check_at=? WHERE id=?`, checkErr, nextCheck.Unix(), id)
	return err
}

const probeCols = `id, flow, token, message_id, created_at, sent_at, send_error, send_response, send_host, send_tls,
	state, attempts, next_check_at, arrived_at, latency_ms, hops, auth_results, check_error, finished_at`

type scanner interface{ Scan(dest ...any) error }

func scanProbe(row scanner) (*Probe, error) {
	var (
		p                                              Probe
		created                                        int64
		sent, next, arrived, latency, hops, finished   sql.NullInt64
		sendErr, sendResp, host, tls, auth, checkError sql.NullString
	)
	if err := row.Scan(&p.ID, &p.Flow, &p.Token, &p.MessageID, &created, &sent, &sendErr, &sendResp, &host, &tls,
		&p.State, &p.Attempts, &next, &arrived, &latency, &hops, &auth, &checkError, &finished); err != nil {
		return nil, err
	}
	p.CreatedAt = unix(sql.NullInt64{Int64: created, Valid: true})
	p.SentAt, p.NextCheckAt, p.ArrivedAt, p.FinishedAt = unix(sent), unix(next), unix(arrived), unix(finished)
	p.SendError, p.SendResponse, p.SendHost, p.SendTLS = sendErr.String, sendResp.String, host.String, tls.String
	p.AuthResults, p.CheckError = auth.String, checkError.String
	p.Latency = time.Duration(latency.Int64) * time.Millisecond
	p.Hops = int(hops.Int64)
	return &p, nil
}

func (s *Store) queryProbes(ctx context.Context, query string, args ...any) ([]*Probe, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*Probe
	for rows.Next() {
		p, err := scanProbe(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PendingProbes returns sent probes still waiting for arrival, oldest first.
func (s *Store) PendingProbes(ctx context.Context) ([]*Probe, error) {
	return s.queryProbes(ctx, `SELECT `+probeCols+` FROM probes WHERE state=? AND sent_at IS NOT NULL ORDER BY id`, StatePending)
}

// UnsentProbes returns pending probes that were never submitted (the
// process stopped between insert and send).
func (s *Store) UnsentProbes(ctx context.Context) ([]*Probe, error) {
	return s.queryProbes(ctx, `SELECT `+probeCols+` FROM probes WHERE state=? AND sent_at IS NULL ORDER BY id`, StatePending)
}

// ProbeByID returns one probe, or nil.
func (s *Store) ProbeByID(ctx context.Context, id int64) (*Probe, error) {
	p, err := scanProbe(s.db.QueryRowContext(ctx, `SELECT `+probeCols+` FROM probes WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return p, err
}

// ProbeByToken returns the probe with the token, or nil.
func (s *Store) ProbeByToken(ctx context.Context, token string) (*Probe, error) {
	p, err := scanProbe(s.db.QueryRowContext(ctx, `SELECT `+probeCols+` FROM probes WHERE token=?`, token))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return p, err
}

// LastProbe returns the newest probe of a flow, or nil. With finishedOnly
// pending probes are ignored.
func (s *Store) LastProbe(ctx context.Context, flow string, finishedOnly bool) (*Probe, error) {
	q := `SELECT ` + probeCols + ` FROM probes WHERE flow=?`
	if finishedOnly {
		q += ` AND state<>'` + StatePending + `'`
	}
	p, err := scanProbe(s.db.QueryRowContext(ctx, q+` ORDER BY id DESC LIMIT 1`, flow))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return p, err
}

// LastSuccess returns when a probe of the flow last arrived (zero if never).
func (s *Store) LastSuccess(ctx context.Context, flow string) (time.Time, error) {
	var at sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT MAX(arrived_at) FROM probes WHERE flow=?`, flow).Scan(&at)
	return unix(at), err
}

// ProbesBetween returns probes created in [from, to), optionally for one
// flow, oldest first. Zero bounds are open.
func (s *Store) ProbesBetween(ctx context.Context, from, to time.Time, flow string) ([]*Probe, error) {
	var where []string
	var args []any
	if !from.IsZero() {
		where, args = append(where, "created_at>=?"), append(args, from.Unix())
	}
	if !to.IsZero() {
		where, args = append(where, "created_at<?"), append(args, to.Unix())
	}
	if flow != "" {
		where, args = append(where, "flow=?"), append(args, flow)
	}
	q := `SELECT ` + probeCols + ` FROM probes`
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, " AND ")
	}
	return s.queryProbes(ctx, q+` ORDER BY id`, args...)
}

// StateCount is the number of probes of a flow in a state.
type StateCount struct {
	Flow, State string
	Count       int
}

// CountsByState returns probe totals per flow and state.
func (s *Store) CountsByState(ctx context.Context) ([]StateCount, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT flow, state, COUNT(*) FROM probes GROUP BY flow, state ORDER BY flow, state`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []StateCount
	for rows.Next() {
		var c StateCount
		if err := rows.Scan(&c.Flow, &c.State, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- seen keys

// IsSeen reports whether a message key was already examined on an endpoint.
func (s *Store) IsSeen(ctx context.Context, endpoint, key string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM seen_keys WHERE endpoint=? AND msg_key=?`, endpoint, key).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// MarkSeen remembers an examined message key.
func (s *Store) MarkSeen(ctx context.Context, endpoint, key string, at time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO seen_keys(endpoint, msg_key, seen_at) VALUES(?,?,?) ON CONFLICT DO NOTHING`, endpoint, key, at.Unix())
	return err
}

// ------------------------------------------------------------------- events

// Event is one recorded state transition.
type Event struct {
	At     time.Time
	Flow   string
	Kind   string
	Detail string
}

// AddEvent records a state transition.
func (s *Store) AddEvent(ctx context.Context, e Event) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO events(ts, flow, kind, detail) VALUES(?,?,?,?)`,
		e.At.Unix(), nullable(e.Flow), e.Kind, nullable(e.Detail))
	return err
}

// RecentEvents returns the newest events first.
func (s *Store) RecentEvents(ctx context.Context, limit int) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT ts, flow, kind, detail FROM events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Event
	for rows.Next() {
		var ts int64
		var flow, detail sql.NullString
		var e Event
		if err := rows.Scan(&ts, &flow, &e.Kind, &detail); err != nil {
			return nil, err
		}
		e.At, e.Flow, e.Detail = time.Unix(ts, 0).UTC(), flow.String, detail.String
		out = append(out, e)
	}
	return out, rows.Err()
}

// -------------------------------------------------------------- maintenance

// Prune deletes finished probes, seen keys and events older than before and
// returns the number of probes removed.
func (s *Store) Prune(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM probes WHERE state<>? AND created_at<?`, StatePending, before.Unix())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if _, err := s.db.ExecContext(ctx, `DELETE FROM seen_keys WHERE seen_at<?`, before.Unix()); err != nil {
		return n, err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM events WHERE ts<?`, before.Unix())
	return n, err
}

// Vacuum compacts the database file.
func (s *Store) Vacuum(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "VACUUM")
	return err
}

// TableCounts returns row counts of the tables.
func (s *Store) TableCounts(ctx context.Context) (map[string]int64, error) {
	out := map[string]int64{}
	for _, t := range []string{"probes", "seen_keys", "events"} {
		var n int64
		if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+t).Scan(&n); err != nil {
			return nil, err
		}
		out[t] = n
	}
	return out, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func unix(n sql.NullInt64) time.Time {
	if !n.Valid {
		return time.Time{}
	}
	return time.Unix(n.Int64, 0).UTC()
}
