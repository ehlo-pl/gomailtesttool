# SMTP Protocol — gomailtest

SMTP connectivity, TLS diagnostics, authentication, and email sending.

> **Legacy name:** `smtptool`. The legacy binary was removed in v3.1. Use `gomailtest smtp <action> --flag` (see the migration table in README.md).

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

## Flags

### Persistent (all subcommands)

| Flag | Description | Environment Variable | Default |
|------|-------------|---------------------|---------|
| `--host` | SMTP server hostname or IP | `SMTPHOST` | — |
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
| `--skipverify` | Skip TLS certificate verification | `SMTPSKIPVERIFY` | false |
| `--tlsversion` | Minimum TLS version: 1.2, 1.3 | `SMTPTLSVERSION` | 1.2 |
| `--address` | Override connection address (uses --host for SNI) | `SMTPADDRESS` | — |
| `--proxy` | HTTP/HTTPS/SOCKS5 proxy URL | `SMTPPROXY` | — |
| `--ratelimit` | Max requests per second (0 = unlimited) | `SMTPRATELIMIT` | 0 |
| `--verbose` | Enable verbose output | `SMTPVERBOSE` | false |
| `--loglevel` | Log level: DEBUG, INFO, WARN, ERROR | `SMTPLOGLEVEL` | INFO |
| `--logformat` | Log file format: csv, json | `SMTPLOGFORMAT` | csv |

### sendmail-only flags

| Flag | Description | Environment Variable |
|------|-------------|---------------------|
| `--from` | Sender email address | `SMTPFROM` |
| `--to` | Comma-separated TO recipients | `SMTPTO` |
| `--subject` | Email subject | `SMTPSUBJECT` |
| `--body` | Email body text | `SMTPBODY` |

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
