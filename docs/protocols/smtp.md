# SMTP Protocol — gomailtest

SMTP connectivity, TLS diagnostics, authentication, and email sending.

> **Legacy name:** `smtptool`. Use `gomailtest smtp <action> --flag` (see the migration table in README.md).

## Quick Start

```powershell
# Test basic connectivity
gomailtest smtp testconnect --host smtp.example.com --port 25

# Comprehensive TLS diagnostics
gomailtest smtp teststarttls --host smtp.example.com --port 587

# Test authentication
gomailtest smtp testauth --host smtp.example.com --port 587 \
  --username user@example.com --password yourpassword

# Send test email
gomailtest smtp sendmail --host smtp.example.com --port 587 \
  --username user@example.com --password yourpassword \
  --from sender@example.com --to recipient@example.com \
  --subject "Test Email" --body "This is a test message"
```

## Actions

### testconnect — Basic Connectivity

Tests TCP connection and SMTP capabilities, including Exchange server detection.

```powershell
gomailtest smtp testconnect --host mail.contoso.com --port 25
```

### teststarttls — Comprehensive TLS Diagnostics

Performs in-depth TLS/SSL testing: certificate chain analysis (subject, issuer, SANs, expiry, key strength), cipher suite assessment, protocol version detection, and Exchange-targeted diagnostics.

```powershell
gomailtest smtp teststarttls --host smtp.office365.com --port 587

# Skip certificate verification (insecure, for testing only)
gomailtest smtp teststarttls --host smtp.example.com --port 587 --skipverify

# Specify TLS version
gomailtest smtp teststarttls --host smtp.example.com --port 587 --tlsversion 1.3
```

### testauth — Authentication Testing

Connects, negotiates STARTTLS, then authenticates. Auto-selects the strongest available method when `--authmethod auto` (default).

**Supported methods and auto-select priority:** `GSSAPI` → `CRAM-MD5` → `NTLM` → `PLAIN` → `LOGIN` (or `XOAUTH2` → `OAUTHBEARER` first when `--accesstoken` provided).

| Method | Use case | Credentials |
|--------|----------|-------------|
| `GSSAPI` | On-premises AD/Exchange (Kerberos 5) | `--username` (`user@REALM`), `--password`, optional `--realm`, `--kdc` |
| `CRAM-MD5` | Secure challenge-response | `--username`, `--password` |
| `NTLM` | On-premises Exchange / Windows SMTP | `--username` (`DOMAIN\user`), `--password` |
| `PLAIN` | Standard username/password over TLS | `--username`, `--password` |
| `LOGIN` | Legacy username/password (two-step) | `--username`, `--password` |
| `XOAUTH2` | Microsoft 365, Google Workspace | `--username`, `--accesstoken` |
| `OAUTHBEARER` | RFC 7628 SASL OAuth (Dovecot, modern servers); OAuth 2.0/2.1 tokens | `--username`, `--accesstoken` |

```powershell
# Auto-detect (picks strongest advertised method)
gomailtest smtp testauth --host smtp.example.com --port 587 \
  --username user@example.com --password "yourpassword"

# CRAM-MD5 explicit
gomailtest smtp testauth --host smtp.example.com --port 587 \
  --username user@example.com --password "secret" --authmethod CRAM-MD5

# NTLM (on-premises Exchange / Windows SMTP relay)
# Username may be in DOMAIN\user or plain user@domain.com format
gomailtest smtp testauth --host exchange.contoso.com --port 25 \
  --username "CONTOSO\user" --password "secret" --authmethod NTLM

# GSSAPI/Kerberos (on-premises Exchange / Active Directory)
# Realm is auto-extracted from user@REALM format
gomailtest smtp testauth --host exchange.contoso.com --port 25 \
  --username "alice@CONTOSO.COM" --password "secret" --authmethod GSSAPI

# GSSAPI with explicit KDC (useful when DNS SRV records are not configured)
gomailtest smtp testauth --host exchange.contoso.com --port 25 \
  --username "alice@CONTOSO.COM" --password "secret" --authmethod GSSAPI \
  --kdc dc01.contoso.com

# XOAUTH2 (Microsoft 365 / Google Workspace)
gomailtest smtp testauth --host smtp.office365.com --port 587 \
  --username user@company.com --accesstoken "eyJ..."

# OAUTHBEARER (RFC 7628 — Dovecot and other modern servers)
# The same token works whether obtained via an OAuth 2.0 or OAuth 2.1 flow.
gomailtest smtp testauth --host smtp.example.com --port 587 \
  --username user@example.com --accesstoken "eyJ..." --authmethod OAUTHBEARER
```

