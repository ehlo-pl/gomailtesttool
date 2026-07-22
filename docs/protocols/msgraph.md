# Microsoft Graph Protocol — gomailtest

Exchange Online mailbox operations via Microsoft Graph API: send emails, manage calendar events, export inbox.

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

# Send from a template file (Go text/template variables via --template-vars)
gomailtest msgraph sendmail --template "C:\Templates\message.eml" --template-vars Name=World
gomailtest msgraph sendmail --to "recipient@example.com" \
    --template "C:\Templates\body.html" --template-vars Name=World
```

> **Note:** Microsoft Graph only passes through `X-`-prefixed custom headers. Standard RFC headers (From, To, Subject, Date, Message-ID, etc.) are set by the Graph API itself and cannot be overridden via `--header`.

`--template` with a `.eml` file parses the rendered message and maps its
recognized fields (`To`/`Cc`/`Bcc`/`Subject`/text and HTML bodies) onto the
Graph send API; recipient flags win over the EML headers when both are given.
The EML `From` header is ignored (Graph always sends as `--mailbox`) and
headers without a Graph mapping are reported in verbose mode. Any other
extension is rendered and used as the HTML body. Mutually exclusive with
`--body`/`--bodyHTML`; `--template-vars key=value`
(repeatable) supplies variables referenced as `{{.key}}`.

### draft — Save an Email as a Draft (does not send)

Builds the same message as `sendmail` but POSTs it to `/users/{mailbox}/messages`,
which stores it in the mailbox's **Drafts** folder without sending. Accepts the
same flags as `sendmail` except `--save-to-sent` (a draft is never sent).

```powershell
gomailtest msgraph draft --to "recipient@example.com" --subject "Test" --body "draft body"
```

> **Note:** Draft creation requires the **`Mail.ReadWrite`** application permission.
> The send-only `Mail.Send` permission is not sufficient.

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

### getinbox — Retrieve Inbox Messages (deprecated)

Deprecated: use `listmail --folder inbox`. Still works and delegates to the
same folder listing.

```powershell
gomailtest msgraph listmail --folder inbox --count 20
```

### getschedule — Check Recipient Availability

```powershell
gomailtest msgraph getschedule --to "colleague@example.com"
```

### findtimeslot — Search Free Meeting Slots

Searches a recipient's calendar for available meeting slots. Busy data comes from the Graph `getSchedule` API and slots are computed client-side, constrained to working hours (08:00–17:00 UTC, Monday–Friday). Defaults: 30-minute slots, next 5 working days, first 3 slots.

```powershell
gomailtest msgraph findtimeslot --to "colleague@example.com"

# Custom duration and count
gomailtest msgraph findtimeslot --to "colleague@example.com" \
    --duration 60 --count 5

# Explicit window
gomailtest msgraph findtimeslot --to "colleague@example.com" \
    --start "2026-08-01T08:00:00Z" --end "2026-08-10T17:00:00Z"
```

### exportinbox — Export Inbox to JSON (deprecated)

Deprecated: use `exportmessages --folder inbox` (note: exportinbox writes JSON,
exportmessages writes `.eml`).

```powershell
gomailtest msgraph exportinbox --count 50
```

Output goes to `%TEMP%\export\{date}\message_{n}_{timestamp}.json`.

### searchandexport — Search by Message ID (deprecated)

Deprecated: use `exportmessages --messageid` (note: searchandexport writes
JSON, exportmessages writes `.eml`).

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

# Scope the search to a folder, or export the newest N messages of a folder
gomailtest msgraph exportmessages --folder inbox --count 10
gomailtest msgraph exportmessages --folder sentitems --subject "Invoice"
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

### testconnect — Probe endpoint reachability

Unauthenticated HTTP/TLS probe against `https://graph.microsoft.com`. No
credentials, tenant/client IDs, or mailbox are required — any HTTP response
(401 is expected) confirms the endpoint is reachable. Because Graph is a global
service, this cannot verify authentication or permissions; its value is catching
proxy/firewall blocks and TLS interception. An unexpected certificate **Issuer**
in the output indicates a TLS-intercepting proxy.

