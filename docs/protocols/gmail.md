# Gmail / Google Workspace Protocol — gomailtest

Google Workspace mailbox and calendar operations via the Gmail and Calendar
APIs: send mail, list and export messages, list events, and create invitations.

The enterprise-grade way to reach Gmail/Workspace programmatically is a
**service account with domain-wide delegation (DWD)** that impersonates a
Workspace user. The `gmail` provider follows that model, and also supports a
pre-obtained bearer token and an interactive OAuth flow.

## Quick Start

```powershell
# Service account with domain-wide delegation (impersonates --mailbox)
gomailtest gmail testauth  --credentials sa.json --mailbox user@corp.com
gomailtest gmail sendmail  --credentials sa.json --mailbox user@corp.com --to "recipient@corp.com" --subject "Hi" --body "test"
gomailtest gmail getinbox  --credentials sa.json --mailbox user@corp.com --count 10
```

Environment variables (`GMAIL*`) can replace any flag:

```powershell
$env:GMAILCREDENTIALS = "C:\path\to\sa.json"
$env:GMAILMAILBOX     = "user@corp.com"
gomailtest gmail getinbox --count 5
```

## Setup: service account + domain-wide delegation

1. **Create a GCP project** (or reuse one): <https://console.cloud.google.com/>.
2. **Enable the APIs** in *APIs & Services ▸ Library*: enable **Gmail API** and
   **Google Calendar API**.
3. **Create a service account** in *IAM & Admin ▸ Service Accounts*. Create a
   **JSON key** for it and download it — this is the `--credentials` file. Note
   the service account's **Client ID** (a long number).
4. **Grant domain-wide delegation** in the Google **Admin Console**
   (admin.google.com) ▸ *Security ▸ Access and data control ▸ API controls ▸
   Domain-wide delegation ▸ Add new*. Enter the service account **Client ID**
   and the **exact OAuth scopes** you intend to use, comma-separated, e.g.:

   ```
   https://www.googleapis.com/auth/gmail.send,
   https://www.googleapis.com/auth/gmail.readonly,
   https://www.googleapis.com/auth/calendar.readonly,
   https://www.googleapis.com/auth/calendar.events
   ```

5. **Impersonate a real user**: `--mailbox` must be an actual Workspace user in
   the domain. It becomes the DWD **Subject**.

> **The `Subject`/`--mailbox` is mandatory.** Without it a service account
> authenticates as *itself* (which has no mailbox) and every call fails. The
> provider requires `--mailbox` for the service-account and OAuth methods.

## Authentication methods (pick exactly one)

| Method | Flags | Notes |
|--------|-------|-------|
| Service account + DWD | `--credentials sa.json --mailbox user@corp.com` | Primary, non-interactive. |
| Bearer token | `--bearertoken "ya29..."` | Reuses a token you already obtained; `--mailbox` optional (the token encodes "me"). |
| Interactive OAuth | `--oauth --oauth-credentials client.json [--token-cache path]` | Opens a browser once; caches the token under the OS user-config dir by default. |

## Scopes

Scopes default **per action** (least privilege):

| Action | Default scope |
|--------|---------------|
| `sendmail` | `gmail.send` |
| `getinbox`, `listmail`, `listfolders`, `exportmessages` | `gmail.readonly` |
| `getevents`, `getschedule` | `calendar.readonly` |
| `sendinvite` | `calendar.events` |
| `testauth`, `exportbearertoken` | `gmail.readonly` |

Override with `--scope` (comma-separated). **Every requested scope must be part
of the DWD authorization in the Admin Console**, or Google rejects the token
exchange with `unauthorized_client`.

## Actions

### testauth — Verify authentication and impersonation
```powershell
gomailtest gmail testauth --credentials sa.json --mailbox user@corp.com
```
Calls `users.getProfile` — the cheapest call that proves impersonation and scope
work. Prints the resolved email and message count.

### sendmail — Send an email
```powershell
gomailtest gmail sendmail --credentials sa.json --mailbox user@corp.com \
    --to "a@corp.com,b@corp.com" --cc "c@corp.com" --bcc "d@corp.com" \
    --subject "Test" --body "plain text" --bodyhtml "<p>html</p>" \
    --attachments report.pdf --inline-attachments logo.png --priority high
```
The message is assembled as RFC 5322 (shared with SMTP) and base64url-encoded
into the Gmail `Raw` field. Unlike SMTP, **Bcc is included in the message
headers** because Gmail derives recipients from the headers. With no recipients
specified, `--to` defaults to the mailbox.

