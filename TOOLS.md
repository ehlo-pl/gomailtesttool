# gomailtesttool Suite - Tool Comparison

This document provides a comprehensive comparison of all tools in the gomailtesttool suite.

## Quick Reference

| Tool | Protocol | Default Port | Primary Use Case |
|------|----------|--------------|------------------|
| **smtptool** | SMTP | 25/587/465 | Test SMTP servers, TLS, authentication |
| **imaptool** | IMAP | 143/993 | Test IMAP servers, list folders |
| **pop3tool** | POP3 | 110/995 | Test POP3 servers, list messages |
| **jmaptool** | JMAP | 443 | Test JMAP servers (modern email API) |
| **ewstool** | EWS | 443 | Test on-premises Exchange EWS (Exchange 2007–2019) |
| **msgraphtool** | Microsoft Graph | 443 | Exchange Online via Microsoft Graph API |

---

## Feature Comparison

### Protocol Support

| Feature | smtptool | imaptool | pop3tool | jmaptool | ewstool | msgraphtool |
|---------|----------|----------|----------|----------|---------|-------------|
| TCP Connection | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Implicit TLS (SSL) | ✅ (SMTPS) | ✅ (IMAPS) | ✅ (POP3S) | ✅ (HTTPS) | ✅ (HTTPS) | ✅ (HTTPS) |
| STARTTLS | ✅ | ✅ | ✅ | N/A | N/A | N/A |
| TLS Version Detection | ✅ | ✅ | ✅ | ✅ | ✅ | - |
| Certificate Validation | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Skip TLS Verification | ✅ | ✅ | ✅ | ✅ | ✅ | - |

### Authentication Methods

| Method | smtptool | imaptool | pop3tool | jmaptool | ewstool | msgraphtool |
|--------|----------|----------|----------|----------|---------|-------------|
| PLAIN | ✅ | ✅ | - | - | - | - |
| LOGIN | ✅ | ✅ | - | - | - | - |
| CRAM-MD5 | ✅ | - | - | - | - | - |
| XOAUTH2 | ✅ | ✅ | ✅ | - | - | - |
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

| Action | smtptool | imaptool | pop3tool | jmaptool | ewstool | msgraphtool |
|--------|----------|----------|----------|----------|---------|-------------|
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

All tools use a consistent naming pattern: `{TOOL}{PARAMETER}` (no underscores)

| Tool | Prefix | Example |
|------|--------|---------|
| smtptool | `SMTP` | `SMTPHOST`, `SMTPPORT`, `SMTPUSERNAME` |
| imaptool | `IMAP` | `IMAPHOST`, `IMAPPORT`, `IMAPUSERNAME` |
| pop3tool | `POP3` | `POP3HOST`, `POP3PORT`, `POP3USERNAME` |
| jmaptool | `JMAP` | `JMAPHOST`, `JMAPPORT`, `JMAPUSERNAME` |
| ewstool | `EWS` | `EWSHOST`, `EWSPORT`, `EWSUSERNAME` |
| msgraphtool | `MSGRAPH` | `MSGRAPHTENANTID`, `MSGRAPHCLIENTID` |

### Common Environment Variables

| Variable Pattern | Description |
|-----------------|-------------|
| `{PREFIX}HOST` | Server hostname |
| `{PREFIX}PORT` | Server port |
| `{PREFIX}ADDRESS` | Override connection address (IP or hostname for TCP connection) |
| `{PREFIX}USERNAME` | Username for authentication |
| `{PREFIX}PASSWORD` | Password for authentication |
| `{PREFIX}ACCESSTOKEN` | OAuth2/Bearer access token |
| `{PREFIX}ACTION` | Action to perform |
| `{PREFIX}VERBOSE` | Enable verbose output |
| `{PREFIX}LOGLEVEL` | Log level (debug, info, warn, error) |
| `{PREFIX}LOGFORMAT` | Log format (csv, json) |

---

## Output Formats

All tools support:
- **Console Output**: Human-readable status and results
- **CSV Logging**: Structured logs for analysis
- **JSON Logging**: Machine-readable logs

### CSV Log Files

Log files are created with the pattern: `_{tool}_{action}_{date}.csv`

Example: `_smtptool_testconnect_20260131.csv`

---

## Quick Start Examples

