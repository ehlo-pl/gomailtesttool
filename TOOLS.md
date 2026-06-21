# gomailtesttool Suite - Protocol Comparison

This document provides a comprehensive comparison of all protocols supported by the `gomailtest` unified CLI.

> Since v3.1, `gomailtest` is a single binary. Every protocol is a subcommand,
> and every action is a sub-subcommand:
>
> ```
> gomailtest <protocol> <action> [flags]
> ```
>
> The legacy per-protocol binaries (`smtptool`, `imaptool`, `pop3tool`, `jmaptool`, `ewstool`, `msgraphtool`) were removed in v3.1.0.

## Quick Reference

| Protocol | Subcommand | Standard Port | Primary Use Case |
|----------|------------|----------------|------------------|
| **SMTP** | `gomailtest smtp` | 25/587/465 | Test SMTP servers, TLS, authentication |
| **IMAP** | `gomailtest imap` | 143/993 | Test IMAP servers, list folders |
| **POP3** | `gomailtest pop3` | 110/995 | Test POP3 servers, list messages |
| **JMAP** | `gomailtest jmap` | 443 | Test JMAP servers (modern email API) |
| **EWS** | `gomailtest ews` | 443 | Test on-premises Exchange EWS (Exchange 2007–2019) |
| **Microsoft Graph** | `gomailtest msgraph` | 443 | Exchange Online via Microsoft Graph API |

---

## Feature Comparison

### Protocol Support

| Feature | smtp | imap | pop3 | jmap | ews | msgraph |
|---------|------|------|------|------|-----|---------|
| TCP Connection | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Implicit TLS (SSL) | ✅ (SMTPS) | ✅ (IMAPS) | ✅ (POP3S) | ✅ (HTTPS) | ✅ (HTTPS) | ✅ (HTTPS) |
| STARTTLS | ✅ | ✅ | ✅ | N/A | N/A | N/A |
| TLS Version Detection | ✅ | ✅ | ✅ | ✅ | ✅ | - |
| Certificate Validation | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Skip TLS Verification | ✅ | ✅ | ✅ | ✅ | ✅ | - |

### Authentication Methods

| Method | smtp | imap | pop3 | jmap | ews | msgraph |
|--------|------|------|------|------|-----|---------|
| PLAIN | ✅ | ✅ | - | - | - | - |
| LOGIN | ✅ | ✅ | - | - | - | - |
| CRAM-MD5 | ✅ | - | - | - | - | - |
| XOAUTH2 | ✅ | ✅ | ✅ | - | - | - |
| OAUTHBEARER | ✅ | - | - | - | - | - |
| USER/PASS | - | - | ✅ | - | - | - |
| APOP | - | - | ✅ | - | - | - |
| NTLM | ✅ | - | - | - | ✅ | - |
| GSSAPI (Kerberos) | ✅ | - | - | - | - | - |
| Basic Auth | - | - | - | ✅ | ✅ | - |
| Bearer Token | - | - | - | ✅ | ✅ | ✅ |
| Client Secret | - | - | - | - | - | ✅ |
| Certificate (PFX) | - | - | - | - | - | ✅ |
| Windows Cert Store | - | - | - | - | - | ✅ |

### Available Actions

| Action | smtp | imap | pop3 | jmap | ews | msgraph |
|--------|------|------|------|------|-----|---------|
| Test Connection | ✅ `testconnect` | ✅ `testconnect` | ✅ `testconnect` | ✅ `testconnect` | ✅ `testconnect` | - |
| Test STARTTLS | ✅ `teststarttls` | - | - | - | - | - |
| Test Authentication | ✅ `testauth` | ✅ `testauth` | ✅ `testauth` | ✅ `testauth` | ✅ `testauth` | - |
| Send Email | ✅ `sendmail` | - | - | - | - | ✅ `sendmail` |
| List Folders | - | ✅ `listfolders` | - | ✅ `getmailboxes` | - | - |
| List Messages | - | - | ✅ `listmail` | - | - | - |
| Get Inbox | - | - | - | - | - | ✅ `getinbox` |
| Get Events | - | - | - | - | - | ✅ `getevents` |
| Get Schedule | - | - | - | - | - | ✅ `getschedule` |
| Send Invite | - | - | - | - | - | ✅ `sendinvite` |
| Export Inbox | - | - | - | - | - | ✅ `exportinbox` |
| Search & Export | - | - | - | - | - | ✅ `searchandexport` |
| Export Messages (.eml) | - | - | - | - | - | ✅ `exportmessages` |
| Get Folder | - | - | - | - | ✅ `getfolder` | - |
| Autodiscover | - | - | - | - | ✅ `autodiscover` | - |

