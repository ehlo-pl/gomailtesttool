# EWS Protocol — gomailtest

Exchange Web Services (EWS) connectivity, TLS diagnostics, authentication testing, folder access, and Autodiscover resolution for on-premises Exchange Server.

## Quick Start

```powershell
# Test basic EWS connectivity (no credentials required)
gomailtest ews testconnect --host mail.example.com

# Test NTLM authentication
gomailtest ews testauth --host mail.example.com \
    --username "DOMAIN\user" --password "secret"

# Test Basic authentication
gomailtest ews testauth --host mail.example.com \
    --username user@example.com --password "secret" --authmethod Basic

# Test Bearer (OAuth2) authentication
gomailtest ews testauth --host mail.example.com \
    --username user@example.com --accesstoken "eyJ0..."

# Get Inbox folder properties
gomailtest ews getfolder --host mail.example.com \
    --username "DOMAIN\user" --password "secret"

# Run Autodiscover for a mailbox
gomailtest ews autodiscover --host mail.example.com \
    --username user@example.com
```

## Actions

### testconnect — HTTP/TLS Connectivity

Probes the EWS endpoint over HTTPS. No credentials are required — an HTTP 401 Unauthorized response confirms the server is alive and EWS is enabled. Reports TLS version, cipher suite, and certificate details.

```powershell
# Default port 443
gomailtest ews testconnect --host mail.example.com

# Custom port
gomailtest ews testconnect --host mail.example.com --port 8443

# Skip TLS certificate verification (self-signed certs)
gomailtest ews testconnect --host mail.example.com --skipverify

# Custom EWS endpoint path
gomailtest ews testconnect --host mail.example.com \
    --ewspath /EWS/Exchange.asmx

# Verbose output
gomailtest ews testconnect --host mail.example.com --verbose
```

### testauth — Authentication Testing

Authenticates to EWS and verifies credentials by performing a `GetFolder(Inbox)` SOAP call. Supports NTLM, Basic, and Bearer (OAuth2) authentication. Auth method is auto-detected when `--authmethod auto` (default).

**Auto-detection rules:**
- Bearer — when `--accesstoken` is provided
- NTLM — when username contains a backslash (`DOMAIN\user`) or `--domain` is set
- Basic — fallback

```powershell
# NTLM (on-premises Active Directory)
gomailtest ews testauth --host mail.example.com \
    --username "CORP\jsmith" --password "secret"

# NTLM with separate domain flag
gomailtest ews testauth --host mail.example.com \
    --username jsmith --domain CORP --password "secret"

# Basic authentication
gomailtest ews testauth --host mail.example.com \
    --username user@example.com --password "secret" \
    --authmethod Basic

# Bearer (OAuth2)
gomailtest ews testauth --host mail.example.com \
    --username user@example.com --accesstoken "eyJ0..."

# With mailbox impersonation
gomailtest ews testauth --host mail.example.com \
    --username "CORP\serviceaccount" --password "secret" \
    --mailbox targetuser@example.com
```

### getfolder — Inbox Folder Properties

Authenticates and retrieves Inbox folder properties: display name, total item count, unread count, and folder ID.

```powershell
# NTLM
gomailtest ews getfolder --host mail.example.com \
    --username "CORP\user" --password "secret"

# Bearer with impersonation
gomailtest ews getfolder --host mail.example.com \
    --username user@example.com --accesstoken "eyJ0..." \
    --mailbox targetuser@example.com
```

### autodiscover — Autodiscover SOAP Endpoint

Posts a `GetUserSettings` request to the Autodiscover endpoint and reports the resolved EWS URLs (internal and external), user display name, and Active Directory server. Useful for diagnosing Exchange client configuration issues.

