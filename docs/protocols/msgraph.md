# Microsoft Graph Protocol — gomailtest

Exchange Online mailbox operations via Microsoft Graph API: send emails, manage calendar events, export inbox.

> **Legacy name:** `msgraphtool`. Use `gomailtest msgraph <action> --flag` (see the migration table in README.md).

## Quick Start

```powershell
# Set authentication via environment variables
$env:MSGRAPHTENANTID = "your-tenant-id"
$env:MSGRAPHCLIENTID = "your-client-id"
$env:MSGRAPHSECRET = "your-secret"
$env:MSGRAPHMAILBOX = "user@example.com"

# Get calendar events
gomailtest msgraph getevents

# Send test email
gomailtest msgraph sendmail --to "recipient@example.com"

# Get inbox messages
gomailtest msgraph getinbox --count 10
```

## Actions

### getevents — Retrieve Calendar Events

```powershell
gomailtest msgraph getevents
gomailtest msgraph getevents --count 10
gomailtest msgraph getevents --count 5 --verbose
```

### sendmail — Send Email Messages

```powershell
# Send to self (default)
gomailtest msgraph sendmail

# Send to specific recipient
gomailtest msgraph sendmail --to "recipient@example.com"

# Custom subject and body
gomailtest msgraph sendmail \
    --to "recipient@example.com" \
    --subject "Test Email" \
    --body "This is a test message"

# Multiple recipients with CC and BCC
gomailtest msgraph sendmail \
    --to "user1@example.com,user2@example.com" \
    --cc "cc@example.com" \
    --bcc "bcc@example.com" \
    --subject "Team Update"

# HTML email
gomailtest msgraph sendmail \
    --to "recipient@example.com" \
    --subject "HTML Email" \
    --bodyHTML "<h1>Hello</h1><p>This is an <strong>HTML</strong> email.</p>"

# Both text and HTML (multipart MIME)
gomailtest msgraph sendmail \
    --to "recipient@example.com" \
    --subject "Multipart Email" \
    --body "This is the plain text version" \
    --bodyHTML "<h1>HTML Version</h1><p>This is the <em>HTML</em> version</p>"

# With attachments
gomailtest msgraph sendmail \
    --to "recipient@example.com" \
    --subject "Report Attached" \
    --attachments "C:\Reports\report.pdf,C:\Data\spreadsheet.xlsx"

# Inline image referenced from HTML body, plus a custom header
gomailtest msgraph sendmail \
    --to "recipient@example.com" \
    --subject "Inline Image" \
    --bodyHTML '<p>Hello! Here is our logo: <img src="cid:logo.png"></p>' \
    --inline-attachments "C:\Branding\logo.png" \
    --header "X-Custom-Header: example-value"
```

> **Note:** Microsoft Graph only passes through `X-`-prefixed custom headers. Standard RFC headers (From, To, Subject, Date, Message-ID, etc.) are set by the Graph API itself and cannot be overridden via `--header`.

### sendinvite — Create Calendar Invitations

```powershell
gomailtest msgraph sendinvite --subject "Team Meeting"

gomailtest msgraph sendinvite \
    --subject "Project Review" \
    --start "2026-01-15T14:00:00Z" \
    --end "2026-01-15T15:00:00Z"

# All-day event
gomailtest msgraph sendinvite \
    --subject "Conference Day" \
    --start "2026-02-01T00:00:00Z" \
    --end "2026-02-02T00:00:00Z"
```

### getinbox — Retrieve Inbox Messages

```powershell
gomailtest msgraph getinbox
gomailtest msgraph getinbox --count 20
```

### getschedule — Check Recipient Availability

```powershell
gomailtest msgraph getschedule --to "colleague@example.com"
```

### exportinbox — Export Inbox to JSON

```powershell
gomailtest msgraph exportinbox --count 50
```

Output goes to `%TEMP%\export\{date}\message_{n}_{timestamp}.json`.

### searchandexport — Search by Message ID

```powershell
gomailtest msgraph searchandexport --messageid "<message-id@example.com>"
```

If the query returns no matches, the tool retries with exponential backoff
(governed by `--maxretries` / `--retrydelay`) because Graph is eventually
consistent — a just-sent message may not be indexed yet. "No message found"
is reported only after all retries are exhausted (~14 s with defaults).

### exportmessages — Export Matching Messages as .eml

Searches messages by Internet Message-ID and/or a subject substring (OData
`contains()`), then downloads each match's raw RFC822 content as a `.eml`
file.

```powershell
gomailtest msgraph exportmessages --subject "Invoice"
gomailtest msgraph exportmessages --messageid "<message-id@example.com>"
gomailtest msgraph exportmessages --subject "Invoice" --count 10
gomailtest msgraph exportmessages --subject "Invoice" --exportdir "C:\exports"
```

