# gomailtesttool

Portable, single-binary CLI tools for testing and managing email infrastructure across Microsoft Graph, SMTP, IMAP, POP3, and JMAP.

**Repository:** [https://github.com/ehlo-pl/gomailtesttool](https://github.com/ehlo-pl/gomailtesttool)

## Purpose

This repository contains five complementary tools for comprehensive email infrastructure testing:

- **msgraphtool**: Microsoft Graph API client for Exchange Online mailbox operations (send mail, calendar events, inbox management).
- **smtptool**: SMTP connectivity testing tool with comprehensive TLS diagnostics for on-premises Exchange servers and generic SMTP servers.
- **imaptool**: IMAP connectivity and mailbox testing tool.
- **pop3tool**: POP3 connectivity and message retrieval testing tool.
- **jmaptool**: JMAP protocol testing tool.

All tools are lightweight, standalone executables requiring no additional runtimes or dependencies. Cross-platform support for Windows, Linux, and macOS with automatic CSV logging.

## Key Features

### Microsoft Graph Tool (msgraphtool)
- **Authentication**: Client Secret, PFX Certificate, Windows Certificate Store (Thumbprint).
- **Operations**: Get Events, Send Mail, Send Invite, Get Inbox, Get Schedule, Export Inbox, Search and Export.
- **Target**: Exchange Online (cloud-based) mailboxes.

### SMTP Tool (smtptool)
- **Operations**: Test Connect, Test STARTTLS (comprehensive TLS diagnostics), Test Auth, Send Mail.
- **Diagnostics**: SSL/TLS handshake analysis, certificate validation, cipher strength assessment, Exchange detection.
- **Target**: On-premises Exchange servers and generic SMTP servers.

### Shared Characteristics
- **Logging**: Automatic CSV logging of all operations to `%TEMP%`.
- **Portable**: Single binary, no dependencies.

## Documentation

### Tool-Specific READMEs

- **[MSGRAPHTOOL_README.md](MSGRAPHTOOL_README.md)**: Microsoft Graph tool - Exchange Online operations
- **[SMTP_TOOL_README.md](SMTP_TOOL_README.md)**: SMTP tool - connectivity and email sending
- **[IMAPTOOL_README.md](IMAPTOOL_README.md)**: IMAP tool - mailbox connectivity testing
- **[POP3TOOL_README.md](POP3TOOL_README.md)**: POP3 tool - message retrieval testing
- **[JMAPTOOL_README.md](JMAPTOOL_README.md)**: JMAP tool - modern email protocol testing

### General Documentation

- **[BUILD.md](BUILD.md)**: Build instructions for all tools
- **[EXAMPLES.md](EXAMPLES.md)**: Microsoft Graph tool usage examples
- **[RELEASE.md](RELEASE.md)**: Release process and versioning policy
- **[SECURITY.md](SECURITY.md)**: Security policy, threat model, and best practices
- **[TROUBLESHOOTING.md](TROUBLESHOOTING.md)**: Common errors and solutions

## Quick Start

### Build the Suite
```powershell
# Build all tools at once
.\build-all.ps1

# Or build individually
go build -C cmd/msgraphtool -o msgraphtool.exe
go build -C cmd/smtptool -o smtptool.exe
```
See [BUILD.md](BUILD.md) for cross-platform builds and additional options.

### Usage Examples

**Microsoft Graph Tool:**
```powershell
# Get calendar events
.\msgraphtool.exe -tenantid "..." -clientid "..." -secret "..." -mailbox "user@example.com" -action getevents

# Send email
.\msgraphtool.exe -tenantid "..." -clientid "..." -secret "..." -mailbox "user@example.com" -action sendmail
```
See [EXAMPLES.md](EXAMPLES.md) for comprehensive scenarios.

**SMTP Tool:**
```powershell
# Test SMTP connectivity
.\smtptool.exe -action testconnect -host smtp.example.com -port 25

# Test STARTTLS with comprehensive TLS diagnostics
.\smtptool.exe -action teststarttls -host smtp.example.com -port 587

# Send test email
.\smtptool.exe -action sendmail -host smtp.example.com -port 587 -username "user@example.com" -password "..." -from "sender@example.com" -to "recipient@example.com"
```
See [SMTP_TOOL_README.md](SMTP_TOOL_README.md) for complete documentation.

### SMTPS vs STARTTLS

When connecting to SMTP servers with encryption, there are two distinct methods:

| Method | Port | Description |
|--------|------|-------------|
| **SMTPS** | 465 | Implicit TLS - encryption starts immediately upon connection |
| **STARTTLS** | 587 (or 25) | Explicit TLS - plain connection upgrades to TLS after STARTTLS command |

**When to use SMTPS (`-smtps` flag):**
- Port 465 connections (Gmail, many hosting providers)
- When the server expects immediate TLS handshake
- Legacy "SSL" SMTP configurations

**When to use STARTTLS (`-starttls` flag or default):**
- Port 587 connections (standard submission port)
- Port 25 connections with opportunistic encryption
- Microsoft 365 / Exchange Online

**Common Provider Configurations:**

| Provider | SMTPS (Port 465) | STARTTLS (Port 587) |
|----------|------------------|---------------------|
| Gmail | `smtp.gmail.com -smtps` | `smtp.gmail.com -port 587` |
| Microsoft 365 | Not supported | `smtp.office365.com -port 587` |
| Yahoo | `smtp.mail.yahoo.com -smtps` | `smtp.mail.yahoo.com -port 587` |

```bash
# Gmail with SMTPS (implicit TLS)
smtptool -action testauth -host smtp.gmail.com -smtps -username user@gmail.com -password "app-password"

# Microsoft 365 with STARTTLS (explicit TLS)
smtptool -action testauth -host smtp.office365.com -port 587 -username user@company.com -password "password"
```

### Environment Variables
- **Microsoft Graph Tool**: `MSGRAPH` prefix (e.g., `MSGRAPHTENANTID`, `MSGRAPHSECRET`)
- **SMTP Tool**: `SMTP` prefix (e.g., `SMTPHOST`, `SMTPPORT`, `SMTPUSERNAME`)

## Security Considerations

⚠️ **Important**: These are diagnostic CLI tools designed for authorized personnel (system administrators, IT staff).

- **CLI flags and environment variables are trusted input** from authorized users
- **Not designed for untrusted web/API input** or public-facing services
- **Defense-in-depth measures** implemented in v2.0.2+ (CRLF sanitization, v2.1.0+: password masking)
- **Comprehensive security testing** in v2.1.0 with 100% coverage on critical functions
- **See [SECURITY.md](SECURITY.md)** for complete threat model and deployment guidelines

**Before production use:**
1. Review [SECURITY.md](SECURITY.md) for security assumptions
2. Follow credential management best practices
3. Restrict tool execution to authorized personnel
4. Monitor CSV logs for unauthorized usage

## License
This tool is provided as-is for testing and automation purposes.

                          ..ooOO END OOoo..
