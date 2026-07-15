# Implemented Actions per Protocol

Quick reference for all CLI actions available across supported mail/calendar protocols.

| Action              | SMTP | IMAP | POP3 | JMAP | EWS | msgraph | Gmail |
|---------------------|:----:|:----:|:----:|:----:|:---:|:-------:|:-----:|
| `testconnect`       | ✓    | ✓    | ✓    | ✓    | ✓   | ✓       | ✓     |
| `testauth`          | ✓    | ✓    | ✓    | ✓    | ✓   | ✓       | ✓     |
| `sendmail`          | ✓    | —    | —    | ✓    | ✓   | ✓       | ✓     |
| `getinbox`          | —    | —    | —    | —    | —   | ✓       | ✓     |
| `exportmessages`    | —    | ✓    | ✓    | ✓    | ✓   | ✓       | ✓     |
| `getevents`         | —    | —    | —    | —    | —   | ✓       | ✓     |
| `sendinvite`        | —    | —    | —    | —    | —   | ✓       | ✓     |
| `exportbearertoken` | —    | —    | —    | —    | —   | ✓       | ✓     |
| `teststarttls`      | ✓    | —    | —    | —    | —   | —       | —     |
| `listfolders`       | —    | ✓    | —    | ✓    | ✓   | ✓       | ✓     |
| `listmail`          | —    | —    | ✓    | ✓    | ✓   | ✓       | ✓     |
| `getmailboxes`      | —    | —    | —    | ✓    | —   | —       | —     |
| `getfolder`         | —    | —    | —    | —    | ✓   | —       | —     |
| `autodiscover`      | —    | —    | —    | —    | ✓   | —       | —     |
| `getschedule`       | —    | —    | —    | —    | —   | ✓       | —     |
| `exportinbox`       | —    | —    | —    | —    | —   | ✓       | —     |
| `searchandexport`   | —    | —    | —    | —    | —   | ✓       | —     |

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
