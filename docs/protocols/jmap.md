# JMAP Protocol — gomailtest

JMAP (JSON Meta Application Protocol) server connectivity, authentication, and mailbox listing.

> **Legacy name:** `jmaptool`. The legacy binary was removed in v3.1. Use `gomailtest jmap <action> --flag` (see the migration table in README.md).

## What is JMAP?

JMAP is a modern, open standard for email access (RFC 8620 / RFC 8621) that replaces IMAP's text-based protocol with JSON over HTTPS. Key benefits: fewer round-trips, mobile-friendly, standardized push notifications.

**JMAP Providers:** Fastmail, Cyrus IMAP, Apache James.

## Quick Start

```powershell
# Test JMAP server connectivity
gomailtest jmap testconnect --host jmap.fastmail.com

# Test authentication with access token
gomailtest jmap testauth --host jmap.fastmail.com \
    --username user@example.com --accesstoken "your-api-token"

# List mailboxes
gomailtest jmap getmailboxes --host jmap.fastmail.com \
    --username user@example.com --accesstoken "your-api-token"
```

## Actions

### testconnect — Server Connectivity

Connects to the JMAP server via HTTPS and discovers the JMAP session from `/.well-known/jmap`.

```powershell
gomailtest jmap testconnect --host jmap.fastmail.com

# With verbose output
gomailtest jmap testconnect --host jmap.fastmail.com --verbose
```

### testauth — Authentication Testing

Authenticates using Bearer token or Basic auth and displays session information.

```powershell
# Bearer token (recommended for Fastmail and most providers)
gomailtest jmap testauth --host jmap.fastmail.com \
    --username user@example.com --accesstoken "fmu1-xxxxxxxx-xxxxxxxxxxxx"

# Basic authentication
gomailtest jmap testauth --host jmap.example.com \
    --username user@example.com --password "yourpassword" --authmethod basic

# Auto-detect (Bearer if accesstoken provided, otherwise Basic)
gomailtest jmap testauth --host jmap.fastmail.com \
    --username user@example.com --accesstoken "token" --authmethod auto
```

### getmailboxes — List Mailboxes

Authenticates and retrieves mailboxes using the `Mailbox/get` JMAP method.

```powershell
gomailtest jmap getmailboxes --host jmap.fastmail.com \
    --username user@example.com --accesstoken "your-api-token"
```

## Flags

| Flag | Description | Environment Variable | Default |
|------|-------------|---------------------|---------|
| `--host` | JMAP server hostname (required) — the service to connect to; also used for TLS SNI/certificate checks and auth | `JMAPHOST` | — |
| `--port` | JMAP server port | `JMAPPORT` | 443 |
| `--address` | Optional: connect to this IP/host instead of --host (e.g. behind a load balancer); --host is still used for SNI/certificate checks and auth | `JMAPADDRESS` | — |
| `--ipv4` | Force IPv4: resolve --host/--address to an A record and connect over IPv4 | `JMAPIPV4` | false |
| `--ipv6` | Force IPv6: resolve --host/--address to an AAAA record and connect over IPv6 | `JMAPIPV6` | false |
| `--username` | Username for authentication | `JMAPUSERNAME` | — |
| `--password` | Password for Basic authentication | `JMAPPASSWORD` | — |
| `--accesstoken` | Access token for Bearer authentication | `JMAPACCESSTOKEN` | — |
| `--authmethod` | Auth method: auto, basic, bearer | `JMAPAUTHMETHOD` | auto |
| `--skipverify` | Skip TLS certificate verification | `JMAPSKIPVERIFY` | false |
| `--verbose` | Enable verbose output | `JMAPVERBOSE` | false |
| `--loglevel` | Log level: debug, info, warn, error | `JMAPLOGLEVEL` | info |
| `--logformat` | Log file format: csv, json | `JMAPLOGFORMAT` | csv |

## Environment Variables

```powershell
$env:JMAPHOST = "jmap.fastmail.com"
$env:JMAPUSERNAME = "user@example.com"
$env:JMAPACCESSTOKEN = "your-api-token"

gomailtest jmap getmailboxes
```

## Authentication Methods

Authentication method is auto-detected from provided credentials:
- **Bearer** — used when `--accesstoken` is provided (recommended for Fastmail)
- **Basic** — used when `--password` is provided

**Getting a Fastmail API Token:**
1. Log in to Fastmail → Settings → Password & Security → API Tokens
2. Create a token with required permissions
3. Use it via `--accesstoken` or `$env:JMAPACCESSTOKEN`

## Provider Examples

```powershell
# Fastmail
gomailtest jmap getmailboxes --host jmap.fastmail.com \
    --username user@fastmail.com --accesstoken "fmu1-xxxxxxxx-xxxxxxxxxxxx"

# Self-hosted (with self-signed cert — testing only)
gomailtest jmap testauth --host jmap.yourdomain.com \
    --username user@yourdomain.com --password "password" \
    --authmethod basic --skipverify
```

## CSV Logging

Operations are logged to `%TEMP%\_jmaptool_{action}_{date}.csv`.

## JMAP Resources

- RFC 8620 — JMAP Core: https://tools.ietf.org/html/rfc8620
- RFC 8621 — JMAP for Mail: https://tools.ietf.org/html/rfc8621
- JMAP website: https://jmap.io

## Related Documentation

- [BUILD.md](../../BUILD.md) — Build instructions
- [docs/protocols/imap.md](imap.md) — IMAP tool
- [SECURITY.md](../../SECURITY.md) — Security policy

                          ..ooOO END OOoo..