---

## Port Reference

| Protocol | Standard Port | TLS Port | Description |
|----------|---------------|----------|-------------|
| SMTP | 25 | 465 (SMTPS) | Mail transfer |
| SMTP Submission | 587 | 465 | Mail submission (recommended) |
| IMAP | 143 | 993 (IMAPS) | Mail access |
| POP3 | 110 | 995 (POP3S) | Mail retrieval |
| JMAP | 443 | 443 | Modern mail API (always HTTPS) |
| EWS | 443 | 443 | Exchange Web Services (always HTTPS) |
| Microsoft Graph | 443 | 443 | Cloud API (always HTTPS) |

---

## Environment Variables

### Naming Convention

Each protocol's flags can be set via environment variables using a consistent prefix: `{PREFIX}{PARAMETER}` (no underscores)

| Protocol | Prefix | Example |
|----------|--------|---------|
| smtp | `SMTP` | `SMTPHOST`, `SMTPPORT`, `SMTPUSERNAME` |
| imap | `IMAP` | `IMAPHOST`, `IMAPPORT`, `IMAPUSERNAME` |
| pop3 | `POP3` | `POP3HOST`, `POP3PORT`, `POP3USERNAME` |
| jmap | `JMAP` | `JMAPHOST`, `JMAPPORT`, `JMAPUSERNAME` |
| ews | `EWS` | `EWSHOST`, `EWSPORT`, `EWSUSERNAME` |
| msgraph | `MSGRAPH` | `MSGRAPHTENANTID`, `MSGRAPHCLIENTID` |

### Common Environment Variables

| Variable Pattern | Description |
|-----------------|-------------|
| `{PREFIX}HOST` | Server hostname |
| `{PREFIX}PORT` | Server port |
| `{PREFIX}ADDRESS` | Override connection address (IP or hostname for TCP connection) |
| `{PREFIX}USERNAME` | Username for authentication |
| `{PREFIX}PASSWORD` | Password for authentication |
| `{PREFIX}ACCESSTOKEN` | OAuth2/Bearer access token |
| `{PREFIX}VERBOSE` | Enable verbose output |
| `{PREFIX}LOGLEVEL` | Log level (debug, info, warn, error) |
| `{PREFIX}LOGFORMAT` | Log format (csv, json) |

> Note: there is no `{PREFIX}ACTION` variable. The action is selected by the
> subcommand you run (e.g. `gomailtest smtp testconnect`), not by a flag or
> environment variable.

---

## Output Formats

All protocols support:
- **Console Output**: Human-readable status and results
- **CSV Logging**: Structured logs for analysis
- **JSON Logging**: Machine-readable logs

### CSV Log Files

Log files are created with the pattern: `_{protocol}tool_{action}_{date}.csv`

Example: `_smtptool_testconnect_20260131.csv`

---

## Quick Start Examples

### SMTP Testing

```bash
# Test basic connectivity
gomailtest smtp testconnect --host smtp.example.com --port 25

# Test STARTTLS / TLS diagnostics
gomailtest smtp teststarttls --host smtp.example.com --port 587

# Test authentication
gomailtest smtp testauth --host smtp.example.com --port 587 \
  --username user@example.com --password "secret"

# Send test email
gomailtest smtp sendmail --host smtp.example.com --port 587 \
  --username user@example.com --password "secret" \
  --from sender@example.com --to recipient@example.com \
  --subject "Test" --body "Hello"
```

### IMAP Testing

```bash
# Test connection with IMAPS
gomailtest imap testconnect --host imap.gmail.com --port 993 --imaps

# Test authentication
gomailtest imap testauth --host imap.gmail.com --port 993 --imaps \
  --username user@gmail.com --password "app-password"

# List folders with OAuth2
gomailtest imap listfolders --host imap.gmail.com --port 993 --imaps \
  --username user@gmail.com --accesstoken "ya29..."
```