### sendmail — End-to-End Email Sending

Full SMTP pipeline: connect, STARTTLS, authenticate, send RFC 5322 message.

```powershell
gomailtest smtp sendmail \
  --host smtp.example.com --port 587 \
  --username user@example.com --password "yourpassword" \
  --from sender@example.com \
  --to "recipient1@example.com,recipient2@example.com" \
  --subject "Test Email" --body "This is a test message"
```

#### HTML body, attachments, inline images, and custom headers

```powershell
gomailtest smtp sendmail \
  --host smtp.example.com --port 587 \
  --username user@example.com --password "yourpassword" \
  --from sender@example.com --to recipient@example.com \
  --subject "Test Email" \
  --body "Plain text fallback" \
  --bodyhtml '<p>Hello! Here is our logo: <img src="cid:logo.png"></p>' \
  --inline-attachments ./logo.png \
  --attachments ./report.pdf,./data.csv \
  --header "X-Custom-Header: example-value"
```

- `--bodyhtml` alone sends an HTML-only message; combined with `--body` it sends a `multipart/alternative` message with both a plain-text and HTML version.
- `--inline-attachments` embeds files as `multipart/related` parts referenced from `--bodyhtml` via `cid:<filename>` (e.g. `cid:logo.png` for `./logo.png`).
- `--attachments` adds regular file attachments (MIME type detected from file extension).
- `--header` adds a custom header in `"Name: Value"` form; repeat the flag for multiple headers. Standard headers (`From`, `To`, `Subject`, `Date`, `Message-ID`, `MIME-Version`, `Content-Type`, etc.) cannot be overridden this way.
- `--cc` recipients receive the message via `RCPT TO` and appear in the `Cc:` header (visible to all recipients). `--bcc` recipients receive the message via `RCPT TO` but are never written to any message header.
- `--priority high` or `--priority low` add the `X-Priority`, `Importance`, and `Priority` headers recognized by most mail clients; `--priority normal` (the default) adds none of these headers.

#### Sending from a template (`--template` / `--template-vars`)

`--template` sends from a file processed through Go `text/template`; variables
supplied with repeatable `--template-vars key=value` flags are referenced as
`{{.key}}` (referencing an unsupplied variable is an error). The file
extension picks the mode:

- **`.eml` (full message mode):** the rendered file is a complete RFC 822
  message and is injected verbatim into the SMTP `DATA` phase — every header
  and MIME part is preserved exactly. The envelope (`MAIL FROM`/`RCPT TO`)
  uses `--from`/`--to`/`--cc`/`--bcc` when given, otherwise falls back to the
  rendered `From`/`To`/`Cc`/`Bcc` headers. Because the file *is* the message,
  `--body`, `--bodyhtml`, `--attachments`, `--inline-attachments`, `--header`,
  and a non-default `--priority` are rejected — embed those in the file.
- **any other extension (HTML body mode):** the rendered content becomes the
  HTML body (same as `--bodyhtml`); all other sendmail flags work as usual.

