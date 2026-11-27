// Package store persists ramp state, send slots, messages, bounce reports and
// examined bounce keys in SQLite (pure Go driver, no CGO).
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

//go:embed schema.sql
var schema string

// Message states.
const (
	StatePending    = "pending"
	StateDelivered  = "delivered"
	StateLate       = "late"
	StateLost       = "lost"
	StateSendFailed = "send_failed"
)

// Slot states.
const (
	SlotPending = "pending"
	SlotSent    = "sent"
	SlotMissed  = "missed"  // not sent in time (downtime, no content)
	SlotSkipped = "skipped" // every target already had its share
	SlotFailed  = "failed"
)

// Ramp is the persisted ramp state of a domain or of one of its targets.
type Ramp struct {
	Day          int
	LastAdvanced string // "2006-01-02"; domain only
	HeldReason   string // "" = not held
	ResumedAt    time.Time
}

// Slot is one planned send time.
type Slot struct {
	ID     int64
	Domain string
	Day    string
	At     time.Time
	Status string
}

// Message is one warm-up message and its outcome.
type Message struct {
	ID           int64
	Domain       string
	Target       string
	Day          string
	RampDay      int
	MessageID    string
	Recipient    string
	Subject      string
	ContentKey   string
	CreatedAt    time.Time
	SentAt       time.Time
	SendResponse string
	SendError    string
	State        string
	Attempts     int
	NextCheckAt  time.Time // zero: no further check
	FoundAt      time.Time
	Placement    string // first observed: "inbox" or "junk"
	Folder       string
	Latency      time.Duration
	Auth         string
	SpamInfo     string
	Headers      string
	EngageAt     time.Time // zero: nothing to do
	EngagedAt    time.Time
	CheckError   string
	FinishedAt   time.Time
}

// Bounce is one delivery status report about a message.
type Bounce struct {
	MessageID  string
	At         time.Time
	Action     string // failed, delayed
	Status     string // enhanced status code
	RemoteMTA  string
	Diagnostic string
}

// Store wraps the SQLite database.
type Store struct{ db *sql.DB }

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
		dsn += "&_pragma=journal_mode(WAL)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

func unix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

func fromUnix(v int64) time.Time {
	if v == 0 {
		return time.Time{}
	}
	return time.Unix(v, 0).UTC()
}

func (s *Store) exec(ctx context.Context, query string, args ...any) error {
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("database: %w", err)
	}
	return nil
}

// ------------------------------------------------------------------- ramp

// DomainRamp returns the domain state (zero value when unknown).
func (s *Store) DomainRamp(ctx context.Context, domain string) (Ramp, error) {
	var r Ramp
	var resumed int64
	err := s.db.QueryRowContext(ctx, `SELECT ramp_day, last_advanced, held_reason, resumed_at FROM domains WHERE domain=?`, domain).
		Scan(&r.Day, &r.LastAdvanced, &r.HeldReason, &resumed)
	if errors.Is(err, sql.ErrNoRows) {
		return Ramp{}, nil
	}
	r.ResumedAt = fromUnix(resumed)
	return r, err
}

// SaveDomainRamp stores the domain state.
func (s *Store) SaveDomainRamp(ctx context.Context, domain string, r Ramp) error {
	return s.exec(ctx, `INSERT INTO domains(domain, ramp_day, last_advanced, held_reason, resumed_at) VALUES(?,?,?,?,?)
		ON CONFLICT(domain) DO UPDATE SET ramp_day=excluded.ramp_day, last_advanced=excluded.last_advanced,
		held_reason=excluded.held_reason, resumed_at=excluded.resumed_at`,
		domain, r.Day, r.LastAdvanced, r.HeldReason, unix(r.ResumedAt))
}

// TargetRamp returns the target state (zero value when unknown).
func (s *Store) TargetRamp(ctx context.Context, domain, target string) (Ramp, error) {
	var r Ramp
	var resumed int64
	err := s.db.QueryRowContext(ctx, `SELECT ramp_day, held_reason, resumed_at FROM targets WHERE domain=? AND target=?`, domain, target).
		Scan(&r.Day, &r.HeldReason, &resumed)
	if errors.Is(err, sql.ErrNoRows) {
		return Ramp{}, nil
	}
	r.ResumedAt = fromUnix(resumed)
	return r, err
}

// SaveTargetRamp stores the target state.
func (s *Store) SaveTargetRamp(ctx context.Context, domain, target string, r Ramp) error {
	return s.exec(ctx, `INSERT INTO targets(domain, target, ramp_day, held_reason, resumed_at) VALUES(?,?,?,?,?)
		ON CONFLICT(domain, target) DO UPDATE SET ramp_day=excluded.ramp_day, held_reason=excluded.held_reason,
		resumed_at=excluded.resumed_at`, domain, target, r.Day, r.HeldReason, unix(r.ResumedAt))
}

// ------------------------------------------------------------------ slots