### POP3 Testing

```bash
# Test connection with POP3S
gomailtest pop3 testconnect --host pop.gmail.com --port 995 --pop3s

# Test authentication
gomailtest pop3 testauth --host pop.gmail.com --port 995 --pop3s \
  --username user@gmail.com --password "app-password"

# List messages with OAuth2
gomailtest pop3 listmail --host pop.gmail.com --port 995 --pop3s \
  --username user@gmail.com --accesstoken "ya29..."
```

### JMAP Testing

```bash
# Test JMAP session discovery
gomailtest jmap testconnect --host jmap.fastmail.com

# Test authentication with Bearer token
gomailtest jmap testauth --host jmap.fastmail.com \
  --username user@fastmail.com --accesstoken "fmu1-..."

# Get mailboxes
gomailtest jmap getmailboxes --host jmap.fastmail.com \
  --username user@fastmail.com --accesstoken "fmu1-..."
```

### EWS Testing

```bash
# Test EWS connectivity (no credentials required)
gomailtest ews testconnect --host mail.example.com

# Test NTLM authentication
gomailtest ews testauth --host mail.example.com \
  --username "CORP\user" --password "secret"

# Get Inbox folder properties
gomailtest ews getfolder --host mail.example.com \
  --username "CORP\user" --password "secret"

# Run Autodiscover
gomailtest ews autodiscover --host mail.example.com \
  --username user@example.com
```

### Microsoft Graph Testing

```bash
# Get inbox messages
gomailtest msgraph getinbox --tenantid "..." --clientid "..." --secret "..." \
  --mailbox "user@example.com"

# Send email
gomailtest msgraph sendmail --tenantid "..." --clientid "..." --secret "..." \
  --mailbox "user@example.com" \
  --to "recipient@example.com" --subject "Test" --body "Hello"

# Export messages matching a subject as .eml (using a Bearer token)
gomailtest msgraph exportmessages --bearertoken "eyJ0..." \
  --mailbox "user@example.com" --subject "Invoice"
```

---

## Choosing the Right Protocol

| Scenario | Recommended Subcommand |
|----------|------------------------|
| Testing on-premises Exchange/SMTP | `gomailtest smtp` |
| Testing on-premises Exchange EWS | `gomailtest ews` |
| Testing Gmail/Office 365 IMAP | `gomailtest imap` |
| Testing legacy POP3 servers | `gomailtest pop3` |
| Testing modern email providers (Fastmail) | `gomailtest jmap` |
| Testing Exchange Online | `gomailtest msgraph` |
| TLS/SSL diagnostics | `gomailtest smtp teststarttls` (best TLS analysis) |
| OAuth2/XOAUTH2 testing | `gomailtest imap`, `gomailtest pop3` |
| Bulk mailbox operations | `gomailtest msgraph` |
| Autodiscover troubleshooting | `gomailtest ews autodiscover` |

---

## Platform Support

All platforms build a single `gomailtest` binary (see [BUILD.md](BUILD.md) for cross-platform build commands).

| Platform | Architecture | Binary |
|----------|--------------|--------|
| Windows | amd64 | `gomailtest.exe` |
| Linux | amd64 | `gomailtest` |
| macOS | amd64 / arm64 (Apple Silicon) | `gomailtest` |

---

## See Also

- [README.md](README.md) - Project overview
- [BUILD.md](BUILD.md) - Build instructions
- [docs/protocols/msgraph.md](docs/protocols/msgraph.md) - Microsoft Graph examples and reference

### Protocol-Specific Documentation

- [docs/protocols/ews.md](docs/protocols/ews.md) - EWS protocol documentation
- [docs/protocols/msgraph.md](docs/protocols/msgraph.md) - Microsoft Graph protocol documentation
- [docs/protocols/smtp.md](docs/protocols/smtp.md) - SMTP protocol documentation
- [docs/protocols/imap.md](docs/protocols/imap.md) - IMAP protocol documentation
- [docs/protocols/pop3.md](docs/protocols/pop3.md) - POP3 protocol documentation
- [docs/protocols/jmap.md](docs/protocols/jmap.md) - JMAP protocol documentation