```powershell
# Query Autodiscover for a mailbox
gomailtest ews autodiscover --host mail.example.com \
    --username user@example.com

# Custom Autodiscover path
gomailtest ews autodiscover --host mail.example.com \
    --username user@example.com \
    --autodiscoverpath /autodiscover/autodiscover.svc
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--host` | *(required)* | Exchange server hostname or IP address |
| `--port` | `443` | HTTPS port |
| `--timeout` | `30` | Connection timeout in seconds |
| `--ewspath` | `/EWS/Exchange.asmx` | EWS endpoint path |
| `--autodiscoverpath` | `/autodiscover/autodiscover.svc` | Autodiscover endpoint path |
| `--username` | | Username: `DOMAIN\user` for NTLM, email for Basic/Bearer |
| `--password` | | Password |
| `--accesstoken` | | OAuth2 Bearer token |
| `--authmethod` | `auto` | Auth method: `NTLM`, `Basic`, `Bearer`, `auto` |
| `--domain` | | AD domain for NTLM (optional; can be embedded in username) |
| `--mailbox` | | Target mailbox SMTP address for impersonation |
| `--skipverify` | `false` | Skip TLS certificate verification (self-signed certs) |
| `--tlsversion` | `1.2` | Minimum TLS version: `1.2`, `1.3` |
| `--proxy` | | HTTP/HTTPS or SOCKS5 proxy URL |
| `--ipv4` | `false` | Force IPv4: resolve --host to an A record and connect over IPv4 |
| `--ipv6` | `false` | Force IPv6: resolve --host to an AAAA record and connect over IPv6 |
| `--verbose` | `false` | Enable verbose output |
| `--loglevel` | `INFO` | Logging level: `DEBUG`, `INFO`, `WARN`, `ERROR` |
| `--logformat` | `csv` | Log file format: `csv`, `json` |

## Environment Variables

All flags can be set via environment variables using the `EWS` prefix:

| Variable | Flag equivalent | Description |
|----------|----------------|-------------|
| `EWSHOST` | `--host` | Exchange server hostname |
| `EWSPORT` | `--port` | HTTPS port |
| `EWSTIMEOUT` | `--timeout` | Connection timeout in seconds |
| `EWSPATH` | `--ewspath` | EWS endpoint path |
| `EWSAUTODISCOVERPATH` | `--autodiscoverpath` | Autodiscover path |
| `EWSUSERNAME` | `--username` | Username |
| `EWSPASSWORD` | `--password` | Password |
| `EWSACCESSTOKEN` | `--accesstoken` | OAuth2 Bearer token |
| `EWSAUTHMETHOD` | `--authmethod` | Auth method |
| `EWSDOMAIN` | `--domain` | AD domain for NTLM |
| `EWSMAILBOX` | `--mailbox` | Impersonation mailbox |
| `EWSSKIPVERIFY` | `--skipverify` | Skip TLS verification |
| `EWSTLSVERSION` | `--tlsversion` | Minimum TLS version |
| `EWSPROXY` | `--proxy` | Proxy URL |
| `EWSLOGFORMAT` | `--logformat` | Log format |

```powershell
# Set credentials via environment variables
$env:EWSHOST     = "mail.example.com"
$env:EWSUSERNAME = "CORP\user"
$env:EWSPASSWORD = "secret"

gomailtest ews testauth
gomailtest ews getfolder
```

## Exchange Version Support

EWS is available on Exchange Server 2007 through Exchange Server 2019. Microsoft deprecated EWS for Exchange Online (Microsoft 365) on October 1, 2026 — use `gomailtest msgraph` for Exchange Online workloads.

## Authentication Methods

| Method | When to use |
|--------|-------------|
| NTLM | On-premises Exchange with Active Directory |
| Basic | Exchange with basic auth enabled (legacy) |
| Bearer | Hybrid or modern auth with OAuth2 token |

## Log Files

CSV and JSON log files are created with the pattern `_ewstool_{action}_{date}.{ext}`:

```
_ewstool_testconnect_20260427.csv
_ewstool_testauth_20260427.csv
_ewstool_getfolder_20260427.csv
_ewstool_autodiscover_20260427.csv
```

## Troubleshooting

| Error | Cause | Resolution |
|-------|-------|------------|
| `HTTP 401 Unauthorized` on testconnect | Expected — server is alive, needs auth | Use `testauth` with credentials |
| `HTTP 403 Forbidden` | EWS blocked or insufficient permissions | Check EWS is enabled; verify CAS settings |
| `x509: certificate signed by unknown authority` | Self-signed or private CA cert | Use `--skipverify` for testing only |
| `connection refused` | Wrong host/port or firewall | Verify `--host` and `--port`; check firewall rules |
| `NTLM: authentication failed` | Wrong credentials or domain | Verify `DOMAIN\user` format or use `--domain` flag |
| `autodiscover requires --username (email address)` | Non-email username passed | Pass a valid email address to `--username` |

## Related Documentation

- [README.md](../../README.md) — Project overview
- [docs/protocols/msgraph.md](msgraph.md) — Microsoft Graph (Exchange Online)
- [INTEGRATION_TESTS.md](../../INTEGRATION_TESTS.md) — Integration test guide
- [UNIT_TESTS.md](../../UNIT_TESTS.md) — Unit test documentation

                          ..ooOO END OOoo..