Output goes to `%TEMP%\export\{date}\msg_{id}.eml`, or
`<exportdir>\{date}\msg_{id}.eml` when `--exportdir` is given.

Like `searchandexport`, an empty result is retried with exponential backoff
(`--maxretries` / `--retrydelay`) to absorb Graph's eventual-consistency
indexing delay; "No messages found" is reported only after the last attempt.

### exportbearertoken — Print access token

Acquires a Microsoft Graph bearer token using configured auth (`--secret`,
`--pfx`, `--thumbprint`) and prints it. If `--bearertoken` is already
provided, this command prints that value directly.

```powershell
# Plain token string (default)
gomailtest msgraph exportbearertoken --tenantid "..." --clientid "..." --secret "..."

# JSON output
gomailtest msgraph exportbearertoken --tenantid "..." --clientid "..." --secret "..." --output json
```
 

## Flags

### Persistent (all subcommands)

| Flag | Description | Environment Variable | Default |
|------|-------------|---------------------|---------|
| `--tenantid` | Azure AD Tenant ID (GUID) | `MSGRAPHTENANTID` | — |
| `--clientid` | Application (Client) ID (GUID) | `MSGRAPHCLIENTID` | — |
| `--mailbox` | Target user email address | `MSGRAPHMAILBOX` | — |
| `--secret` | Client Secret | `MSGRAPHSECRET` | — |
| `--pfx` | Path to .pfx certificate file | `MSGRAPHPFX` | — |
| `--pfxpass` | Password for .pfx certificate | `MSGRAPHPFXPASS` | — |
| `--thumbprint` | Certificate thumbprint (Windows only) | `MSGRAPHTHUMBPRINT` | — |
| `--bearertoken` | Pre-obtained Bearer token | `MSGRAPHBEARERTOKEN` | — |
| `--proxy` | HTTP/HTTPS proxy URL | `MSGRAPHPROXY` | — |
| `--maxretries` | Maximum retry attempts | `MSGRAPHMAXRETRIES` | 3 |
| `--retrydelay` | Retry delay (milliseconds) | `MSGRAPHRETRYDELAY` | 2000 |
| `--count` | Number of items to retrieve | `MSGRAPHCOUNT` | 3 |
| `--verbose` | Enable verbose output | — | false |
| `--loglevel` | Log level: DEBUG, INFO, WARN, ERROR | `MSGRAPHLOGLEVEL` | INFO |
| `--logformat` | Log file format: csv, json | `MSGRAPHLOGFORMAT` | csv |

### Email flags (sendmail / sendinvite)

| Flag | Description | Environment Variable |
|------|-------------|---------------------|
| `--to` | Comma-separated TO recipients | `MSGRAPHTO` |
| `--cc` | Comma-separated CC recipients | `MSGRAPHCC` |
| `--bcc` | Comma-separated BCC recipients | `MSGRAPHBCC` |
| `--subject` | Email subject | `MSGRAPHSUBJECT` |
| `--body` | Email body text | `MSGRAPHBODY` |
| `--bodyHTML` | Email body HTML | `MSGRAPHBODYHTML` |
| `--body-template` | Path to HTML template file | `MSGRAPHBODYTEMPLATE` |
| `--attachments` | Comma-separated file paths | `MSGRAPHATTACHMENTS` |
| `--inline-attachments` | Comma-separated file paths to embed inline via `cid:<filename>` (referenced from `--bodyHTML`) | `MSGRAPHINLINEATTACHMENTS` |
| `--header` | Custom header in `"Name: Value"` form (repeatable); when set via env var use comma-separated values (avoid commas in header values). **Microsoft Graph only passes through `X-`-prefixed custom headers** — standard RFC headers (From, To, Subject, etc.) are controlled by the API itself and cannot be injected here | `MSGRAPHHEADER` |
| `--priority` | Email priority/importance: `high`, `normal`, `low` (maps to the Graph `importance` field) | `MSGRAPHPRIORITY` |
| `--start` | Start time (RFC3339) | `MSGRAPHSTART` |
| `--end` | End time (RFC3339) | `MSGRAPHEND` |
| `--messageid` | Internet Message ID | `MSGRAPHMESSAGEID` |
| `--exportdir` | Directory under which to create the dated export folder (used by `exportinbox`, `searchandexport`, `exportmessages`) | `MSGRAPHEXPORTDIR` | OS temp dir |

### exportmessages-specific

| Flag | Description | Environment Variable | Default |
|------|-------------|---------------------|---------|
| `--messageid` | Internet Message-ID to search for | `MSGRAPHMESSAGEID` | — |
| `--subject` | Subject substring to search for (OData `contains()`) | `MSGRAPHSUBJECT` | — |
| `--count` | Maximum number of matching messages to export | `MSGRAPHCOUNT` | 25 |

## Authentication Methods

Provide exactly one method. Mutually exclusive:

### Client Secret

