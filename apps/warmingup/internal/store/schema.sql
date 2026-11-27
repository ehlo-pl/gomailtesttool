CREATE TABLE IF NOT EXISTS domains (
    domain           TEXT PRIMARY KEY,
    ramp_day         INTEGER NOT NULL DEFAULT 0,
    last_advanced    TEXT    NOT NULL DEFAULT '',
    held_reason      TEXT    NOT NULL DEFAULT '',
    resumed_at       INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS targets (
    domain      TEXT NOT NULL,
    target      TEXT NOT NULL,
    ramp_day    INTEGER NOT NULL DEFAULT 0,
    held_reason TEXT    NOT NULL DEFAULT '',
    resumed_at  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (domain, target)
);

CREATE TABLE IF NOT EXISTS slots (
    id     INTEGER PRIMARY KEY,
    domain TEXT    NOT NULL,
    day    TEXT    NOT NULL,
    at     INTEGER NOT NULL,
    status TEXT    NOT NULL DEFAULT 'pending'
);
CREATE INDEX IF NOT EXISTS slots_due ON slots (domain, status, at);

CREATE TABLE IF NOT EXISTS messages (
    id            INTEGER PRIMARY KEY,
    domain        TEXT    NOT NULL,
    target        TEXT    NOT NULL,
    day           TEXT    NOT NULL,
    ramp_day      INTEGER NOT NULL,
    message_id    TEXT    NOT NULL UNIQUE,
    recipient     TEXT    NOT NULL,
    subject       TEXT    NOT NULL,
    content_key   TEXT    NOT NULL,
    created_at    INTEGER NOT NULL,
    sent_at       INTEGER NOT NULL DEFAULT 0,
    send_response TEXT    NOT NULL DEFAULT '',
    send_error    TEXT    NOT NULL DEFAULT '',
    state         TEXT    NOT NULL,
    attempts      INTEGER NOT NULL DEFAULT 0,
    next_check_at INTEGER NOT NULL DEFAULT 0,
    found_at      INTEGER NOT NULL DEFAULT 0,
    placement     TEXT    NOT NULL DEFAULT '',
    folder        TEXT    NOT NULL DEFAULT '',
    latency       INTEGER NOT NULL DEFAULT 0,
    auth          TEXT    NOT NULL DEFAULT '',
    spam_info     TEXT    NOT NULL DEFAULT '',
    headers       TEXT    NOT NULL DEFAULT '',
    engage_at     INTEGER NOT NULL DEFAULT 0,
    engaged_at    INTEGER NOT NULL DEFAULT 0,
    check_error   TEXT    NOT NULL DEFAULT '',
    finished_at   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS messages_target ON messages (domain, target, created_at);
CREATE INDEX IF NOT EXISTS messages_next ON messages (next_check_at);
CREATE INDEX IF NOT EXISTS messages_engage ON messages (engage_at);

CREATE TABLE IF NOT EXISTS bounces (
    id         INTEGER PRIMARY KEY,
    message_id TEXT    NOT NULL,
    at         INTEGER NOT NULL,
    action     TEXT    NOT NULL,
    status     TEXT    NOT NULL,
    remote_mta TEXT    NOT NULL,
    diagnostic TEXT    NOT NULL,
    UNIQUE (message_id, action, status)
);

CREATE TABLE IF NOT EXISTS seen_keys (
    endpoint TEXT    NOT NULL,
    key      TEXT    NOT NULL,
    at       INTEGER NOT NULL,
    PRIMARY KEY (endpoint, key)
);