```powershell
# Full message from a .eml template
gomailtest smtp sendmail --host smtp.example.com --port 587 \
  --template .\welcome.eml --template-vars Name=World --template-vars Rcpt=user@example.com

# HTML body from a template file
gomailtest smtp sendmail --host smtp.example.com --port 587 \
  --from sender@example.com --to user@example.com \
  --template .\body.html --template-vars Name=World
```

Where `welcome.eml` might contain:

```
From: Sender <sender@example.com>
To: {{.Rcpt}}
Subject: Welcome, {{.Name}}!
Content-Type: text/plain; charset=utf-8

Hello {{.Name}}, this message came from an EML template.
```

## Flags

### Persistent (all subcommands)

| Flag | Description | Environment Variable | Default |
|------|-------------|---------------------|---------|
| `--host` | SMTP server hostname (required) — the service to connect to; also used for TLS SNI/certificate checks and auth | `SMTPHOST` | — |
| `--port` | SMTP server port | `SMTPPORT` | 25 |
| `--timeout` | Connection timeout (seconds) | `SMTPTIMEOUT` | 30 |
| `--username` | SMTP username (`DOMAIN\user` for NTLM, `user@REALM` for GSSAPI) | `SMTPUSERNAME` | — |
| `--password` | SMTP password | `SMTPPASSWORD` | — |
| `--accesstoken` | OAuth2 bearer token for XOAUTH2 or OAUTHBEARER | `SMTPACCESSTOKEN` | — |
| `--authmethod` | Auth method: PLAIN, LOGIN, CRAM-MD5, NTLM, GSSAPI, XOAUTH2, OAUTHBEARER, auto | `SMTPAUTHMETHOD` | auto |
| `--realm` | Kerberos realm for GSSAPI (auto-extracted from `user@REALM` if omitted) | `SMTPREALM` | — |
| `--kdc` | KDC address for GSSAPI (uses DNS SRV if omitted) | `SMTPKDC` | — |
| `--starttls` | Force STARTTLS usage | `SMTPSTARTTLS` | false |
| `--smtps` | Use implicit TLS (port 465) | `SMTPSMTPS` | false |
| `--no-starttls` | Force plain connection: disable STARTTLS (including the automatic upgrade `sendmail`/`testauth` perform when the server advertises it); errors if `--starttls` is also set | `SMTPNOSTARTTLS` | false |
| `--no-smtps` | Force plain connection: errors if `--smtps` is also set | `SMTPNOSMTPS` | false |
| `--skipverify` | Skip TLS certificate verification | `SMTPSKIPVERIFY` | false |
| `--tlsversion` | Minimum TLS version: 1.2, 1.3 | `SMTPTLSVERSION` | 1.2 |
| `--address` | Optional: connect to this IP/host instead of --host (e.g. behind a load balancer); --host is still used for SNI/certificate checks and auth | `SMTPADDRESS` | — |
| `--ipv4` | Force IPv4: resolve --host/--address to an A record and connect over IPv4 | `SMTPIPV4` | false |
| `--ipv6` | Force IPv6: resolve --host/--address to an AAAA record and connect over IPv6 | `SMTPIPV6` | false |
| `--use-mx` | Treat --host as a domain name and connect to its MX record instead; mutually exclusive with --address. The resolved MX hostname is also used for TLS SNI/certificate checks. For `sendmail`, also mutually exclusive with --host — the MX lookup domain is instead derived from the first `--to` recipient | `SMTPUSEMX` | false |
| `--proxy` | HTTP/HTTPS/SOCKS5 proxy URL | `SMTPPROXY` | — |
| `--ratelimit` | Max requests per second (0 = unlimited) | `SMTPRATELIMIT` | 0 |
| `--verbose` | Enable verbose output | `SMTPVERBOSE` | false |
| `--loglevel` | Log level: DEBUG, INFO, WARN, ERROR | `SMTPLOGLEVEL` | INFO |
| `--logformat` | Log file format: csv, json | `SMTPLOGFORMAT` | csv |

