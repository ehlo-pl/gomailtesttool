# POP3 Protocol — gomailtest

POP3 server connectivity, TLS configuration, authentication, and message listing.

> **Legacy name:** `pop3tool`. Use `gomailtest pop3 <action> --flag` (see the migration table in README.md).

## Quick Start

```powershell
# Test basic connectivity
gomailtest pop3 testconnect --host pop.example.com --port 995 --pop3s

# Test authentication
gomailtest pop3 testauth --host pop.example.com --port 995 --pop3s \
    --username user@example.com --password "yourpassword"

# List messages
gomailtest pop3 listmail --host pop.example.com --port 995 --pop3s \
    --username user@example.com --password "yourpassword"
```

## Actions

### testconnect — Basic Connectivity

Tests TCP connection, reads server greeting, and retrieves POP3 capabilities via CAPA.

```powershell
# POP3S (port 995, implicit TLS)
gomailtest pop3 testconnect --host pop.gmail.com --port 995 --pop3s

# POP3 with STLS (port 110, explicit TLS)
gomailtest pop3 testconnect --host pop.example.com --port 110 --starttls
```

### testauth — Authentication Testing

Connects, establishes TLS, and authenticates. Supports USER/PASS, APOP, XOAUTH2.

```powershell
# Password authentication (USER/PASS)
gomailtest pop3 testauth --host pop.example.com --port 995 --pop3s \
    --username user@example.com --password "yourpassword"

# OAuth2 token
gomailtest pop3 testauth --host pop.gmail.com --port 995 --pop3s \
    --username user@gmail.com --accesstoken "ya29.xxx..."

# Specify auth method
gomailtest pop3 testauth --host pop.example.com --port 995 --pop3s \
    --username user@example.com --password "secret" --authmethod USER
```

### listmail — List Messages

Authenticates and lists messages using STAT, LIST, and UIDL commands.

```powershell
# List messages (default max 100)
gomailtest pop3 listmail --host pop.example.com --port 995 --pop3s \
    --username user@example.com --password "yourpassword"

# Limit to 50 messages
gomailtest pop3 listmail --host pop.example.com --port 995 --pop3s \
    --username user@example.com --password "yourpassword" --maxmessages 50
```

### exportmessages — Export Matching Messages as .eml

Authenticates, fetches headers for each message via `TOP`, matches against the
given Message-ID and/or Subject, and exports each match's full message (via
`RETR`) as a `.eml` file.

```powershell
gomailtest pop3 exportmessages --host pop.example.com --port 995 --pop3s \
    --username user@example.com --password "yourpassword" \
    --subject "Invoice"

gomailtest pop3 exportmessages --host pop.example.com --port 995 --pop3s \
    --username user@example.com --password "yourpassword" \
    --messageid "<message-id@example.com>" --count 10

# Custom export directory
gomailtest pop3 exportmessages --host pop.example.com --port 995 --pop3s \
    --username user@example.com --password "yourpassword" \
    --subject "Invoice" --exportdir "C:\exports"
```

Output goes to `%TEMP%\export\{date}\msg_{message-id}.eml`, or
`<exportdir>\{date}\msg_{message-id}.eml` when `--exportdir` is given.

## Flags

### Persistent (all subcommands)

