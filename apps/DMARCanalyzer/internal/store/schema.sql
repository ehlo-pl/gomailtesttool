-- DMARC analyzer schema, version 1. Timestamps are Unix seconds (UTC).

CREATE TABLE IF NOT EXISTS sources (
    id              INTEGER PRIMARY KEY,
    protocol        TEXT NOT NULL,
    mailbox         TEXT NOT NULL,
    folder          TEXT NOT NULL,
    last_collect_at INTEGER,
    UNIQUE (protocol, mailbox, folder)
);

CREATE TABLE IF NOT EXISTS fetched_messages (
    source_id   INTEGER NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    msg_key     TEXT NOT NULL,
    received_at INTEGER,
    eml_path    TEXT NOT NULL,
    sha256      TEXT NOT NULL,
    fetched_at  INTEGER NOT NULL,
    deleted_at  INTEGER,
    PRIMARY KEY (source_id, msg_key)
);

CREATE TABLE IF NOT EXISTS processed_files (
    path          TEXT PRIMARY KEY,
    sha256        TEXT NOT NULL,
    status        TEXT NOT NULL,      -- ok | duplicate | no_report | forensic_skipped | error
    reports_found INTEGER NOT NULL DEFAULT 0,
    error         TEXT,
    processed_at  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS reports (
    id            INTEGER PRIMARY KEY,
    org_name      TEXT NOT NULL,
    org_email     TEXT,
    report_id     TEXT NOT NULL,
    begin_ts      INTEGER NOT NULL,
    end_ts        INTEGER NOT NULL,
    policy_domain TEXT NOT NULL,
    adkim         TEXT,
    aspf          TEXT,
    p             TEXT,
    sp            TEXT,
    np            TEXT,
    pct           INTEGER,
    fo            TEXT,
    warnings      TEXT,
    xml_sha256    TEXT NOT NULL,
    source_file   TEXT,
    imported_at   INTEGER NOT NULL,
    UNIQUE (org_name, report_id, policy_domain)
);
CREATE INDEX IF NOT EXISTS idx_reports_begin ON reports(begin_ts);
CREATE INDEX IF NOT EXISTS idx_reports_domain ON reports(policy_domain);

CREATE TABLE IF NOT EXISTS records (
    id            INTEGER PRIMARY KEY,
    report_id     INTEGER NOT NULL REFERENCES reports(id) ON DELETE CASCADE,
    source_ip     TEXT NOT NULL,
    count         INTEGER NOT NULL,
    disposition   TEXT NOT NULL,
    dkim_eval     TEXT,
    spf_eval      TEXT,
    header_from   TEXT NOT NULL,
    envelope_from TEXT,
    envelope_to   TEXT,
    dmarc_pass    INTEGER NOT NULL   -- derived: dkim_eval='pass' OR spf_eval='pass'
);
CREATE INDEX IF NOT EXISTS idx_records_report ON records(report_id);
CREATE INDEX IF NOT EXISTS idx_records_header_from ON records(header_from);
CREATE INDEX IF NOT EXISTS idx_records_source_ip ON records(source_ip);

CREATE TABLE IF NOT EXISTS record_auth (
    record_id INTEGER NOT NULL REFERENCES records(id) ON DELETE CASCADE,
    kind      TEXT NOT NULL,          -- dkim | spf
    domain    TEXT,
    selector  TEXT,
    scope     TEXT,
    result    TEXT
);
CREATE INDEX IF NOT EXISTS idx_record_auth_record ON record_auth(record_id);

CREATE TABLE IF NOT EXISTS record_reasons (
    record_id INTEGER NOT NULL REFERENCES records(id) ON DELETE CASCADE,
    type      TEXT NOT NULL,
    comment   TEXT
);
CREATE INDEX IF NOT EXISTS idx_record_reasons_record ON record_reasons(record_id);

CREATE TABLE IF NOT EXISTS ptr_cache (
    ip          TEXT PRIMARY KEY,
    ptr         TEXT,
    resolved_at INTEGER NOT NULL
);
