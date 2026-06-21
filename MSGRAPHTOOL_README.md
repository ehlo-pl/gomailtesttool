# Microsoft Graph Tool (msgraphtool)

A comprehensive command-line tool for interacting with Microsoft Graph API to manage Exchange Online mailboxes - send emails, manage calendar events, and export inbox messages. Part of the **gomailtesttool** suite.

## Overview

The **msgraphtool** provides production-grade Microsoft Graph API operations for Exchange Online:

- **Multiple Authentication Methods**: Client Secret, PFX Certificate, Windows Certificate Store (Thumbprint), Bearer Token
- **Email Operations**: Send emails with attachments, HTML content, and multiple recipients
- **Calendar Management**: Create calendar invites, retrieve events, check availability
- **Inbox Management**: List, export, and search inbox messages
- **Automatic Logging**: CSV/JSON logging to temp directory for audit trails

**Target Use Cases:**
- Exchange Online mailbox automation
- Email sending and notification systems
- Calendar event management
- Inbox monitoring and export
- Microsoft 365 integration testing

## Features

- **7 Comprehensive Actions**:
  - `getevents` - Retrieve calendar events
  - `sendmail` - Send email messages with attachments
  - `sendinvite` - Create calendar invitations
  - `getinbox` - Retrieve inbox messages
  - `getschedule` - Check recipient availability
  - `exportinbox` - Export inbox to JSON files
  - `searchandexport` - Search and export by Message ID

- **No External Dependencies**: Pure Go implementation
- **Cross-Platform**: Windows, Linux, macOS
- **CSV/JSON Logging**: Automatic logging to `%TEMP%\_msgraphtool_{action}_{date}.csv`
- **Shell Completion**: Bash and PowerShell completion scripts

## Installation

### Build from Source

```powershell
# Build msgraphtool only
go build -C cmd/msgraphtool -ldflags="-s -w" -o msgraphtool.exe

# Or build all tools
.\build-all.ps1
```

See [BUILD.md](BUILD.md) for detailed build instructions.

## Quick Start

```powershell
# Set authentication via environment variables
$env:MSGRAPHTENANTID = "your-tenant-id"
$env:MSGRAPHCLIENTID = "your-client-id"
$env:MSGRAPHSECRET = "your-secret"
$env:MSGRAPHMAILBOX = "user@example.com"

# Get calendar events
.\msgraphtool.exe -action getevents

# Send test email
.\msgraphtool.exe -action sendmail -to "recipient@example.com"

# Get inbox messages
.\msgraphtool.exe -action getinbox -count 10
```

## Actions

### 1. getevents - Retrieve Calendar Events

Retrieves upcoming calendar events from the specified mailbox.

```powershell
# Get default 3 upcoming events
.\msgraphtool.exe -action getevents

# Get 10 upcoming events
.\msgraphtool.exe -action getevents -count 10

# With verbose output
.\msgraphtool.exe -action getevents -count 5 -verbose
```

### 2. sendmail - Send Email Messages

Sends email through Microsoft Graph API with support for attachments and HTML content.

```powershell
# Send to self (default)
.\msgraphtool.exe -action sendmail

# Send to specific recipient
.\msgraphtool.exe -action sendmail -to "recipient@example.com"

# Send with custom subject and body
.\msgraphtool.exe -action sendmail \
    -to "recipient@example.com" \
    -subject "Test Email" \
    -body "This is a test message"

# Send to multiple recipients with CC and BCC
.\msgraphtool.exe -action sendmail \
    -to "user1@example.com,user2@example.com" \
    -cc "cc@example.com" \
    -bcc "bcc@example.com" \
    -subject "Team Update"

# Send HTML email
.\msgraphtool.exe -action sendmail \
    -to "recipient@example.com" \
    -subject "HTML Email" \
    -bodyHTML "<h1>Hello</h1><p>This is an <strong>HTML</strong> email.</p>"

# Send with attachments
.\msgraphtool.exe -action sendmail \
    -to "recipient@example.com" \
    -subject "Report Attached" \
    -attachments "C:\Reports\report.pdf,C:\Data\spreadsheet.xlsx"
```

### 3. sendinvite - Create Calendar Invitations

Creates calendar meeting invitations.

```powershell
# Create invite with default subject and time
.\msgraphtool.exe -action sendinvite

# Create invite with custom subject
.\msgraphtool.exe -action sendinvite -subject "Team Meeting"

# Create invite with specific start and end times
.\msgraphtool.exe -action sendinvite \
    -subject "Project Review" \
    -start "2026-01-15T14:00:00Z" \
    -end "2026-01-15T15:00:00Z"
```

### 4. getinbox - Retrieve Inbox Messages

Retrieves recent messages from the inbox.

