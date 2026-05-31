# gomailtesttool

Portable CLI for testing email infrastructure: SMTP, IMAP, POP3, JMAP, and Microsoft EWS (on-prem) + Graph (Exchange Online). Single binary, no runtime dependencies, cross-platform (Windows, Linux, macOS).

**Repository:** [https://github.com/ziembor/gomailtesttool](https://github.com/ziembor/gomailtesttool)

## Commands

```
gomailtest <protocol> <action> [flags]
```

| Protocol | Actions | Use case |
|----------|---------|----------|
| `smtp` | `testconnect`, `teststarttls`, `testauth`, `sendmail` | On-premises SMTP / Exchange relay |
| `imap` | `testconnect`, `testauth`, `listfolders` | IMAP mailbox access |
| `pop3` | `testconnect`, `testauth`, `listmail` | POP3 mailbox access |
| `jmap` | `testconnect`, `testauth`, `getmailboxes` | JMAP (RFC 8620) servers |
| `ews` | `testconnect`, `testauth`, `getfolder`, `autodiscover` | On-premises Exchange via EWS (Exchange 2007–2019) |
| `msgraph` | `getevents`, `sendmail`, `sendinvite`, `getinbox`, `getschedule`, `exportinbox`, `searchandexport` | Exchange Online via Microsoft Graph API |

Run `gomailtest <protocol> --help` for flags and environment variables.

## Quick Start

### Build

```powershell
# Build all tools
.\build-all.ps1

# Or build only the unified binary
go build -o bin/gomailtest.exe ./cmd/gomailtest
```

See [BUILD.md](BUILD.md) for cross-platform builds and individual binary instructions.

### SMTP

```powershell
# Test connectivity
gomailtest smtp testconnect --host smtp.example.com --port 25

# Comprehensive TLS diagnostics
gomailtest smtp teststarttls --host smtp.example.com --port 587

# Test authentication (auto-selects best method)
gomailtest smtp testauth --host smtp.example.com --port 587 \
  --username user@example.com --password "..."

# NTLM — on-premises Exchange / Windows SMTP
gomailtest smtp testauth --host exchange.contoso.com --port 25 \
  --username "CONTOSO\user" --password "..."

# GSSAPI/Kerberos — on-premises Exchange / Active Directory
gomailtest smtp testauth --host exchange.contoso.com --port 25 \
  --username "alice@CONTOSO.COM" --password "..." --authmethod GSSAPI

# XOAUTH2 — Microsoft 365 / Google Workspace
gomailtest smtp testauth --host smtp.office365.com --port 587 \
  --username user@company.com --accesstoken "eyJ..."

# Send test email
gomailtest smtp sendmail --host smtp.example.com --port 587 \
  --username user@example.com --password "..." \
  --from sender@example.com --to recipient@example.com
```

See [docs/protocols/smtp.md](docs/protocols/smtp.md) for full documentation.

### IMAP

```powershell
gomailtest imap testconnect --host imap.gmail.com --imaps
gomailtest imap testauth --host imap.gmail.com --imaps \
  --username user@gmail.com --password "app-password"
gomailtest imap listfolders --host imap.gmail.com --imaps \
  --username user@gmail.com --password "app-password"
```

See [docs/protocols/imap.md](docs/protocols/imap.md) for full documentation.

### POP3

```powershell
gomailtest pop3 testconnect --host pop.example.com --port 995 --pop3s
gomailtest pop3 testauth --host pop.example.com --port 995 --pop3s \
  --username user@example.com --password "..."
```

See [docs/protocols/pop3.md](docs/protocols/pop3.md) for full documentation.

### JMAP

```powershell
gomailtest jmap testconnect --host jmap.fastmail.com
gomailtest jmap getmailboxes --host jmap.fastmail.com \
  --username user@fastmail.com --accesstoken "fmu1-..."
```

See [docs/protocols/jmap.md](docs/protocols/jmap.md) for full documentation.

### EWS (Exchange Web Services)

```powershell
gomailtest ews testconnect --host mail.example.com
gomailtest ews testauth --host mail.example.com \
  --username "DOMAIN\user" --password "..."
gomailtest ews getfolder --host mail.example.com \
  --username "DOMAIN\user" --password "..."
gomailtest ews autodiscover --host mail.example.com \
  --username user@example.com
```

See [docs/protocols/ews.md](docs/protocols/ews.md) for full documentation.

### Microsoft Graph (Exchange Online)

```powershell
$env:MSGRAPHTENANTID = "your-tenant-id"
$env:MSGRAPHCLIENTID = "your-client-id"
$env:MSGRAPHSECRET   = "your-secret"
$env:MSGRAPHMAILBOX  = "user@example.com"

gomailtest msgraph getevents
gomailtest msgraph sendmail --to "recipient@example.com"
gomailtest msgraph getinbox --count 10
```

See [docs/protocols/msgraph.md](docs/protocols/msgraph.md) for full documentation.

## SMTPS vs STARTTLS

| Method | Port | Flag | Description |
|--------|------|------|-------------|
| SMTPS | 465 | `--smtps` | Implicit TLS — encryption starts immediately |
| STARTTLS | 587/25 | `--starttls` | Explicit TLS — plain connection upgrades after STARTTLS |

| Provider | SMTPS (port 465) | STARTTLS (port 587) |
|----------|------------------|---------------------|
| Gmail | `smtp.gmail.com --smtps` | `smtp.gmail.com --port 587` |
| Microsoft 365 | Not supported | `smtp.office365.com --port 587` |
| Yahoo | `smtp.mail.yahoo.com --smtps` | `smtp.mail.yahoo.com --port 587` |

## Environment Variables

Each protocol uses a dedicated prefix:

| Protocol | Prefix | Example |
|----------|--------|---------|
| SMTP | `SMTP` | `SMTPHOST`, `SMTPPORT`, `SMTPPASSWORD` |
| IMAP | `IMAP` | `IMAPHOST`, `IMAPPORT`, `IMAPPASSWORD` |
| POP3 | `POP3` | `POP3HOST`, `POP3PORT`, `POP3PASSWORD` |
| JMAP | `JMAP` | `JMAPHOST`, `JMAPPORT`, `JMAPACCESSTOKEN` |
| EWS | `EWS` | `EWSHOST`, `EWSUSERNAME`, `EWSPASSWORD` |
| msgraph | `MSGRAPH` | `MSGRAPHTENANTID`, `MSGRAPHSECRET` |

## Migrating from Legacy Binary Names

The individual tool binaries (`smtptool`, `imaptool`, `pop3tool`, `jmaptool`, `msgraphtool`) were removed in v3.1.0. Replace them with `gomailtest <protocol> <action> --flag`:

| Old | New |
|-----|-----|
| `smtptool -action testconnect -host X` | `gomailtest smtp testconnect --host X` |
| `imaptool -action testauth -host X -imaps` | `gomailtest imap testauth --host X --imaps` |
| `pop3tool -action listmail -host X -pop3s` | `gomailtest pop3 listmail --host X --pop3s` |
| `jmaptool -action getmailboxes -host X` | `gomailtest jmap getmailboxes --host X` |
| `msgraphtool -action getevents` | `gomailtest msgraph getevents` |

## Documentation

### Protocol Docs
- [docs/protocols/smtp.md](docs/protocols/smtp.md) — SMTP tool
- [docs/protocols/imap.md](docs/protocols/imap.md) — IMAP tool
- [docs/protocols/pop3.md](docs/protocols/pop3.md) — POP3 tool
- [docs/protocols/jmap.md](docs/protocols/jmap.md) — JMAP tool
- [docs/protocols/ews.md](docs/protocols/ews.md) — EWS tool (on-premises Exchange)
- [docs/protocols/msgraph.md](docs/protocols/msgraph.md) — Microsoft Graph tool
- [docs/protocols/serve.md](docs/protocols/serve.md) — HTTP serve mode (REST API with X-API-Key)

### General Docs
- [BUILD.md](BUILD.md) — Build instructions
- [docs/protocols/msgraph.md](docs/protocols/msgraph.md) — Microsoft Graph examples, CSV schemas, and tips
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) — Common errors and solutions
- [SECURITY.md](SECURITY.md) — Security policy and threat model
- [RELEASE.md](RELEASE.md) — Release process and versioning policy

## Security

These are diagnostic CLI tools designed for authorized personnel (system administrators, IT staff).

- CLI flags and environment variables are **trusted input** from authorized users
- **Not designed** for untrusted web/API input or public-facing services
- Defense-in-depth measures: CRLF sanitization, password masking in logs
- See [SECURITY.md](SECURITY.md) for the complete threat model

## License

Provided as-is for testing and automation purposes.

                          ..ooOO END OOoo..