```powershell
gomailtest msgraph testconnect
gomailtest msgraph testconnect --verbose
gomailtest msgraph testconnect --proxy "http://proxy.example.com:8080"
gomailtest msgraph testconnect --output json
```

### testauth — Verify credentials

Acquires an access token from Microsoft Entra ID using the configured credential.
Successful token acquisition is the authentication verdict; the token's claims
(application name, assigned roles) are displayed so you can see the granted
application permissions. No mailbox is required.

If `--mailbox` is supplied, one lightweight authenticated Graph call
(`GET /users/{mailbox}`) is made as end-to-end verification: a `403` there is
reported as **success** (the token was accepted; the app simply lacks
`User.Read.All`), while a `401` is a genuine authentication failure.

> **Note on `--bearertoken`:** a pre-obtained token is used as-is and is **not**
> validated by acquisition (Entra ID is not contacted), so `testauth` cannot
> confirm it on its own. Pass `--mailbox` to verify a bearer token end to end;
> otherwise the output reports it as accepted-locally-but-not-verified.

```powershell
gomailtest msgraph testauth --tenantid "..." --clientid "..." --secret "..."
gomailtest msgraph testauth --bearertoken "..."
gomailtest msgraph testauth --tenantid "..." --clientid "..." --secret "..." --mailbox "user@example.com"
gomailtest msgraph testauth --tenantid "..." --clientid "..." --secret "..." --output json
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
| ~~`--delegated`~~ | **Deprecated.** Enable delegated auth flow. | `MSGRAPHDELEGATED` | false |
| ~~`--authflow`~~ | **Deprecated.** Delegated auth flow: `devicecode` or `browser`. | `MSGRAPHAUTHFLOW` | devicecode |
| ~~`--redirecturl`~~ | **Deprecated.** Redirect URL for browser delegated flow. | `MSGRAPHREDIRECTURL` | — |
| ~~`--scope`~~ | **Deprecated.** Comma-separated delegated scopes. | `MSGRAPHSCOPE` | — |
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
| `--template` | Message template file with Go `text/template` variables: `.eml` fields are mapped to the Graph API, any other extension is used as the HTML body; mutually exclusive with `--body`/`--bodyHTML` | `MSGRAPHTEMPLATE` |
| `--template-vars` | Template variable in `key=value` form, referenced as `{{.key}}` in `--template` (repeatable) | `MSGRAPHTEMPLATEVARS` |
| `--attachments` | Comma-separated file paths | `MSGRAPHATTACHMENTS` |
| `--inline-attachments` | Comma-separated file paths to embed inline via `cid:<filename>` (referenced from `--bodyHTML`) | `MSGRAPHINLINEATTACHMENTS` |
| `--header` | Custom header in `"Name: Value"` form (repeatable); when set via env var use comma-separated values (avoid commas in header values). **Microsoft Graph only passes through `X-`-prefixed custom headers** — standard RFC headers (From, To, Subject, etc.) are controlled by the API itself and cannot be injected here. Note: several X-item (on the same names) will end with only first one - it's MAPI limitation.  | `MSGRAPHHEADER` |
| `--priority` | Email priority/importance: `high`, `normal`, `low` (maps to the Graph `importance` field) | `MSGRAPHPRIORITY` |
| `--start` | Start time (RFC3339) | `MSGRAPHSTART` |
| `--end` | End time (RFC3339) | `MSGRAPHEND` |
| `--messageid` | Internet Message ID | `MSGRAPHMESSAGEID` |
| `--exportdir` | Directory under which to create the dated export folder (used by `exportinbox`, `searchandexport`, `exportmessages`) | `MSGRAPHEXPORTDIR` |

### exportmessages-specific

| Flag | Description | Environment Variable | Default |
|------|-------------|---------------------|---------|
| `--messageid` | Internet Message-ID to search for | `MSGRAPHMESSAGEID` | — |
| `--subject` | Subject substring to search for (OData `contains()`) | `MSGRAPHSUBJECT` | — |
| `--count` | Maximum number of matching messages to export | `MSGRAPHCOUNT` | 25 |

## Authentication Methods

Microsoft Graph supports two authentication models. Pick the one that matches how
the tool should run, then provide exactly one credential within it (credentials are
mutually exclusive):

- **Application (app-only) mode** — the app authenticates as itself for unattended
  automation. See below.
- **Delegated mode** — the app acts on behalf of an interactively signed-in user.
  See [Delegated mode](#delegated-mode).

### Application (app-only) mode

The application authenticates as itself via the OAuth2 **client-credentials** flow —
no user signs in. This requires **application permissions** granted with tenant admin
consent (see [Required Azure AD Permissions](#required-azure-ad-permissions)), and
`--mailbox` selects the mailbox to operate on. Best for scheduled jobs, monitoring,
and service scenarios. Provide exactly one of the credentials below.

#### Client Secret

```powershell
gomailtest msgraph getevents \
    --tenantid "..." --clientid "..." \
    --secret "your-client-secret" \
    --mailbox "user@example.com"