### listmail — List recent messages by label
```powershell
gomailtest gmail listmail --credentials sa.json --mailbox user@corp.com \
    --label INBOX --count 10
```
Gmail's list returns IDs only; each message is then fetched with a metadata Get
to resolve Subject/From/Date (a 1+N pattern — mind the quota for large counts).
The deprecated `getinbox` action is equivalent to `listmail --label INBOX`.

### exportmessages — Search and export as .eml
```powershell
gomailtest gmail exportmessages --credentials sa.json --mailbox user@corp.com \
    --messageid "<id@host>" --exportdir C:\exports
gomailtest gmail exportmessages --credentials sa.json --mailbox user@corp.com \
    --subject "invoice" --count 50
gomailtest gmail exportmessages --credentials sa.json --mailbox user@corp.com \
    --search "from:billing@vendor.com newer_than:7d"
```
Searches with the Gmail `rfc822msgid:` and/or `subject:` operators, fetches each
match in `raw` format, base64url-decodes it, and writes a `.eml` file. Requires
`gmail.readonly` (the metadata scope cannot return raw content). `--messageid`
and `--subject` are validated to block search-operator injection; `--search`
passes a raw Gmail query verbatim and overrides both. `searchandexport` is an
alias of this action.

### getevents — List upcoming calendar events
```powershell
gomailtest gmail getevents --credentials sa.json --mailbox user@corp.com --count 5
```

### sendinvite — Create a calendar event and invite attendees
```powershell
gomailtest gmail sendinvite --credentials sa.json --mailbox user@corp.com \
    --subject "Sync" --start 2026-01-15T14:00:00Z --end 2026-01-15T15:00:00Z \
    --to "a@corp.com,b@corp.com"
```
Times accept RFC3339 or the PowerShell sortable format. Invitations are sent to
attendees (`sendUpdates=all`).

### getschedule — Check a recipient's free/busy availability
```powershell
gomailtest gmail getschedule --credentials sa.json --mailbox user@corp.com \
    --to colleague@corp.com
gomailtest gmail getschedule --credentials sa.json --mailbox user@corp.com \
    --to colleague@corp.com --start 2026-08-01T08:00:00Z --end 2026-08-01T18:00:00Z
```
Queries the Calendar `freeBusy` endpoint and prints the recipient's busy blocks
(default window: next 24 hours). Requires `calendar.readonly` and visibility of
the recipient's free/busy information.

### findtimeslot — Search Free Meeting Slots

Searches a recipient's calendar for available meeting slots. Busy data comes from the Calendar `freeBusy` API and slots are computed client-side, constrained to working hours (08:00–17:00 UTC, Monday–Friday). Defaults: 30-minute slots, next 5 working days, first 3 slots.

```powershell
gomailtest gmail findtimeslot --credentials sa.json --mailbox user@corp.com \
    --to colleague@corp.com

# Custom duration and count
gomailtest gmail findtimeslot --credentials sa.json --mailbox user@corp.com \
    --to colleague@corp.com --duration 60 --count 5

# Explicit window
gomailtest gmail findtimeslot --credentials sa.json --mailbox user@corp.com \
    --to colleague@corp.com \
    --start "2026-08-01T08:00:00Z" --end "2026-08-10T17:00:00Z"
```

### exportbearertoken — Print an access token
```powershell
gomailtest gmail exportbearertoken --credentials sa.json --mailbox user@corp.com --output json
```
Acquires a token from the configured credential and prints it (text, or
`{"bearertoken":"..."}` with `--output json`) so it can be reused as
`--bearertoken` elsewhere.

## Troubleshooting

- **`unauthorized_client`** — the service-account Client ID is not authorized for
  the requested scope(s) in the Admin Console, or `--mailbox` is not a real
  Workspace user. Add the exact scopes to the DWD grant and double-check the
  mailbox.
- **`403 / access denied`** — the token's scopes don't cover the operation (e.g.
  using `gmail.metadata` for `exportmessages`, which needs `gmail.readonly`), or
  DWD is not authorized for that mailbox.
- **`429 / rate limit`** — the provider retries throttling and 5xx responses with
  exponential backoff; increase `--retrydelay` or reduce `--count` for large jobs.

## Environment variables

All flags have a `GMAIL*` equivalent, e.g. `GMAILCREDENTIALS`, `GMAILMAILBOX`,
`GMAILBEARERTOKEN`, `GMAILOAUTH`, `GMAILOAUTHCREDENTIALS`, `GMAILTOKENCACHE`,
`GMAILSCOPE`, `GMAILTO`, `GMAILSUBJECT`, `GMAILBODY`, `GMAILMESSAGEID`,
`GMAILEXPORTDIR`, `GMAILCOUNT`, `GMAILPROXY`.
