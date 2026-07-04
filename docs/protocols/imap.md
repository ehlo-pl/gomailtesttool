# IMAP Protocol — gomailtest

IMAP server connectivity, TLS configuration, authentication, and folder listing.

> **Legacy name:** `imaptool`. Use `gomailtest imap <action> --flag` (see the migration table in README.md).

## Quick Start

```powershell
# Test basic connectivity
gomailtest imap testconnect --host imap.example.com --port 993 --imaps

# Test authentication
gomailtest imap testauth --host imap.example.com --port 993 --imaps \
    --username user@example.com --password "yourpassword"

# List mailbox folders
gomailtest imap listfolders --host imap.example.com --port 993 --imaps \
    --username user@example.com --password "yourpassword"
```

## Actions

### testconnect — Basic Connectivity

Tests TCP connection, reads server greeting, sends CAPABILITY command, and displays supported capabilities.

```powershell
# IMAPS (port 993)
gomailtest imap testconnect --host imap.gmail.com --port 993 --imaps

# STARTTLS (port 143)
gomailtest imap testconnect --host imap.example.com --port 143 --starttls
```

### testauth — Authentication Testing

Connects, establishes TLS, and authenticates. Supports PLAIN, LOGIN, XOAUTH2.

```powershell
# Password authentication
gomailtest imap testauth --host imap.example.com --port 993 --imaps \
    --username user@example.com --password "yourpassword"

# OAuth2 token
gomailtest imap testauth --host imap.gmail.com --port 993 --imaps \
    --username user@gmail.com --accesstoken "ya29.xxx..."

# Specify auth method
gomailtest imap testauth --host imap.example.com --port 993 --imaps \
    --username user@example.com --password "secret" --authmethod PLAIN
```

### listfolders — List Mailbox Folders

Authenticates and lists all folders using the LIST command.

```powershell
gomailtest imap listfolders --host imap.example.com --port 993 --imaps \
    --username user@example.com --password "yourpassword"
```

### exportmessages — Export Matching Messages as .eml

Authenticates, selects a mailbox, SEARCHes for messages by Message-ID and/or
a Subject substring, and exports each match's full raw RFC822 message as a
`.eml` file.

```powershell
gomailtest imap exportmessages --host imap.example.com --port 993 --imaps \
    --username user@example.com --password "yourpassword" \
    --subject "Invoice"

gomailtest imap exportmessages --host imap.example.com --port 993 --imaps \
    --username user@example.com --password "yourpassword" \
    --messageid "<message-id@example.com>" --mailbox "INBOX" --count 10

# Custom export directory
gomailtest imap exportmessages --host imap.example.com --port 993 --imaps \
    --username user@example.com --password "yourpassword" \
    --subject "Invoice" --exportdir "C:\exports"
```

Output goes to `%TEMP%\export\{date}\msg_{uid}.eml`, or
`<exportdir>\{date}\msg_{uid}.eml` when `--exportdir` is given.

## Flags

