# DMARC analyzer (`dmarcanalyzer`)

`dmarcanalyzer` reuses gomailtesttool's mail access (Microsoft Graph, Gmail API, IMAP, POP3) to:

1. **collect** messages from a DMARC report mailbox and save them as `*.eml`,
2. **extract** the DMARC aggregate (rua) reports (zip / gzip / xml attachments) into a local SQLite database,
3. **report** summaries over periods (day / week / month / quarter / year / total) as text, Markdown, CSV, JSON or a self-contained HTML page.

> Original requirement: *a tool which, using any form of gomailtesttool (Graph API, Google, POP3 or IMAP), collects mails from a specific mailbox (to \*.eml), extracts DMARC reports and summarizes them over periods.*

```
mailbox ──collect──▶ <datadir>/eml/<protocol>/<YYYY-MM>/*.eml ──extract──▶ SQLite ──report──▶ text / md / csv / json / html
```

Forensic (ruf / ARF) reports are detected and counted as `forensic_skipped`; they are not analyzed.

## Build

```bash
make build-dmarc                 # bin/dmarcanalyzer(.exe)
./build-all.ps1 -Dmarc           # Windows / PowerShell
go build -o bin/dmarcanalyzer ./apps/DMARCanalyzer/cmd/dmarcanalyzer
```

Pure Go (SQLite via `modernc.org/sqlite`, no CGO). Release ZIPs include the binary.

## Quick start

```bash
# 1. Collect the last 30 days from an IMAP folder and extract reports
dmarcanalyzer collect imap --host imap.example.com --port 993 --imaps \
  --username dmarc@example.com --password "app-password" --folder DMARC --since 30d

# 2. Summarize by week for the last quarter
dmarcanalyzer report --period week --last 90d

# 3. HTML report with reverse DNS of sending IPs
dmarcanalyzer report --period month --format html --out dmarc.html --resolve
```

Already have `.eml` files (e.g. from `gomailtest <protocol> exportmessages`) or raw report files?

```bash
dmarcanalyzer extract --input ./exported --input ./reports   # .eml, .xml, .xml.gz, .gz, .zip
```

## Commands

| Command | Purpose |
|---|---|
| `collect msgraph\|gmail\|imap\|pop3` | Download new messages as .eml, extract reports (default), optionally delete server copies |
| `extract [--input …] [--reprocess]` | Extract reports from files/directories (default `<datadir>/eml`) |
| `report [flags]` | Aggregate stored records and render a summary |
| `run msgraph\|gmail\|imap\|pop3` | `collect` + `report` in one step (scheduled tasks / cron) |
| `db stats` / `db vacuum` | Database statistics / compaction |

### Global flags

| Flag | Env | Default |
|---|---|---|
| `--config` | `DMARCCONFIG` | – YAML with `dmarc:`, `msgraph:`, `gmail:`, `imap:`, `pop3:` sections |
| `--datadir` | `DMARCDATADIR` | Windows `%LOCALAPPDATA%\dmarcanalyzer`, macOS `~/Library/Application Support/dmarcanalyzer`, Linux `$XDG_DATA_HOME/dmarcanalyzer` or `~/.local/share/dmarcanalyzer` |
| `--db` | `DMARCDB` | `<datadir>/dmarc.db` |
| `--verbose`, `--loglevel` | `DMARCVERBOSE`, `DMARCLOGLEVEL` | `INFO` (logs go to stderr) |

Precedence: CLI flag > environment variable > config file > default.

### `collect` / `run`

All authentication and connection flags are those of `gomailtest <protocol>` and use the same environment variables (`MSGRAPHTENANTID`, `GMAILCREDENTIALS`, `IMAPHOST`, `POP3USERNAME`, …). Additional flags:

| Flag | Meaning |
|---|---|
| `--folder` | IMAP mailbox (default `INBOX`), Graph folder id or well-known name (default `inbox`), Gmail label. POP3 ignores it. |
| `--since` | Received on/after: `YYYY-MM-DD`, RFC 3339 or relative (`30d`, `4w`). Default: previous successful run − `--overlap`; first run: 30 days. |
| `--until` | Received before (exclusive). |
| `--overlap` | Re-scan window before the previous run (default `2d`) to catch late delivery. Already-downloaded messages are never fetched again. |
| `--subject` | Case-insensitive subject substring filter (e.g. `Report domain`). |
| `--query` | Gmail only: raw Gmail search added to the filter (e.g. `has:attachment`). |
| `--max` | Download at most N messages. |
| `--extract` | Extract right after download (default `true`). |
| `--delete-after --yes` | Delete the server copy **only for messages whose aggregate report was stored** (status ok or duplicate). Graph: moved to Deleted Items; Gmail: moved to Trash; IMAP: `\Deleted` + expunge (UIDPLUS-safe); POP3: `DELE`, committed on `QUIT`. |

Protocol notes:

| Protocol | How messages are found | Message key (incremental state) | Permissions |
|---|---|---|---|
| msgraph | `/users/{mailbox}/mailFolders/{folder}/messages`, `$filter` on `receivedDateTime`, paged via `@odata.nextLink`; MIME via `$value` | Graph message id | `Mail.Read` (application); `Mail.ReadWrite` only with `--delete-after` |
| gmail | `users.messages.list` with `label:`/`after:`/`before:`/`subject:` query, paged via `pageToken`; `format=raw` | Gmail message id | `gmail.readonly`; `gmail.modify` only with `--delete-after` |
| imap | `UID SEARCH SINCE/BEFORE` (whole days) + optional `SUBJECT`; `BODY[]` | `<UIDVALIDITY>:<UID>` (deletion refused if UIDVALIDITY changed) | mailbox login / XOAUTH2 |
| pop3 | `UIDL`, then `TOP 0` for date/subject filters, `RETR` | UIDL (server must support UIDL) | mailbox login / XOAUTH2 |

Files are written with owner-only permissions (0600, directories 0700) as `<datadir>/eml/<protocol>/<YYYY-MM>/<key-prefix>-<hash>.eml`.

### `extract`

* Walks inputs recursively; accepts `.eml`, `.xml`, `.xml.gz`, `.gz`, `.zip`.
* MIME parts recognized by type (`application/zip`, `application/gzip`, `application/xml`, `text/xml`, …) or by file name, and by magic bytes (also single-part messages whose body is the archive).
* Safety limits (in memory, nothing unpacked to disk): 25 MB compressed, 100 MB decompressed per report, max 10 entries per zip, zip entries with paths rejected.
* XML parsed per RFC 7489 Appendix C, namespace-tolerant (DMARCbis `urn:ietf:params:xml:ns:dmarc-2.0`), UTF-8 / ISO-8859-1 / Windows-1252.
* Each file's SHA-256 is remembered; unchanged files are skipped unless `--reprocess`.
* A report is identified by (reporter `org_name`, `report_id`, policy domain) and stored once, even if it arrives twice.

### `report`

| Flag | Meaning |
|---|---|
| `--from`, `--to`, `--last` | Range on the report's `date_range` begin (`--to` exclusive). |
| `--period` | `day`, `week` (ISO 8601), `month` (default), `quarter`, `year`, `total`. |
| `--tz` | IANA time zone for period boundaries (default `UTC`). |
| `--domain` | Only these domains (policy domain or header From incl. subdomains), repeatable. |
| `--org` | Only these reporters (e.g. `google.com`, `Outlook.com`), repeatable. |
| `--top` | Rows in source / failure lists (default 20). |
| `--failing-only` | Source list ranked by failures, only sources that fail. |
| `--resolve` | PTR lookup for listed IPs (10 parallel, 3 s timeout, cached 7 days). |
| `--format` | `text` (default), `md`, `csv`, `json`, `html`. |
| `--section` | CSV only: `periods` (default), `domains`, `reporters`, `sources`, `failing`, `reasons`, `policy_changes` or `all` (needs `--out <dir>`). |
| `--out` | Output file (0600) or directory for `--section all`. |