```

#### PFX Certificate

```powershell
gomailtest msgraph getevents \
    --tenantid "..." --clientid "..." \
    --pfx "C:\Certs\app-cert.pfx" --pfxpass "certificate-password" \
    --mailbox "user@example.com"
```

#### Windows Certificate Store (Thumbprint)

```powershell
gomailtest msgraph getevents \
    --tenantid "..." --clientid "..." \
    --thumbprint "CD817B3329802E692CF30D8DDF896FE811B048AB" \
    --mailbox "user@example.com"
```

### Delegated mode *(deprecated)*

> **Deprecated.** Delegated mode and its flags (`--delegated`, `--authflow`, `--redirecturl`, `--scope`) are deprecated and hidden from interactive help. They remain functional for backward compatibility. for such tests uer i.e. [github.com/ziembor/msgraph-js-sandbox](https://github.com/ziembor/msgraph-js-sandbox)

The application acts **on behalf of a signed-in user** via an interactive sign-in.
This requires **delegated permissions** consented by the user (or an admin);
operations run in that user's context, so app-only application permissions do not
apply.

#### Delegated Permissions (Device Code)

```powershell
gomailtest msgraph getinbox \
    --tenantid "..." --clientid "..." \
    --authflow devicecode \
    --mailbox "user@example.com"
```

#### Delegated Permissions (Browser + Redirect URL)

```powershell
gomailtest msgraph sendmail \
    --tenantid "..." --clientid "..." \
    --authflow browser \
    --redirecturl "http://localhost:8400/callback" \
    --mailbox "user@example.com" \
    --to "recipient@example.com"
```

### Bearer Token (either mode)

A pre-obtained access token is used as-is — whichever model it was minted for
(app-only or delegated) is already encoded in the token, and Entra ID is not
contacted to acquire it. Works with any action; pass `--mailbox` where the action
needs a target mailbox.

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
| `getschedule` | Timestamp, Action, Status, Mailbox, Recipient, Check DateTime, Availability |
| `exportinbox` | Timestamp, Action, Status, Mailbox, Detail, Export Dir |
| `searchandexport` | Timestamp, Action, Status, Mailbox, Detail, Message ID |
| `exportmessages` | Timestamp, Action, Status, Mailbox, Detail, Message ID, Filename |
| `testconnect` | Action, Status, Endpoint, HTTP Status, HTTP Status Code, TLS Version, Cipher Suite, Cert Subject, Cert Issuer, Cert SANs, Cert Valid From, Cert Valid To, Response Time (ms), Error |
| `testauth` | Action, Status, App Name, Assigned Roles, Token Expiry, Mailbox Check, Error |

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
| draft | `Mail.ReadWrite` |
| getevents, sendinvite | `Calendars.ReadWrite` |
| getinbox, exportinbox, searchandexport, exportmessages | `Mail.Read` |
| getschedule | `Calendars.Read` |
| testconnect | None (unauthenticated network probe) |
| testauth | None for the core token check; the optional `--mailbox` probe uses `User.Read.All` |
| exportbearertoken | None (token acquisition only) |

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