```powershell
# Get default 3 newest messages
.\msgraphtool.exe -action getinbox

# Get 20 newest messages
.\msgraphtool.exe -action getinbox -count 20

# With verbose output
.\msgraphtool.exe -action getinbox -count 10 -verbose
```

### 5. getschedule - Check Recipient Availability

Checks the availability/free-busy schedule for a recipient.

```powershell
# Check availability for a recipient
.\msgraphtool.exe -action getschedule -to "colleague@example.com"
```

### 6. exportinbox - Export Inbox to JSON

Exports inbox messages to individual JSON files.

```powershell
# Export default 3 newest messages
.\msgraphtool.exe -action exportinbox

# Export 50 messages
.\msgraphtool.exe -action exportinbox -count 50
```

**Output Directory:** `%TEMP%\export\{date}\message_{n}_{timestamp}.json`

### 7. searchandexport - Search by Message ID

Finds and exports a specific email by its Internet Message ID.

```powershell
# Search and export specific message
.\msgraphtool.exe -action searchandexport \
    -messageid "<message-id@example.com>"
```

## Command-Line Flags

### Core Flags

| Flag | Description | Environment Variable | Default |
|------|-------------|---------------------|---------|
| `-action` | Action to perform | `MSGRAPHACTION` | getinbox |
| `-tenantid` | Azure AD Tenant ID (GUID) | `MSGRAPHTENANTID` | - |
| `-clientid` | Application (Client) ID (GUID) | `MSGRAPHCLIENTID` | - |
| `-mailbox` | Target user email address | `MSGRAPHMAILBOX` | - |

### Authentication Flags (mutually exclusive)

| Flag | Description | Environment Variable |
|------|-------------|---------------------|
| `-secret` | Client Secret | `MSGRAPHSECRET` |
| `-pfx` | Path to .pfx certificate file | `MSGRAPHPFX` |
| `-pfxpass` | Password for .pfx certificate | `MSGRAPHPFXPASS` |
| `-thumbprint` | Certificate thumbprint (Windows) | `MSGRAPHTHUMBPRINT` |
| `-bearertoken` | Pre-obtained Bearer token | `MSGRAPHBEARERTOKEN` |

### Email Flags

| Flag | Description | Environment Variable |
|------|-------------|---------------------|
| `-to` | Comma-separated TO recipients | `MSGRAPHTO` |
| `-cc` | Comma-separated CC recipients | `MSGRAPHCC` |
| `-bcc` | Comma-separated BCC recipients | `MSGRAPHBCC` |
| `-subject` | Email subject | `MSGRAPHSUBJECT` |
| `-body` | Email body text | `MSGRAPHBODY` |
| `-bodyHTML` | Email body HTML | `MSGRAPHBODYHTML` |
| `-body-template` | Path to HTML template file | `MSGRAPHBODYTEMPLATE` |
| `-attachments` | Comma-separated file paths | `MSGRAPHATTACHMENTS` |

### Calendar Flags

| Flag | Description | Environment Variable |
|------|-------------|---------------------|
| `-start` | Start time (RFC3339 format) | `MSGRAPHSTART` |
| `-end` | End time (RFC3339 format) | `MSGRAPHEND` |

### Network Flags

| Flag | Description | Environment Variable | Default |
|------|-------------|---------------------|---------|
| `-proxy` | HTTP/HTTPS proxy URL | `MSGRAPHPROXY` | - |
| `-maxretries` | Maximum retry attempts | `MSGRAPHMAXRETRIES` | 3 |
| `-retrydelay` | Retry delay (milliseconds) | `MSGRAPHRETRYDELAY` | 2000 |

### Runtime Flags

| Flag | Description | Environment Variable | Default |
|------|-------------|---------------------|---------|
| `-count` | Number of items to retrieve | `MSGRAPHCOUNT` | 3 |
| `-verbose` | Enable verbose output | - | false |
| `-loglevel` | Log level: DEBUG, INFO, WARN, ERROR | `MSGRAPHLOGLEVEL` | INFO |
| `-output` | Output format: text, json | `MSGRAPHOUTPUT` | text |
| `-logformat` | Log file format: csv, json | `MSGRAPHLOGFORMAT` | csv |
| `-version` | Show version information | - | - |
| `-completion` | Generate completion script (bash/powershell) | - | - |

## Environment Variables

All flags can be set via environment variables with the `MSGRAPH` prefix:

```powershell
# Windows PowerShell
$env:MSGRAPHTENANTID = "tenant-id"
$env:MSGRAPHCLIENTID = "client-id"
$env:MSGRAPHSECRET = "your-secret"
$env:MSGRAPHMAILBOX = "user@example.com"

.\msgraphtool.exe -action sendmail -to "recipient@example.com"

# Linux/macOS Bash
export MSGRAPHTENANTID="tenant-id"
export MSGRAPHCLIENTID="client-id"
export MSGRAPHSECRET="your-secret"
export MSGRAPHMAILBOX="user@example.com"

./msgraphtool -action sendmail -to "recipient@example.com"
```

