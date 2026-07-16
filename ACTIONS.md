# Implemented Actions per Protocol

Quick reference for all CLI actions available across supported mail/calendar protocols.

| Action              | SMTP | IMAP | POP3 | JMAP | EWS | msgraph | Gmail |
|---------------------|:----:|:----:|:----:|:----:|:---:|:-------:|:-----:|
| `testconnect`       | ✓    | ✓    | ✓    | ✓    | ✓   | ✓       | ✓     |
| `testauth`          | ✓    | ✓    | ✓    | ✓    | ✓   | ✓       | ✓     |
| `teststarttls`      | ✓    | ✓    | ✓    | —    | —   | —       | —     |
| `sendmail`          | ✓    | —    | —    | ✓    | ✓   | ✓       | ✓     |
| `listfolders`       | —    | ✓    | —    | ✓    | ✓   | ✓       | ✓     |
| `listmail`          | —    | ✓    | ✓    | ✓    | ✓   | ✓       | ✓     |
| `exportmessages`    | —    | ✓    | ✓    | ✓    | ✓   | ✓       | ✓     |
| `getevents`         | —    | —    | —    | —    | ✓   | ✓       | ✓     |
| `sendinvite`        | —    | —    | —    | —    | ✓   | ✓       | ✓     |
| `getschedule`       | —    | —    | —    | —    | ✓   | ✓       | ✓     |
| `findtimeslot`      | —    | —    | —    | —    | ✓   | ✓       | ✓     |
| `exportbearertoken` | —    | —    | —    | —    | —   | ✓       | ✓     |
| `getfolder`         | —    | —    | —    | —    | ✓   | —       | —     |
| `autodiscover`      | —    | —    | —    | —    | ✓   | —       | —     |
| `getmailboxes`      | —    | —    | —    | A    | —   | —       | —     |
| `getinbox`          | —    | —    | —    | —    | —   | D       | D     |
| `exportinbox`       | —    | —    | —    | —    | —   | D       | —     |
| `searchandexport`   | —    | —    | —    | —    | —   | D       | A     |

Legend: ✓ implemented · A alias of a canonical action · D deprecated (still works)

## Aliases and deprecations

| Old invocation              | Canonical replacement                        | Notes |
|-----------------------------|----------------------------------------------|-------|
| `jmap getmailboxes`         | `jmap listfolders`                           | Cobra alias, identical behavior |
| `gmail searchandexport`     | `gmail exportmessages --search "..."`        | Cobra alias of exportmessages |
| `msgraph getinbox`          | `msgraph listmail --folder inbox`            | Deprecated; delegates to listmail |
| `gmail getinbox`            | `gmail listmail --label INBOX`               | Deprecated; delegates to listmail |
| `msgraph exportinbox`       | `msgraph exportmessages --folder inbox`      | Deprecated; note exportinbox writes JSON, exportmessages writes .eml |
| `msgraph searchandexport`   | `msgraph exportmessages --messageid "..."`   | Deprecated; note searchandexport writes JSON, exportmessages writes .eml |

Related flags: `msgraph exportmessages` accepts `--folder` (scope search / export newest N of a folder); `gmail exportmessages` accepts `--search` (raw Gmail query, overrides `--messageid`/`--subject`).

## findtimeslot

Searches a **specific other user's** calendar (`--to`, required) for free meeting slots. Busy data comes from the protocol's free/busy API (EWS GetUserAvailability, msgraph getSchedule, Gmail Calendar freeBusy) and free slots are computed client-side — identical semantics on every protocol, and it works with app-only authentication.

Defaults: 30-minute slots (`--duration`), window = now → +5 working days (`--start`/`--end`), first 3 slots (`--count`), constrained to working hours 08:00–17:00 UTC, Monday–Friday.

JMAP is not supported: JMAP (RFC 8620) has no availability/free-busy API, and the JMAP Calendars extension is still a draft without server support.

## Authentication methods

| Protocol  | Auth methods                                                                 |
|-----------|------------------------------------------------------------------------------|
| SMTP      | PLAIN, LOGIN, CRAM-MD5, XOAUTH2, NTLM, GSSAPI                               |
| IMAP      | PLAIN, LOGIN, XOAUTH2                                                        |
| POP3      | USER/PASS, APOP, XOAUTH2                                                     |
| JMAP      | Bearer token, Basic                                                          |
| EWS       | NTLM, Basic, Bearer (OAuth2)                                                 |
| msgraph   | Client secret, PFX certificate, Windows cert store thumbprint, bearer token, device code / browser (delegated) |
| Gmail     | Service account with domain-wide delegation, bearer token, interactive OAuth (loopback) |
