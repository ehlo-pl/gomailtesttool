-- testmailflow schema v1. Timestamps are Unix seconds (UTC).

CREATE TABLE IF NOT EXISTS probes (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    flow          TEXT    NOT NULL,
    token         TEXT    NOT NULL UNIQUE,
    message_id    TEXT    NOT NULL,
    created_at    INTEGER NOT NULL,
    sent_at       INTEGER,            -- set when the server accepted the message
    send_error    TEXT,
    send_response TEXT,
    send_host     TEXT,
    send_tls      TEXT,
    state         TEXT    NOT NULL,   -- pending, delivered, late, lost, send_failed
    attempts      INTEGER NOT NULL DEFAULT 0,
    next_check_at INTEGER,
    arrived_at    INTEGER,
    latency_ms    INTEGER,
    hops          INTEGER,
    auth_results  TEXT,
    check_error   TEXT,
    finished_at   INTEGER
);
CREATE INDEX IF NOT EXISTS probes_flow_created ON probes(flow, created_at);
CREATE INDEX IF NOT EXISTS probes_state ON probes(state, next_check_at);

-- Message keys already examined on a check endpoint (never downloaded twice).
CREATE TABLE IF NOT EXISTS seen_keys (
    endpoint TEXT    NOT NULL,
    msg_key  TEXT    NOT NULL,
    seen_at  INTEGER NOT NULL,
    PRIMARY KEY (endpoint, msg_key)
);

-- State transitions reported to the event log.
CREATE TABLE IF NOT EXISTS events (
    id     INTEGER PRIMARY KEY AUTOINCREMENT,
    ts     INTEGER NOT NULL,
    flow   TEXT,
    kind   TEXT    NOT NULL,
    detail TEXT
);
CREATE INDEX IF NOT EXISTS events_ts ON events(ts);