| Flag | Description | Environment Variable | Default |
|------|-------------|---------------------|---------|
| `--host` | IMAP server hostname (required) — the service to connect to; also used for TLS SNI/certificate checks and auth | `IMAPHOST` | — |
| `--port` | IMAP server port | `IMAPPORT` | 143 |
| `--timeout` | Connection timeout (seconds) | `IMAPTIMEOUT` | 30 |
| `--username` | Username for authentication | `IMAPUSERNAME` | — |
| `--password` | Password for authentication | `IMAPPASSWORD` | — |
| `--accesstoken` | OAuth2 access token for XOAUTH2 | `IMAPACCESSTOKEN` | — |
| `--authmethod` | Auth method: auto, PLAIN, LOGIN, XOAUTH2 | `IMAPAUTHMETHOD` | auto |
| `--imaps` | Use IMAPS (implicit TLS on port 993) | `IMAPIMAPS` | false |
| `--starttls` | Force STARTTLS upgrade | `IMAPSTARTTLS` | false |
| `--no-imaps` | Force plain connection: errors if `--imaps` is also set | `IMAPNOIMAPS` | false |
| `--no-starttls` | Force plain connection: errors if `--starttls` is also set | `IMAPNOSTARTTLS` | false |
| `--skipverify` | Skip TLS certificate verification | `IMAPSKIPVERIFY` | false |
| `--tlsversion` | TLS version: 1.2, 1.3 | `IMAPTLSVERSION` | 1.2 |
| `--address` | Optional: connect to this IP/host instead of --host (e.g. behind a load balancer); --host is still used for SNI/certificate checks and auth | `IMAPADDRESS` | — |
| `--ipv4` | Force IPv4: resolve --host/--address to an A record and connect over IPv4 | `IMAPIPV4` | false |
| `--ipv6` | Force IPv6: resolve --host/--address to an AAAA record and connect over IPv6 | `IMAPIPV6` | false |
| `--proxy` | HTTP/HTTPS proxy URL | `IMAPPROXY` | — |
| `--maxretries` | Maximum retry attempts | `IMAPMAXRETRIES` | 3 |
| `--retrydelay` | Retry delay (milliseconds) | `IMAPRETRYDELAY` | 2000 |
| `--ratelimit` | Max requests per second (0 = unlimited) | `IMAPRATELIMIT` | 0 |
| `--verbose` | Enable verbose output | `IMAPVERBOSE` | false |
| `--loglevel` | Log level: DEBUG, INFO, WARN, ERROR | `IMAPLOGLEVEL` | INFO |
| `--output` | Output format: text, json | `IMAPOUTPUT` | text |
| `--logformat` | Log file format: csv, json | `IMAPLOGFORMAT` | csv |

### exportmessages-specific

| Flag | Description | Environment Variable | Default |
|------|-------------|---------------------|---------|
| `--messageid` | Message-ID header value to search for | `IMAPMESSAGEID` | — |
| `--subject` | Subject substring to search for | `IMAPSUBJECT` | — |
| `--mailbox` | Mailbox to search | `IMAPMAILBOX` | INBOX |
| `--count` | Maximum number of matching messages to export | `IMAPCOUNT` | 25 |
| `--exportdir` | Directory under which to create the dated export folder | `IMAPEXPORTDIR` | OS temp dir |

**Note:** `--imaps` and `--starttls` cannot be used together. When `--imaps` is set and port is the default 143, the port automatically changes to 993. `--no-imaps`+`--imaps` and `--no-starttls`+`--starttls` are each mutually exclusive (useful to catch conflicting defaults from `--config`/env vars).

## Environment Variables

```powershell
$env:IMAPHOST = "imap.example.com"
$env:IMAPPORT = "993"
$env:IMAPIMAPS = "true"
$env:IMAPUSERNAME = "user@example.com"
$env:IMAPPASSWORD = "yourpassword"

gomailtest imap testauth
```

## Common Ports

| Port | Usage | TLS |
|------|-------|-----|
| 143 | IMAP (standard) | Optional STARTTLS |
| 993 | IMAPS (implicit TLS) | Implicit TLS |

## Common Provider Configurations

| Provider | Host | Port | Method |
|----------|------|------|--------|
| Gmail | imap.gmail.com | 993 | `--imaps` |
| Microsoft 365 | outlook.office365.com | 993 | `--imaps` |
| Yahoo | imap.mail.yahoo.com | 993 | `--imaps` |
| iCloud | imap.mail.me.com | 993 | `--imaps` |

```powershell
# Gmail with password
gomailtest imap testauth --host imap.gmail.com --imaps \
    --username user@gmail.com --password "app-password"

# Microsoft 365
gomailtest imap testauth --host outlook.office365.com --imaps \
    --username user@company.com --password "password"

# Gmail with OAuth2
gomailtest imap testauth --host imap.gmail.com --imaps \
    --username user@gmail.com --accesstoken "ya29.xxx..."
```

## CSV Logging

Operations are logged to `%TEMP%\_imaptool_{action}_{date}.csv`.

## Related Documentation

- [BUILD.md](../../BUILD.md) — Build instructions
- [docs/protocols/smtp.md](smtp.md) — SMTP tool
- [docs/protocols/pop3.md](pop3.md) — POP3 tool
- [SECURITY.md](../../SECURITY.md) — Security policy

                          ..ooOO END OOoo..