**Note:** `--smtps`+`--no-smtps` and `--starttls`+`--no-starttls` are each
mutually exclusive (useful to catch conflicting defaults from `--config`/env
vars). `teststarttls` requires either STARTTLS or `--smtps` to test, so
`--no-starttls` without `--smtps` is rejected for that action.

### sendmail-only flags

| Flag | Description | Environment Variable |
|------|-------------|---------------------|
| `--from` | Sender email address | `SMTPFROM` |
| `--to` | Comma-separated TO recipients | `SMTPTO` |
| `--cc` | Comma-separated CC recipients; included in the `Cc:` header and the SMTP envelope | `SMTPCC` |
| `--bcc` | Comma-separated BCC recipients; included in the SMTP envelope only, never in message headers | `SMTPBCC` |
| `--subject` | Email subject | `SMTPSUBJECT` |
| `--body` | Email body text | `SMTPBODY` |
| `--bodyhtml` | HTML body content; combine with `--body` for `multipart/alternative` | `SMTPBODYHTML` |
| `--attachments` | Comma-separated file paths to attach | `SMTPATTACHMENTS` |
| `--inline-attachments` | Comma-separated file paths to embed inline via `cid:<filename>` | `SMTPINLINEATTACHMENTS` |
| `--header` | Custom header in `"Name: Value"` form (repeatable); when set via env var use comma-separated values (avoid commas in header values) | `SMTPHEADER` |
| `--priority` | Email priority: `high`, `normal`, `low`. `high`/`low` add `X-Priority`, `Importance`, and `Priority` headers; `normal` (default) adds no extra headers | `SMTPPRIORITY` |
| `--template` | Message template file with Go `text/template` variables: a `.eml` file is sent as the complete RFC 822 message (mutually exclusive with `--body`/`--bodyhtml` and, for `.eml`, the attachment/header/priority flags); any other extension is used as the HTML body | `SMTPTEMPLATE` |
| `--template-vars` | Template variable in `key=value` form, referenced as `{{.key}}` in `--template` (repeatable) | `SMTPTEMPLATEVARS` |

## Environment Variables

Flags can be set via environment variables with the `SMTP` prefix:

```powershell
$env:SMTPHOST = "smtp.example.com"
$env:SMTPPORT = "587"
$env:SMTPUSERNAME = "user@example.com"
$env:SMTPPASSWORD = "yourpassword"

gomailtest smtp testauth
```

## Common Ports

| Port | Usage | TLS |
|------|-------|-----|
| 25 | SMTP relay (server-to-server) | Optional STARTTLS |
| 587 | Submission (client-to-server) | STARTTLS required |
| 465 | SMTPS (implicit TLS) | Implicit TLS |

## SMTPS vs STARTTLS

| Method | Port | Flag | Description |
|--------|------|------|-------------|
| SMTPS | 465 | `--smtps` | Implicit TLS — encryption starts immediately |
| STARTTLS | 587/25 | `--starttls` | Explicit TLS — plain connection upgrades after STARTTLS command |

## CSV Logging

Operations are logged to `%TEMP%\_smtptool_{action}_{date}.csv`.

## Testing with Mailpit

For local SMTP testing use [Mailpit](https://github.com/axllent/mailpit):

```bash
docker run -d --name mailpit -p 1025:1025 -p 8025:8025 axllent/mailpit
```

```powershell
gomailtest smtp testconnect --host localhost --port 1025
gomailtest smtp testauth --host localhost --port 1025 --username test --password test
gomailtest smtp sendmail --host localhost --port 1025 \
  --from sender@test.local --to recipient@test.local \
  --subject "Test" --body "Hello from gomailtest"
```

View captured emails at [http://localhost:8025](http://localhost:8025).

## Related Documentation

- [BUILD.md](../../BUILD.md) — Build instructions
- [SECURITY.md](../../SECURITY.md) — Security policy
- [TROUBLESHOOTING.md](../../TROUBLESHOOTING.md) — Common issues

                          ..ooOO END OOoo..
