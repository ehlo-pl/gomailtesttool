# Serve Mode — gomailtest

HTTP REST server for sending emails programmatically via API calls.

Credentials are loaded from environment variables at startup. Each HTTP request carries only message content — no credentials in request bodies.

## Quick Start

```bash
# Start the server (SERVE_API_KEY is required)
export SERVE_API_KEY="mysecretkey"
export SMTPHOST="smtp.example.com"
export SMTPPORT="587"
export SMTPUSERNAME="user@example.com"
export SMTPPASSWORD="yourpassword"
export SMTPFROM="sender@example.com"

gomailtest serve --api-key mysecretkey

# Send an email via the REST API
curl -X POST http://localhost:8080/smtp/sendmail \
  -H "X-API-Key: mysecretkey" \
  -H "Content-Type: application/json" \
  -d '{"to":["recipient@example.com"],"subject":"Test","body":"Hello from serve mode"}'
```

## Starting the Server

```bash
gomailtest serve --api-key <secret> [--port 8080] [--listen 127.0.0.1]
```

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--api-key` | `SERVE_API_KEY` | — | **Required.** Value expected in the `X-API-Key` request header |
| `--port` | `SERVE_PORT` | `8080` | HTTP listen port |
| `--listen` | `SERVE_LISTEN` | `127.0.0.1` | Bind address (e.g., `0.0.0.0` to listen on all interfaces) |

## Authentication

All endpoints except `GET /health` and `GET /` require the `X-API-Key` header:

```
X-API-Key: <value set via --api-key or SERVE_API_KEY>
```

Missing or incorrect keys return `401 Unauthorized`:

```json
{"status":"error","message":"missing or invalid X-API-Key"}
```

## Endpoints

| Method | Path | Auth required | Description |
|--------|------|---------------|-------------|
| `GET` | `/health` | No | Health check |
| `GET` | `/` | No | Lists available endpoints and their availability |
| `POST` | `/smtp/sendmail` | Yes | Send email via SMTP |
| `POST` | `/msgraph/sendmail` | Yes | Send email via Microsoft Graph |
| `POST` | `/ews/sendmail` | Yes | Not yet implemented (returns 501) |

### GET /health

```bash
curl http://localhost:8080/health
```

```json
{"status":"ok","version":"3.3.1"}
```

### GET /

Returns the endpoint summary including which backends are available (i.e., whether SMTP and MS Graph env vars were set at startup):

```bash
curl http://localhost:8080/
```

```json
{
  "name": "gomailtest serve",
  "version": "3.3.1",
  "endpoints": [
    {"method":"GET","path":"/health","description":"Health check (no API key required)","available":true},
    {"method":"POST","path":"/smtp/sendmail","description":"Send email via SMTP (X-API-Key required)","available":true},
    {"method":"POST","path":"/msgraph/sendmail","description":"Send email via Microsoft Graph (X-API-Key required)","available":false},
    {"method":"POST","path":"/ews/sendmail","description":"Send email via EWS — not yet implemented","available":false}
  ]
}
```

### POST /smtp/sendmail

Send an email via SMTP. SMTP connection credentials are taken from the `SMTP*` environment variables set at server startup.

**Request body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `to` | `[]string` | Yes | List of TO recipient addresses |
| `cc` | `[]string` | No | List of CC recipient addresses; included in the `Cc:` header and the SMTP envelope |
| `bcc` | `[]string` | No | List of BCC recipient addresses; included in the SMTP envelope only, never in message headers |
| `subject` | `string` | Yes | Email subject |
| `from` | `string` | No | Sender address (overrides `SMTPFROM`; required if `SMTPFROM` is not set) |
| `body` | `string` | No | Plain-text email body |
| `priority` | `string` | No | Email priority: `high`, `normal`, `low` (default: `normal`). `high`/`low` add `X-Priority`, `Importance`, and `Priority` headers |

**Example:**

```bash
curl -X POST http://localhost:8080/smtp/sendmail \
  -H "X-API-Key: mysecretkey" \
  -H "Content-Type: application/json" \
  -d '{
    "to": ["recipient@example.com"],
    "subject": "Hello",
    "body": "This is a test message"
  }'
```

```json
{"status":"ok"}
```

**With explicit sender:**

```bash
curl -X POST http://localhost:8080/smtp/sendmail \
  -H "X-API-Key: mysecretkey" \
  -H "Content-Type: application/json" \
  -d '{
    "to": ["alice@example.com", "bob@example.com"],
    "from": "noreply@example.com",
    "subject": "Hello",
    "body": "This is a test message"
  }'