// AddSlots stores the planned send times of a day together with the domain
// state, so a crash cannot advance the ramp without a plan.
func (s *Store) AddSlots(ctx context.Context, domain, day string, times []time.Time, r Ramp) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, at := range times {
		if _, err := tx.ExecContext(ctx, `INSERT INTO slots(domain, day, at) VALUES(?,?,?)`, domain, day, at.Unix()); err != nil {
			return fmt.Errorf("add slot: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO domains(domain, ramp_day, last_advanced, held_reason, resumed_at) VALUES(?,?,?,?,?)
		ON CONFLICT(domain) DO UPDATE SET ramp_day=excluded.ramp_day, last_advanced=excluded.last_advanced`,
		domain, r.Day, r.LastAdvanced, r.HeldReason, unix(r.ResumedAt)); err != nil {
		return fmt.Errorf("save ramp: %w", err)
	}
	return tx.Commit()
}

// DueSlots returns the pending slots of a domain whose time has come.
func (s *Store) DueSlots(ctx context.Context, domain string, now time.Time) ([]Slot, error) {
	return s.slots(ctx, `WHERE domain=? AND status='pending' AND at<=? ORDER BY at`, domain, now.Unix())
}

// DaySlots returns all slots of one day.
func (s *Store) DaySlots(ctx context.Context, domain, day string) ([]Slot, error) {
	return s.slots(ctx, `WHERE domain=? AND day=? ORDER BY at`, domain, day)
}

func (s *Store) slots(ctx context.Context, where string, args ...any) ([]Slot, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, domain, day, at, status FROM slots `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("query slots: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Slot
	for rows.Next() {
		var sl Slot
		var at int64
		if err := rows.Scan(&sl.ID, &sl.Domain, &sl.Day, &at, &sl.Status); err != nil {
			return nil, err
		}
		sl.At = fromUnix(at)
		out = append(out, sl)
	}
	return out, rows.Err()
}

// SetSlot changes the status of a slot.
func (s *Store) SetSlot(ctx context.Context, id int64, status string) error {
	return s.exec(ctx, `UPDATE slots SET status=? WHERE id=?`, status, id)
}

// --------------------------------------------------------------- messages

const columns = `id, domain, target, day, ramp_day, message_id, recipient, subject, content_key, created_at, sent_at,
	send_response, send_error, state, attempts, next_check_at, found_at, placement, folder, latency, auth, spam_info,
	headers, engage_at, engaged_at, check_error, finished_at`

// Insert stores a message before it is submitted and sets m.ID.
func (s *Store) Insert(ctx context.Context, m *Message) error {
	res, err := s.db.ExecContext(ctx, `INSERT INTO messages(domain, target, day, ramp_day, message_id, recipient, subject,
		content_key, created_at, state) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		m.Domain, m.Target, m.Day, m.RampDay, m.MessageID, m.Recipient, m.Subject, m.ContentKey, m.CreatedAt.Unix(), StatePending)
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}
	m.State = StatePending
	m.ID, err = res.LastInsertId()
	return err
}

// Update writes every mutable field of m.
func (s *Store) Update(ctx context.Context, m *Message) error {
	return s.exec(ctx, `UPDATE messages SET sent_at=?, send_response=?, send_error=?, state=?, attempts=?, next_check_at=?,
		found_at=?, placement=?, folder=?, latency=?, auth=?, spam_info=?, headers=?, engage_at=?, engaged_at=?,
		check_error=?, finished_at=? WHERE id=?`,
		unix(m.SentAt), m.SendResponse, m.SendError, m.State, m.Attempts, unix(m.NextCheckAt), unix(m.FoundAt),
		m.Placement, m.Folder, int64(m.Latency/time.Second), m.Auth, m.SpamInfo, m.Headers, unix(m.EngageAt),
		unix(m.EngagedAt), m.CheckError, unix(m.FinishedAt), m.ID)
}

func (s *Store) messages(ctx context.Context, where string, args ...any) ([]*Message, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+columns+` FROM messages `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*Message
	for rows.Next() {
		m := &Message{}
		var created, sent, next, found, latency, engage, engaged, finished int64
		if err := rows.Scan(&m.ID, &m.Domain, &m.Target, &m.Day, &m.RampDay, &m.MessageID, &m.Recipient, &m.Subject,
			&m.ContentKey, &created, &sent, &m.SendResponse, &m.SendError, &m.State, &m.Attempts, &next, &found,
			&m.Placement, &m.Folder, &latency, &m.Auth, &m.SpamInfo, &m.Headers, &engage, &engaged, &m.CheckError,
			&finished); err != nil {
			return nil, err
		}
		m.CreatedAt, m.SentAt, m.NextCheckAt, m.FoundAt = fromUnix(created), fromUnix(sent), fromUnix(next), fromUnix(found)
		m.Latency = time.Duration(latency) * time.Second
		m.EngageAt, m.EngagedAt, m.FinishedAt = fromUnix(engage), fromUnix(engaged), fromUnix(finished)
		out = append(out, m)
	}
	return out, rows.Err()
}

// Get returns one message by row id, or nil.
func (s *Store) Get(ctx context.Context, id int64) (*Message, error) {
	return first(s.messages(ctx, `WHERE id=?`, id))
}

// ByMessageID returns the message with this Message-ID, or nil.
func (s *Store) ByMessageID(ctx context.Context, messageID string) (*Message, error) {
	return first(s.messages(ctx, `WHERE message_id=?`, messageID))
}

func first(list []*Message, err error) (*Message, error) {
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return list[0], nil
}

// DueChecks returns sent messages whose next mailbox check has come.
func (s *Store) DueChecks(ctx context.Context, now time.Time) ([]*Message, error) {
	return s.messages(ctx, `WHERE next_check_at>0 AND next_check_at<=? ORDER BY target, sent_at`, now.Unix())
}

// DueEngagements returns found messages to be marked read and rescued.
func (s *Store) DueEngagements(ctx context.Context, now time.Time) ([]*Message, error) {
	return s.messages(ctx, `WHERE engage_at>0 AND engaged_at=0 AND engage_at<=? ORDER BY target, engage_at`, now.Unix())
}

// Unsent returns messages stored but never submitted (crash in between).
func (s *Store) Unsent(ctx context.Context) ([]*Message, error) {
	return s.messages(ctx, `WHERE state='pending' AND sent_at=0`)
}

// Since returns the messages created at or after since, oldest first. An
// empty domain means all domains.
func (s *Store) Since(ctx context.Context, domain string, since time.Time) ([]*Message, error) {
	return s.messages(ctx, `WHERE (?='' OR domain=?) AND created_at>=? ORDER BY created_at, id`, domain, domain, unix(since))
}

// Resolved returns the newest n messages of a target that have an outcome
// (delivered, late or lost) and were created after since.
func (s *Store) Resolved(ctx context.Context, domain, target string, since time.Time, n int) ([]*Message, error) {
	return s.messages(ctx, `WHERE domain=? AND target=? AND state IN ('delivered','late','lost') AND created_at>?
		ORDER BY created_at DESC, id DESC LIMIT ?`, domain, target, unix(since), n)
}

// LastSent returns the newest n submitted-or-failed messages of a domain
// (target "" = all targets), newest first.
func (s *Store) LastSent(ctx context.Context, domain, target string, since time.Time, n int) ([]*Message, error) {
	return s.messages(ctx, `WHERE domain=? AND (?='' OR target=?) AND created_at>? ORDER BY created_at DESC, id DESC LIMIT ?`,
		domain, target, target, unix(since), n)
}

// UsedContent returns the content keys already sent, by any domain.
func (s *Store) UsedContent(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT content_key FROM messages WHERE state!='send_failed'`)
	if err != nil {
		return nil, fmt.Errorf("query content: %w", err)
	}
	defer func() { _ = rows.Close() }()
	used := map[string]bool{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		used[k] = true
	}
	return used, rows.Err()
}

// DayCounts returns how many messages each target of a domain got on day,
// and the total ever sent per target.
func (s *Store) DayCounts(ctx context.Context, domain, day string) (today, total map[string]int, err error) {
	rows, err := s.db.QueryContext(ctx, `SELECT target, SUM(day=?), COUNT(*) FROM messages WHERE domain=? GROUP BY target`, day, domain)
	if err != nil {
		return nil, nil, fmt.Errorf("count messages: %w", err)
	}
	defer func() { _ = rows.Close() }()
	today, total = map[string]int{}, map[string]int{}
	for rows.Next() {
		var target string
		var d, t int
		if err := rows.Scan(&target, &d, &t); err != nil {
			return nil, nil, err
		}
		today[target], total[target] = d, t
	}
	return today, total, rows.Err()
}

// ------------------------------------------------------------ bounces etc.

// AddBounce stores a report; a repeated one is ignored.
func (s *Store) AddBounce(ctx context.Context, b Bounce) error {
	return s.exec(ctx, `INSERT OR IGNORE INTO bounces(message_id, at, action, status, remote_mta, diagnostic) VALUES(?,?,?,?,?,?)`,
		b.MessageID, b.At.Unix(), b.Action, b.Status, b.RemoteMTA, b.Diagnostic)
}

// Bounces returns the reports about one message.
func (s *Store) Bounces(ctx context.Context, messageID string) ([]Bounce, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT message_id, at, action, status, remote_mta, diagnostic FROM bounces
		WHERE message_id=? ORDER BY at, id`, messageID)
	if err != nil {
		return nil, fmt.Errorf("query bounces: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Bounce
	for rows.Next() {
		var b Bounce
		var at int64
		if err := rows.Scan(&b.MessageID, &at, &b.Action, &b.Status, &b.RemoteMTA, &b.Diagnostic); err != nil {
			return nil, err
		}
		b.At = fromUnix(at)
		out = append(out, b)
	}
	return out, rows.Err()
}

// MarkSeen records an examined bounce mailbox message.
func (s *Store) MarkSeen(ctx context.Context, endpoint, key string, at time.Time) error {
	return s.exec(ctx, `INSERT OR IGNORE INTO seen_keys(endpoint, key, at) VALUES(?,?,?)`, endpoint, key, at.Unix())
}

// IsSeen reports whether a bounce mailbox message was examined before.
func (s *Store) IsSeen(ctx context.Context, endpoint, key string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM seen_keys WHERE endpoint=? AND key=?`, endpoint, key).Scan(&n)
	return n > 0, err
}
