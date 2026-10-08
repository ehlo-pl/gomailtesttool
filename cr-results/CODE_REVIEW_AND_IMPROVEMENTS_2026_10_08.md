# Code Review Results - gomailtesttool

**Scope:** Full codebase review of version 4.1.11 (178 Go files; 151 Markdown files), 2026-10-08. Focused on security-sensitive behavior and consistency between implementation, user documentation, and the security policy.
**Method:** Traced credential and email-data handling through implementation and docs; checked export permissions, HTTP server transport/authentication, and stated threat model. Findings below are limited to behaviors verified in the current tree. No dependency advisory scan or live network penetration test was performed.

## 1. Security findings

### 1.1 Major: Exported message files are world-readable on common Unix configurations

- **Location:** `internal/common/export/export.go:12-23`; `internal/protocols/imap/exportmessages.go:111`; `internal/protocols/pop3/exportmessages.go:133`; `internal/protocols/gmail/handlers.go:402`; `internal/protocols/msgraph/handlers.go:1481,1551`.
- **Issue:** The shared export directory is created with mode `0755`, and these exporters create raw `.eml` messages and Graph JSON exports with mode `0644`. With the usual Unix umask of `022`, other local users can traverse the directory and read exported message bodies, headers, and attachments represented in the message. JMAP and EWS use `0600` for `.eml` output, so protection is inconsistent across protocols.
- **Impact:** On a shared host, mailbox content exported by an authorized account can be read by unrelated local users. The default export location is under the OS temp directory, but the created `export/<date>` path and files remain accessible according to these modes.
- **Recommendation:** Use owner-only permissions for sensitive export directories and files (for example, `0700` and `0600`) consistently across every protocol. Add tests that check file modes on supported Unix platforms.

### 1.2 Major: Remote serve traffic carries the API key and email content without TLS

- **Location:** `internal/serve/server.go:43-51` (`http.Server`); `internal/serve/server.go:53-60` (`ListenAndServe`); `internal/serve/server.go:103-115` (API-key middleware); `docs/protocols/serve.md:274,289-295`.
- **Issue:** The serve process starts a plaintext HTTP listener and has no TLS configuration. The docs describe Streamable HTTP as suitable for “Networked / remote MCP callers” and direct clients to `http://<host>:<port>/mcp`. When bound to a non-loopback interface, the `X-API-Key` and message content are sent without transport encryption. A network observer can capture and replay the key to invoke the protected send endpoints.
- **Impact:** Credentials and email content can be disclosed, and captured API keys can be used to send email. The default loopback bind limits exposure only when it is retained; the command also supports binding to all interfaces.
- **Recommendation:** Either add TLS support or clearly require a TLS-terminating reverse proxy/tunnel for any non-loopback deployment. Document that the built-in HTTP listener must not be exposed directly to an untrusted network, and adjust the “remote MCP” guidance accordingly.

### 1.3 Medium: Serve guide recommends placing its API key in process arguments

- **Location:** `docs/protocols/serve.md:10-18,30-37`; contrast `SECURITY.md:177-185`.
- **Issue:** The Quick Start sets `SERVE_API_KEY` and then redundantly passes the same key as `--api-key`; the invocation template also recommends `gomailtest serve --api-key <secret>`. Command-line arguments may be visible to process-inspection tools, while the security guide explicitly advises using environment variables instead of arguments for secrets. The CLI already accepts `SERVE_API_KEY`.
- **Impact:** Users following the guide can expose the server’s authentication key through process listings, diagnostics, or process monitoring.
- **Recommendation:** Use only `gomailtest serve` in the environment-based Quick Start and make `SERVE_API_KEY` the preferred documented configuration. If the flag remains documented, warn that it may be visible in process arguments.

## 2. Documentation coherence findings

### 2.1 Medium: Security policy denies the network-service use case now documented as supported

- **Location:** `SECURITY.md:32-36`; `README.md:277,292`; `docs/protocols/serve.md:3-5,270-295`.
- **Issue:** The security policy says the tool is not designed to run as a network service or accept input from public APIs. The current product includes `serve`, with authenticated REST and MCP endpoints, and its documentation describes networked/remote clients. The policy does not define the trust boundary, deployment requirements, or how its CLI-only assumptions apply to this service.
- **Impact:** Users and reviewers receive conflicting security expectations about a supported attack surface. In particular, the policy can be read as excluding serve mode from security guidance, while the feature documentation presents it as a network API.
- **Recommendation:** Update the security policy to distinguish the trusted CLI use case from `serve`, describe the API-key and transport limitations, and set explicit deployment assumptions. Link to those requirements from the serve guide.

### 2.2 Minor: `GET /` documentation omits the MCP endpoint returned by the server

- **Location:** `docs/protocols/serve.md:74-92`; `internal/serve/server.go:122-143`.
- **Issue:** The documented `GET /` response lists `/health`, SMTP, MS Graph, and EWS, but the actual response also includes `/mcp` with availability based on `EnableMCP`. Since MCP is enabled by default, the example does not match a default server response.
- **Recommendation:** Add the `/mcp` entry to the example or label it as a shortened response.

## 3. Prioritized backlog

Score = (Impact + Risk) × (6 − Effort), each 1–5.

| # | Item | Impact | Risk | Effort | Score |
|---|------|--------|------|--------|-------|
| 1 | Remove secret-bearing CLI invocation from serve examples (1.3) | 3 | 4 | 1 | 35 |
| 2 | Prefer owner-only permissions for exported mail and directories (1.1) | 4 | 4 | 2 | 32 |
| 3 | Require/document TLS protection for remote serve deployments (1.2) | 4 | 4 | 2 | 32 |
| 4 | Reconcile security policy with serve mode (2.1) | 3 | 3 | 1 | 30 |
| 5 | Bring the `GET /` sample into sync with the response (2.2) | 1 | 1 | 1 | 10 |

## 4. Validation

- `go test ./...`: all packages passed except `internal/common/network`, where `TestLookupMX_KnownDomain` failed because the sandbox DNS resolver at `127.0.0.53` returned “server misbehaving”. This test requires external DNS; no application-code failure was observed.
- No source code was changed; this review report does not require a build or code lint.

## 5. Summary

The most important verified risk is confidentiality: several protocols save exported mailbox messages with permissions that are readable by other local users under common Unix defaults, unlike JMAP/EWS. The HTTP/MCP service also provides no TLS itself, although documentation describes remote use; deployments need an explicitly documented TLS boundary. Finally, serve guidance should stop putting the API key in command arguments and the global security model should cover this supported network-facing feature. No code was changed as part of this review; the findings are recommendations for follow-up work.