```

**SMTP environment variables used at startup:**

| Variable | Description |
|----------|-------------|
| `SMTPHOST` | SMTP server hostname (required to enable endpoint) |
| `SMTPPORT` | SMTP server port (default: 25) |
| `SMTPUSERNAME` | SMTP username |
| `SMTPPASSWORD` | SMTP password |
| `SMTPFROM` | Default sender address |
| `SMTPSTARTTLS` | Use STARTTLS (`true`/`false`) |
| `SMTPSMTPS` | Use implicit TLS on port 465 (`true`/`false`) |
| `SMTPSKIPVERIFY` | Skip TLS certificate verification (`true`/`false`) |
| `SMTPPROXY` | HTTP/HTTPS/SOCKS5 proxy URL |

### POST /msgraph/sendmail

Send an email via Microsoft Graph (Exchange Online). MS Graph credentials are taken from the `MSGRAPH*` environment variables set at server startup.

> **Note:** Attachments are not supported via the REST API. Accepting raw file paths from HTTP clients would allow any API-key holder to read arbitrary server-side files.

**Request body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `to` | `[]string` | Yes* | List of TO recipient addresses |
| `cc` | `[]string` | No | List of CC recipient addresses |
| `bcc` | `[]string` | No | List of BCC recipient addresses |
| `subject` | `string` | Yes | Email subject |
| `body` | `string` | No | Plain-text email body |
| `bodyHTML` | `string` | No | HTML email body (takes precedence over `body` if both provided) |
| `priority` | `string` | No | Email priority/importance: `high`, `normal`, `low` (default: `normal`) |

\* At least one of `to`, `cc`, or `bcc` is required.

**Example:**

```bash
curl -X POST http://localhost:8080/msgraph/sendmail \
  -H "X-API-Key: mysecretkey" \
  -H "Content-Type: application/json" \
  -d '{
    "to": ["recipient@example.com"],
    "subject": "Hello from Graph",
    "body": "This is a test message"
  }'
```

```json
{"status":"ok"}
```

**With HTML body and CC/BCC:**

```bash
curl -X POST http://localhost:8080/msgraph/sendmail \
  -H "X-API-Key: mysecretkey" \
  -H "Content-Type: application/json" \
  -d '{
    "to": ["alice@example.com"],
    "cc": ["manager@example.com"],
    "bcc": ["audit@example.com"],
    "subject": "Hello",
    "bodyHTML": "<h1>Hello</h1><p>This is a <strong>test</strong> message.</p>"
  }'
```

**MS Graph environment variables used at startup:**

| Variable | Description |
|----------|-------------|
| `MSGRAPHTENANTID` | Azure Tenant ID (required to enable endpoint) |
| `MSGRAPHCLIENTID` | Application (Client) ID (required to enable endpoint) |
| `MSGRAPHSECRET` | Client Secret |
| `MSGRAPHPFX` | Path to .pfx certificate file (alternative to secret) |
| `MSGRAPHPFXPASS` | Password for the .pfx file |
| `MSGRAPHTHUMBPRINT` | Certificate thumbprint from the CurrentUser\My store |
| `MSGRAPHBEARERTOKEN` | Pre-obtained Bearer token (alternative auth) |
| `MSGRAPHMAILBOX` | Mailbox to send from (e.g., `sender@example.com`) |
| `MSGRAPHPROXY` | HTTP/HTTPS proxy URL |

## Response Format

All endpoints return a JSON object:

```json
{"status":"ok"}
```

On error:

```json
{"status":"error","message":"<description>"}
```

| HTTP Status | Meaning |
|-------------|---------|
| `200` | Success |
| `400` | Invalid request (bad JSON, missing required field, invalid email address) |
| `401` | Missing or invalid `X-API-Key` |
| `501` | Endpoint not implemented (EWS) |
| `503` | Backend not configured (required env vars not set at startup) |
| `500` | Internal error (SMTP or MS Graph send failed) |

## PowerShell Examples

```powershell
# Start the server
$env:SERVE_API_KEY  = "mysecretkey"
$env:SMTPHOST       = "smtp.example.com"
$env:SMTPPORT       = "587"
$env:SMTPUSERNAME   = "user@example.com"
$env:SMTPPASSWORD   = "yourpassword"
$env:SMTPFROM       = "sender@example.com"

gomailtest serve

# Send via PowerShell Invoke-RestMethod
$headers = @{ "X-API-Key" = "mysecretkey"; "Content-Type" = "application/json" }
$body = @{ to = @("recipient@example.com"); subject = "Test"; body = "Hello" } | ConvertTo-Json

Invoke-RestMethod -Method POST -Uri "http://localhost:8080/smtp/sendmail" `
  -Headers $headers -Body $body
```

## Related Documentation

- [docs/protocols/smtp.md](smtp.md) — SMTP CLI usage and flags
- [docs/protocols/msgraph.md](msgraph.md) — Microsoft Graph CLI usage and flags
- [SECURITY.md](../../SECURITY.md) — Security policy
- [BUILD.md](../../BUILD.md) — Build instructions

                          ..ooOO END OOoo..