| Flag | Description | Environment Variable | Default |
|------|-------------|---------------------|---------|
| `--host` | POP3 server hostname (required) — the service to connect to; also used for TLS SNI/certificate checks and auth | `POP3HOST` | — |
| `--port` | POP3 server port | `POP3PORT` | 110 |
| `--timeout` | Connection timeout (seconds) | `POP3TIMEOUT` | 30 |
| `--username` | Username for authentication | `POP3USERNAME` | — |
| `--password` | Password for authentication | `POP3PASSWORD` | — |
| `--accesstoken` | OAuth2 access token for XOAUTH2 | `POP3ACCESSTOKEN` | — |
| `--authmethod` | Auth method: auto, USER, APOP, XOAUTH2 | `POP3AUTHMETHOD` | auto |
| `--pop3s` | Use POP3S (implicit TLS on port 995) | `POP3POP3S` | false |
| `--starttls` | Force STLS upgrade | `POP3STARTTLS` | false |
| `--no-pop3s` | Force plain connection: errors if `--pop3s` is also set | `POP3NOPOP3S` | false |
| `--no-starttls` | Force plain connection: errors if `--starttls` is also set | `POP3NOSTARTTLS` | false |
| `--skipverify` | Skip TLS certificate verification | `POP3SKIPVERIFY` | false |
| `--tlsversion` | TLS version: 1.2, 1.3 | `POP3TLSVERSION` | 1.2 |
| `--address` | Optional: connect to this IP/host instead of --host (e.g. behind a load balancer); --host is still used for SNI/certificate checks and auth | `POP3ADDRESS` | — |
| `--ipv4` | Force IPv4: resolve --host/--address to an A record and connect over IPv4 | `POP3IPV4` | false |
| `--ipv6` | Force IPv6: resolve --host/--address to an AAAA record and connect over IPv6 | `POP3IPV6` | false |
| `--proxy` | HTTP/HTTPS proxy URL | `POP3PROXY` | — |
| `--maxretries` | Maximum retry attempts | `POP3MAXRETRIES` | 3 |
| `--retrydelay` | Retry delay (milliseconds) | `POP3RETRYDELAY` | 2000 |
| `--ratelimit` | Max requests per second (0 = unlimited) | `POP3RATELIMIT` | 0 |
| `--verbose` | Enable verbose output | `POP3VERBOSE` | false |
| `--loglevel` | Log level: DEBUG, INFO, WARN, ERROR | `POP3LOGLEVEL` | INFO |
| `--logformat` | Log file format: csv, json | `POP3LOGFORMAT` | csv |

**Note:** `--pop3s` and `--starttls` cannot be used together. `--no-pop3s`+`--pop3s` and `--no-starttls`+`--starttls` are each mutually exclusive (useful to catch conflicting defaults from `--config`/env vars).

### listmail-only flags

| Flag | Description | Environment Variable | Default |
|------|-------------|---------------------|---------|
| `--maxmessages` | Maximum messages to list | `POP3MAXMESSAGES` | 100 |

### exportmessages-specific

| Flag | Description | Environment Variable | Default |
|------|-------------|---------------------|---------|
| `--messageid` | Message-ID header value to search for | `POP3MESSAGEID` | — |
| `--subject` | Subject substring to search for | `POP3SUBJECT` | — |
| `--count` | Maximum number of matching messages to export | `POP3COUNT` | 25 |
| `--exportdir` | Directory under which to create the dated export folder | `POP3EXPORTDIR` | OS temp dir |

## Environment Variables

```powershell
$env:POP3HOST = "pop.example.com"
$env:POP3PORT = "995"
$env:POP3POP3S = "true"
$env:POP3USERNAME = "user@example.com"
$env:POP3PASSWORD = "yourpassword"

gomailtest pop3 testauth
```

## Common Ports

| Port | Usage | TLS |
|------|-------|-----|
| 110 | POP3 (standard) | Optional STLS |
| 995 | POP3S (implicit TLS) | Implicit TLS |

## POP3S vs STLS

| Method | Port | Flag | Description |
|--------|------|------|-------------|
| POP3S | 995 | `--pop3s` | Implicit TLS — encryption starts immediately |
| STLS | 110 | `--starttls` | Explicit TLS — plain connection upgrades after STLS command |

## CSV Logging

Operations are logged to `%TEMP%\_pop3tool_{action}_{date}.csv`.

## Related Documentation

- [BUILD.md](../../BUILD.md) — Build instructions
- [docs/protocols/imap.md](imap.md) — IMAP tool
- [docs/protocols/smtp.md](smtp.md) — SMTP tool
- [SECURITY.md](../../SECURITY.md) — Security policy

                          ..ooOO END OOoo..
