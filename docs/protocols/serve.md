# Serve Mode — gomailtest

Server for sending emails programmatically — over an HTTP REST API **and** over MCP (Model Context Protocol).

Credentials are loaded from environment variables at startup. Each request carries only message content — no credentials in request bodies. The MCP tools expose the same sendmail capabilities as the REST endpoints (see [MCP (Model Context Protocol)](#mcp-model-context-protocol) below).

> **Security boundary:** `serve` can send mail using the credentials configured in its process, so only authorized, trusted clients should be able to reach it. Its shared API key is not caller-specific authorization, and the built-in listener uses plain HTTP (no TLS). Remote/non-loopback deployments **must** use a TLS-terminating reverse proxy or an authenticated encrypted tunnel and block direct access to the listener. A private network alone does not provide encryption. Never expose the built-in HTTP listener directly to untrusted networks. See [Secure Remote Deployments](#secure-remote-deployments) and [SECURITY.md](../../SECURITY.md#serve-mode-trust-boundary).

## Quick Start

```bash
# Start the server (SERVE_API_KEY is required)
export SERVE_API_KEY="mysecretkey"
export SMTPHOST="smtp.example.com"
export SMTPPORT="587"
export SMTPUSERNAME="user@example.com"
export SMTPPASSWORD="yourpassword"
export SMTPFROM="sender@example.com"

gomailtest serve

# Send an email via the REST API
curl -X POST http://localhost:8080/smtp/sendmail \
  -H "X-API-Key: mysecretkey" \
  -H "Content-Type: application/json" \
  -d '{"to":["recipient@example.com"],"subject":"Test","body":"Hello from serve mode"}'
```

## Starting the Server

```bash
gomailtest serve [--port 8080] [--listen 127.0.0.1]
```

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--api-key` | `SERVE_API_KEY` | — | **Required.** Prefer `SERVE_API_KEY` to avoid exposing the key in command-line arguments; otherwise, value expected in the `X-API-Key` request header |
| `--port` | `SERVE_PORT` | `8080` | HTTP listen port |
| `--listen` | `SERVE_LISTEN` | `127.0.0.1` | Bind address (e.g., `0.0.0.0` to listen on all interfaces) |

If you use `--api-key`, be aware that process-inspection tools may expose its value.

The built-in HTTP server does not provide TLS. The API key is a shared bearer secret, not a user identity or per-caller authorization mechanism. All protected callers can invoke the available sendmail tools using the server's configured credentials. Restrict network access to authorized clients and follow the remote deployment requirements below. `GET /health` and `GET /` are unauthenticated.

## Secure Remote Deployments

Plain HTTP exposes `X-API-Key` and email content to network observers, who can replay a captured key. For any remote/non-loopback deployment, encrypted transport is **required** for both REST and MCP. SMTP TLS settings protect only the outbound SMTP connection, not client connections to `serve`.

### TLS-terminating reverse proxy (recommended)

1. Run `gomailtest serve --listen 127.0.0.1 --port 8080` with `SERVE_API_KEY` set in its environment.
2. Run a reverse proxy on the same host, exposing HTTPS only to authorized clients. For example, a Caddy configuration for a hostname you control:

   ```caddyfile
   mail-api.example.com {
       reverse_proxy 127.0.0.1:8080
   }
   ```

   Caddy can obtain and renew a publicly trusted certificate via ACME. Point the hostname's DNS at the proxy and satisfy the certificate authority's validation requirements. Configure firewall/allowlist or VPN access controls for authorized callers; TLS does not make this a public-facing API.
3. Block direct network access to port 8080, including container port publishing. Clients must use `https://mail-api.example.com`, never the backend HTTP address. If the proxy is on another host, protect the proxy-to-listener hop with an authenticated encrypted tunnel as well; firewall restrictions alone do not encrypt it.
4. Keep the `X-API-Key` header intact and support MCP's streaming responses without buffering or premature proxy timeouts. Do not log API keys or message bodies.

**Certificate choices:**

- **Let's Encrypt / another public CA:** Recommended for a DNS hostname you control. Automate issuance and renewal at the proxy. Public ACME issuance requires domain validation (HTTP, TLS, or DNS challenge); DNS validation can be used for a private deployment without opening the API to the internet.
- **Private CA / self-signed certificate:** Suitable for managed internal clients if you securely distribute the CA or certificate to each client's trust store and issue a certificate valid for the hostname clients use. Renew it before expiry. A self-signed certificate without configured client trust is not sufficient; never disable certificate verification (`curl -k` / `--insecure`) to make it work.

Verify HTTPS and certificate trust before sending secrets:

```bash
curl --fail https://mail-api.example.com/health
# For a private CA or explicitly trusted self-signed certificate:
curl --fail --cacert /path/to/trusted-ca-or-cert.pem https://mail-api.example.com/health
```

Also verify from a remote client that the backend port is unreachable. Send authenticated requests directly to HTTPS; an HTTP-to-HTTPS redirect cannot protect a key or body already sent over HTTP.

### Encrypted tunnel alternative

For a small controlled deployment, keep `serve` on the remote host's loopback interface and forward it over SSH:

```bash
ssh -N -L 127.0.0.1:18080:127.0.0.1:8080 user@mail-server.example.com
```

Verify the SSH host key through a trusted channel. While the tunnel is active, REST clients use `http://127.0.0.1:18080` and MCP clients use `http://127.0.0.1:18080/mcp`, still supplying `X-API-Key`. HTTP is confined to loopback at both ends; the network hop is encrypted and authenticated by SSH. Do not bind the forwarded port to a non-loopback address or open the remote backend port.

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
| `POST` | `/mcp` | Yes | MCP (Streamable HTTP) endpoint exposing the sendmail tools; enabled by default (see [MCP](#mcp-model-context-protocol)) |

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
    {"method":"POST","path":"/ews/sendmail","description":"Send email via EWS — not yet implemented","available":false},
    {"method":"POST","path":"/mcp","description":"MCP (Streamable HTTP) endpoint exposing the sendmail tools (X-API-Key required)","available":true}
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

## MCP (Model Context Protocol)

Serve mode also exposes the sendmail capabilities as MCP tools, so AI clients (e.g. Claude Desktop/Code) and other MCP callers can send mail through the same configured backends. The MCP tools reuse the exact validation and send logic as the REST endpoints.

### Transports

| Transport | How to enable | Auth | Use case |
|-----------|---------------|------|----------|
| **Streamable HTTP** | On by default when the HTTP server runs; disable with `--mcp=false` (`SERVE_MCP`) | `X-API-Key` header (same as REST) | Local callers; remote callers only via HTTPS proxy or encrypted tunnel |
| **stdio** | `serve --mcp-stdio` (`SERVE_MCP_STDIO`) | None (local subprocess) | Local AI clients that launch the binary |

In `--mcp-stdio` mode no HTTP server is started, `--api-key` is not required, and all diagnostic output goes to **stderr** (stdout is reserved for the MCP JSON-RPC channel).

### Tools

| Tool | Backend | Arguments |
|------|---------|-----------|
| `smtp_sendmail` | SMTP | `to` (required), `cc`, `bcc`, `from`, `subject` (required), `body`, `priority` — see [POST /smtp/sendmail](#post-smtpsendmail) |
| `msgraph_sendmail` | Microsoft Graph | `to`/`cc`/`bcc` (at least one), `subject` (required), `body`, `bodyHTML`, `priority` — see [POST /msgraph/sendmail](#post-msgraphsendmail). Attachments are not supported. |
| `list_backends` | — | none; reports which backends are configured and available |

A tool call against an unconfigured backend returns an error result (mirroring the REST `503`), rather than the tool being hidden.

### Streamable HTTP endpoint

For local callers, point an MCP client at `http://127.0.0.1:8080/mcp`. For remote callers, use the TLS proxy URL, e.g. `https://mail-api.example.com/mcp`, with certificate verification enabled, or the local endpoint of an encrypted tunnel as described in [Secure Remote Deployments](#secure-remote-deployments). Never send the API key or tool arguments to a remote plaintext HTTP URL.

Send the API key as a header:

```
X-API-Key: <value set via --api-key or SERVE_API_KEY>
```

### stdio (Claude Desktop / Claude Code)

Run the server as a subprocess with credentials in its environment. Example MCP client configuration:

```json
{
  "mcpServers": {
    "gomailtest": {
      "command": "gomailtest",
      "args": ["serve", "--mcp-stdio"],
      "env": {
        "SMTPHOST": "smtp.example.com",
        "SMTPPORT": "587",
        "SMTPUSERNAME": "user@example.com",
        "SMTPPASSWORD": "yourpassword",
        "SMTPFROM": "sender@example.com"
      }
    }
  }
}
```

For Claude Code you can register the same command with `claude mcp add gomailtest -- gomailtest serve --mcp-stdio` (supply the `SMTP*`/`MSGRAPH*` environment variables to the process).

## Related Documentation

- [docs/INTEGRATION_TESTS.md](../INTEGRATION_TESTS.md) — end-to-end loops such as send via SMTP/MS Graph and verify delivery with Gmail
- [docs/protocols/smtp.md](smtp.md) — SMTP CLI usage and flags
- [docs/protocols/msgraph.md](msgraph.md) — Microsoft Graph CLI usage and flags
- [SECURITY.md](../../SECURITY.md) — Security policy
- [BUILD.md](../../BUILD.md) — Build instructions

                          ..ooOO END OOoo..