### SMTP Testing

```bash
# Test basic connectivity
./smtptool -action testconnect -host smtp.example.com -port 25

# Test STARTTLS upgrade
./smtptool -action teststarttls -host smtp.example.com -port 587

# Test authentication
./smtptool -action testauth -host smtp.example.com -port 587 \
  -username user@example.com -password "secret" -starttls

# Send test email
./smtptool -action sendmail -host smtp.example.com -port 587 \
  -username user@example.com -password "secret" -starttls \
  -to recipient@example.com -subject "Test" -body "Hello"
```

### IMAP Testing

```bash
# Test connection with IMAPS
./imaptool -action testconnect -host imap.gmail.com -imaps

# Test authentication
./imaptool -action testauth -host imap.gmail.com -imaps \
  -username user@gmail.com -password "app-password"

# List folders with OAuth2
./imaptool -action listfolders -host imap.gmail.com -imaps \
  -username user@gmail.com -accesstoken "ya29..."
```

### POP3 Testing

```bash
# Test connection with POP3S
./pop3tool -action testconnect -host pop.gmail.com -pop3s

# Test authentication
./pop3tool -action testauth -host pop.gmail.com -pop3s \
  -username user@gmail.com -password "app-password"

# List messages
./pop3tool -action listmail -host pop.gmail.com -pop3s \
  -username user@gmail.com -accesstoken "ya29..."
```

### JMAP Testing

```bash
# Test JMAP session discovery
./jmaptool -action testconnect -host jmap.fastmail.com

# Test authentication with Bearer token
./jmaptool -action testauth -host jmap.fastmail.com \
  -username user@fastmail.com -accesstoken "fmu1-..."

# Get mailboxes
./jmaptool -action getmailboxes -host jmap.fastmail.com \
  -username user@fastmail.com -accesstoken "fmu1-..."
```

### EWS Testing

```bash
# Test EWS connectivity
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
./msgraphtool -tenantid "..." -clientid "..." -secret "..." \
  -mailbox "user@example.com" -action getinbox

# Send email
./msgraphtool -tenantid "..." -clientid "..." -secret "..." \
  -mailbox "user@example.com" -action sendmail \
  -to "recipient@example.com" -subject "Test" -body "Hello"

# Using Bearer token
./msgraphtool -bearertoken "eyJ0..." \
  -mailbox "user@example.com" -action getinbox
```

---

## Choosing the Right Tool

| Scenario | Recommended Tool |
|----------|------------------|
| Testing on-premises Exchange/SMTP | smtptool |
| Testing on-premises Exchange EWS | ewstool |
| Testing Gmail/Office 365 IMAP | imaptool |
| Testing legacy POP3 servers | pop3tool |
| Testing modern email providers (Fastmail) | jmaptool |
| Testing Exchange Online | msgraphtool |
| TLS/SSL diagnostics | smtptool (best TLS analysis) |
| OAuth2/XOAUTH2 testing | imaptool, pop3tool |
| Bulk mailbox operations | msgraphtool |
| Autodiscover troubleshooting | ewstool |

---

## Platform Support

| Platform | Architecture | File |
|----------|--------------|------|
| Windows | amd64 | `gomailtesttool-windows-amd64.zip` |
| Linux | amd64 | `gomailtesttool-linux-amd64.zip` |
| macOS | arm64 (Apple Silicon) | `gomailtesttool-macos-arm64.zip` |

---

## See Also

- [README.md](README.md) - Project overview
- [BUILD.md](BUILD.md) - Build instructions
- [docs/protocols/msgraph.md](docs/protocols/msgraph.md) - Microsoft Graph examples and reference

### Tool-Specific Documentation

- [docs/protocols/ews.md](docs/protocols/ews.md) - EWS tool documentation
- [docs/protocols/msgraph.md](docs/protocols/msgraph.md) - Microsoft Graph tool documentation
- [docs/protocols/smtp.md](docs/protocols/smtp.md) - SMTP tool documentation
- [docs/protocols/imap.md](docs/protocols/imap.md) - IMAP tool documentation
- [docs/protocols/pop3.md](docs/protocols/pop3.md) - POP3 tool documentation
- [docs/protocols/jmap.md](docs/protocols/jmap.md) - JMAP tool documentation