```powershell
gomailtest msgraph getevents \
    --tenantid "..." --clientid "..." \
    --secret "your-client-secret" \
    --mailbox "user@example.com"
```

### PFX Certificate

```powershell
gomailtest msgraph getevents \
    --tenantid "..." --clientid "..." \
    --pfx "C:\Certs\app-cert.pfx" --pfxpass "certificate-password" \
    --mailbox "user@example.com"
```

### Windows Certificate Store (Thumbprint)

```powershell
gomailtest msgraph getevents \
    --tenantid "..." --clientid "..." \
    --thumbprint "CD817B3329802E692CF30D8DDF896FE811B048AB" \
    --mailbox "user@example.com"
```

### Bearer Token

```powershell
gomailtest msgraph getevents \
    --tenantid "..." --clientid "..." \
    --bearertoken "eyJ0eXAi..." \
    --mailbox "user@example.com"
```

## Environment Variables

```powershell
$env:MSGRAPHTENANTID = "tenant-id"
$env:MSGRAPHCLIENTID = "client-id"
$env:MSGRAPHSECRET = "your-secret"
$env:MSGRAPHMAILBOX = "user@example.com"

gomailtest msgraph sendmail --to "recipient@example.com"
```

## Proxy Usage

```powershell
# Specify proxy on command line
gomailtest msgraph sendmail \
    --to "user@example.com" \
    --proxy "http://proxy.company.com:8080"

# Proxy via environment variable
$env:MSGRAPHPROXY = "http://proxy.company.com:8080"
gomailtest msgraph getevents
```

## Advanced Examples

### HTML Report with Multiple Attachments

```powershell
gomailtest msgraph sendmail \
    --to "team-lead@example.com,manager@example.com" \
    --cc "team@example.com" \
    --subject "Q1 2026 Performance Report" \
    --bodyHTML "<h1>Q1 Performance Report</h1><p>See attached metrics and analysis.</p>" \
    --attachments "C:\Reports\Q1-Metrics.xlsx,C:\Reports\Q1-Analysis.pdf" \
    --verbose
```

### Automated Monitoring Script

```powershell
# Log inbox and calendar to files
gomailtest msgraph getinbox --count 50 | Out-File -Append "C:\Logs\inbox-monitor.log"
gomailtest msgraph getevents --count 20 | Out-File -Append "C:\Logs\calendar-monitor.log"
```

## CSV Logging

Operations are logged to `%TEMP%\_msgraphtool_{action}_{date}.csv`.

### Log Schemas

| Action | Columns |
|--------|---------|
| `getevents` | Timestamp, Action, Status, Mailbox, Event Subject, Event ID |
| `sendmail` | Timestamp, Action, Status, Mailbox, To, CC, BCC, Subject, Body Type, Attachments |
| `sendinvite` | Timestamp, Action, Status, Mailbox, Subject, Start Time, End Time, Event ID |
| `getinbox` | Timestamp, Action, Status, Mailbox, Subject, From, To, Received DateTime |

## Retry Configuration

```powershell
# Custom retry settings
gomailtest msgraph getevents --maxretries 5 --retrydelay 3000

# Disable retries
gomailtest msgraph sendmail --maxretries 0
```

Retry uses exponential backoff: 2s → 4s → 8s → 16s → 30s (capped). Retries on HTTP 429, 503, 504, network timeouts. Never retries authentication failures or 4xx errors.

For `searchandexport` and `exportmessages`, a successful-but-empty result is also retried with the same backoff, because Graph is eventually consistent and a just-sent message may not be indexed yet. Only after the last attempt does the tool report that no messages were found.

## Required Azure AD Permissions

| Action | Permission |
|--------|-----------|
| sendmail | `Mail.Send` |
| getevents, sendinvite | `Calendars.ReadWrite` |
| getinbox, exportinbox, searchandexport, exportmessages | `Mail.Read` |
| getschedule | `Calendars.Read` |

## Tips and Best Practices

1. **Security**: Use environment variables for sensitive data — avoid passing secrets as CLI flags (visible in process list)
2. **Verbose Mode**: Use `--verbose` when troubleshooting authentication or API issues
3. **CSV Logs**: Check CSV log files for historical records of all operations
4. **Graceful Shutdown**: Press Ctrl+C to interrupt long-running operations safely (CSV logger closes cleanly)
5. **Flag Precedence**: Command-line flags override environment variables
6. **Comma Separation**: Lists (`--to`, `--cc`, `--bcc`, `--attachments`) use comma-separation; spaces are trimmed
7. **Time Format**: Calendar times use RFC3339 format (e.g., `2026-01-15T14:00:00Z`)

## Related Documentation

- [BUILD.md](../../BUILD.md) — Build instructions
- [TROUBLESHOOTING.md](../../TROUBLESHOOTING.md) — Common issues
- [SECURITY.md](../../SECURITY.md) — Security policy

                          ..ooOO END OOoo..