**Note:** Command-line flags take precedence over environment variables.

## Authentication Methods

### Client Secret

```powershell
.\msgraphtool.exe -action getevents \
    -tenantid "..." -clientid "..." \
    -secret "your-client-secret" \
    -mailbox "user@example.com"
```

### PFX Certificate

```powershell
.\msgraphtool.exe -action getevents \
    -tenantid "..." -clientid "..." \
    -pfx "C:\Certs\app-cert.pfx" \
    -pfxpass "certificate-password" \
    -mailbox "user@example.com"
```

### Windows Certificate Store (Thumbprint)

```powershell
.\msgraphtool.exe -action getevents \
    -tenantid "..." -clientid "..." \
    -thumbprint "CD817B3329802E692CF30D8DDF896FE811B048AB" \
    -mailbox "user@example.com"
```

### Bearer Token

```powershell
.\msgraphtool.exe -action getevents \
    -tenantid "..." -clientid "..." \
    -bearertoken "eyJ0eXAi..." \
    -mailbox "user@example.com"
```

## CSV Logging

All operations are automatically logged to action-specific CSV files:

**Location:** `%TEMP%\_msgraphtool_{action}_{date}.csv`

**Examples:**
- `C:\Users\username\AppData\Local\Temp\_msgraphtool_sendmail_2026-01-31.csv`
- `C:\Users\username\AppData\Local\Temp\_msgraphtool_getevents_2026-01-31.csv`

### CSV Schemas

- **getevents**: Timestamp, Action, Status, Mailbox, Event Subject, Event ID
- **sendmail**: Timestamp, Action, Status, Mailbox, To, CC, BCC, Subject, Body Type, Attachments
- **sendinvite**: Timestamp, Action, Status, Mailbox, Subject, Start Time, End Time, Event ID
- **getinbox**: Timestamp, Action, Status, Mailbox, Subject, From, To, Received DateTime

## Shell Completion

Generate shell completion scripts for enhanced CLI experience:

```powershell
# Generate Bash completion
.\msgraphtool.exe -completion bash > msgraphtool-completion.bash
source msgraphtool-completion.bash

# Generate PowerShell completion
.\msgraphtool.exe -completion powershell > msgraphtool-completion.ps1
. .\msgraphtool-completion.ps1
```

## Retry Configuration

Configure automatic retry for transient failures:

```powershell
# Custom retry settings
.\msgraphtool.exe -action getevents \
    -maxretries 5 \
    -retrydelay 3000

# Disable retries
.\msgraphtool.exe -action sendmail -maxretries 0
```

**Retry Behavior:**
- Exponential backoff: 2s -> 4s -> 8s -> 16s -> 30s (capped)
- Automatic retry on: HTTP 429, 503, 504, network timeouts
- Never retries: Authentication failures, bad requests (400), not found (404)

## Troubleshooting

### Authentication Issues

**"Missing authentication"**
- Provide one of: `-secret`, `-pfx`, `-thumbprint`, or `-bearertoken`

**"Multiple authentication methods provided"**
- Use only one authentication method at a time

**"Invalid Tenant ID / Client ID"**
- Ensure GUID format: `xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx`

### Permission Issues

**"Insufficient privileges"**
- Verify Azure AD app has required Graph API permissions
- For sendmail: `Mail.Send`
- For calendar: `Calendars.ReadWrite`
- For inbox: `Mail.Read`

### Rate Limiting

**HTTP 429 Too Many Requests**
- Tool automatically retries with backoff
- Increase `-retrydelay` for persistent issues
- Reduce request frequency

## Security Best Practices

1. **Credential Management**:
   - Use environment variables for secrets
   - Never commit credentials to version control
   - Use certificate authentication in production

2. **Logging**:
   - CSV logs may contain email addresses
   - Secure log files appropriately
   - Implement log rotation for production use

3. **Access Control**:
   - Use least-privilege Azure AD permissions
   - Restrict tool execution to authorized users
   - Monitor Graph API audit logs

## Related Documentation

- **Build Instructions**: [BUILD.md](BUILD.md)
- **Usage Examples**: [EXAMPLES.md](EXAMPLES.md)
- **SMTP Tool**: [SMTP_TOOL_README.md](SMTP_TOOL_README.md)
- **Security Policy**: [SECURITY.md](SECURITY.md)
- **Troubleshooting**: [TROUBLESHOOTING.md](TROUBLESHOOTING.md)

## Support

**Repository:** [https://github.com/ehlo-pl/gomailtesttool](https://github.com/ehlo-pl/gomailtesttool)

                          ..ooOO END OOoo..