Report contents:

* **Overall**: messages, DMARC pass/fail and %, DKIM-aligned and SPF-aligned pass %, dispositions (none / quarantine / reject), reports, reporters, distinct source IPs.
* **By period** with Δ pass % against the previous period.
* **By header From domain** with the latest published policy (`p= sp= pct=`).
* **By reporter**, **top sending sources** (IP, PTR, header From domains).
* **DMARC failures (actionable)**: grouped by source IP, header From, disposition and raw DKIM/SPF results, with a hint (alignment issue, forwarding, unauthorized sender…).
* **Override reasons** (forwarded, mailing_list, local_policy, …) and **published policy changes**.

Untrusted report content is escaped in every format (HTML auto-escaping, Markdown escaping, terminal control characters stripped, CSV formula injection neutralized). The HTML page is self-contained (inline CSS and SVG, no external scripts) and supports light/dark mode.

## Configuration file example

```yaml
# dmarc.yaml — dmarcanalyzer run msgraph --config dmarc.yaml
dmarc:
  datadir: D:\dmarc
  period: week
  last: 12w
  format: html
  out: D:\dmarc\weekly.html
msgraph:
  tenantid: 00000000-0000-0000-0000-000000000000
  clientid: 00000000-0000-0000-0000-000000000000
  thumbprint: 0123456789ABCDEF0123456789ABCDEF01234567
  mailbox: dmarc@example.com
```

More: [docs/config-examples/dmarc-msgraph.yaml](../../docs/config-examples/dmarc-msgraph.yaml), [docs/config-examples/dmarc-imap.yaml](../../docs/config-examples/dmarc-imap.yaml), [docs/env-examples/dmarc-gmail.env](../../docs/env-examples/dmarc-gmail.env).

## Scheduling

Windows Task Scheduler (daily):

```powershell
dmarcanalyzer.exe run msgraph --config C:\dmarc\dmarc.yaml
```

cron:

```bash
15 6 * * * /usr/local/bin/dmarcanalyzer run imap --config /etc/dmarc.yaml >> /var/log/dmarc.log 2>&1
```

`run` writes collection progress to stderr when the report goes to stdout.

## Database schema (SQLite)

| Table | Content |
|---|---|
| `sources` | protocol, mailbox, folder, `last_collect_at` |
| `fetched_messages` | per-source message keys, received time, .eml path, SHA-256, `deleted_at` |
| `processed_files` | path, SHA-256, status (`ok`, `duplicate`, `no_report`, `forensic_skipped`, `error`), error text |
| `reports` | reporter, report id, date range, published policy (adkim, aspf, p, sp, np, pct, fo), warnings |
| `records` | source IP, count, disposition, aligned DKIM/SPF result, header/envelope From/To, `dmarc_pass` |
| `record_auth` | raw DKIM (domain, selector, result) and SPF (domain, scope, result) results |
| `record_reasons` | policy override reasons |
| `ptr_cache` | reverse DNS cache |

Timestamps are Unix seconds (UTC). The file can be queried directly, e.g. with `sqlite3 dmarc.db`.

## Code layout

```
apps/DMARCanalyzer/
  cmd/dmarcanalyzer/      main
  internal/cli/           cobra commands (collect, extract, report, run, db)
  internal/config/        global config (DMARC* env vars, data dir)
  internal/dmarc/         RFC 7489 model + parser
  internal/extract/       MIME walk, zip/gzip unpacking with limits
  internal/pipeline/      extract → parse → store
  internal/store/         SQLite schema and queries
  internal/report/        period bucketing, aggregation, renderers
  internal/testutil/      synthetic reports and messages for tests
internal/common/fetch/    protocol-neutral fetch.Session API
internal/protocols/*/fetchraw.go   Graph, Gmail, IMAP, POP3 implementations
```
