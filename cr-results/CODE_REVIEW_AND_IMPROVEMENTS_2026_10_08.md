# Code Review Results - gomailtesttool

**Scope:** Full codebase review of version 4.1.11 (178 Go files; 151 Markdown files), 2026-10-08. Focused on security-sensitive behavior and consistency between implementation, user documentation, and the security policy.
**Method:** Traced credential and email-data handling through implementation, release workflow, and docs; checked export permissions, SMTP/HTTP transport security, shell interpolation, and stated threat model. Findings below are limited to behaviors verified in the current tree. No dependency advisory scan, live network test, or check of repository tag protections was performed.

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

### 1.4 Major: SMTP authentication can proceed without TLS

- **Location:** `internal/protocols/smtp/testauth.go:113-159,161-193`; `internal/protocols/smtp/sendmail.go:122-168,170-193`; `internal/protocols/smtp/smtp_client.go:323-329,543-554`.
- **Issue:** On non-SMTPS connections, `testauth` and `sendmail` upgrade only when STARTTLS is advertised. If it is absent—including when an on-path attacker strips the EHLO capability—the code continues to select an advertised AUTH mechanism and authenticate. The custom PLAIN implementation explicitly bypasses the standard library’s TLS check and sends the username and password in a base64-encoded, reversible payload.
- **Impact:** Passwords and bearer tokens can be exposed to a network observer when authentication occurs over plaintext SMTP. The same behavior is reachable with `--no-starttls`, but can also occur by default if STARTTLS is not advertised.
- **Recommendation:** Refuse password/token authentication without an established TLS connection by default. If insecure SMTP authentication must remain available for diagnostics, require an explicit opt-in and warn clearly.

### 1.5 Major: A pushed tag is interpolated into a shell command in the release workflow

- **Location:** `.github/workflows/build.yml:4-6,10-11,55-59,107-109`.
- **Issue:** The tag-triggered workflow expands `${{ github.ref_name }}` directly inside a Bash `run` command. Git accepts a tag such as `v1$(id)` (verified with `git check-ref-format`), and Bash evaluates the command substitution after Actions inserts the tag into the script. The workflow also grants `contents: write` at workflow scope.
- **Impact:** A user able to push a matching tag can cause shell command execution on the release runner, potentially affecting release artifacts and the write-enabled workflow token. Repository tag protection rules were not available for this review, so the set of users able to trigger this is unknown.
- **Recommendation:** Pass the tag through an environment variable and reference that variable in the script; shell does not recursively evaluate command substitutions in expanded variable values. Limit write permissions to the release job and only the required steps.

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

### 2.3 Minor: Build troubleshooting documents an outdated minimum Go version

- **Location:** `docs/BUILD.md:213`; `go.mod:3`.
- **Issue:** The troubleshooting section says Go 1.24 or later is sufficient, while the module declares Go 1.25. This misstates the module’s minimum toolchain requirement, especially for users with automatic toolchain downloads disabled.
- **Recommendation:** Change the troubleshooting prerequisite to Go 1.25 or later.

## 3. Prioritized backlog

Score = (Impact + Risk) × (6 − Effort), each 1–5.

| # | Item | Impact | Risk | Effort | Score |
|---|------|--------|------|--------|-------|
| 1 | Prevent shell evaluation of tag names in release workflow (1.5) | 5 | 5 | 2 | 40 |
| 2 | Fail closed on SMTP credential auth without TLS (1.4) | 5 | 5 | 2 | 40 |
| 3 | Remove secret-bearing CLI invocation from serve examples (1.3) | 3 | 4 | 1 | 35 |
| 4 | Prefer owner-only permissions for exported mail and directories (1.1) | 4 | 4 | 2 | 32 |
| 5 | Require/document TLS protection for remote serve deployments (1.2) | 4 | 4 | 2 | 32 |
| 6 | Reconcile security policy with serve mode (2.1) | 3 | 3 | 1 | 30 |
| 7 | Bring the `GET /` sample into sync with the response (2.2) | 1 | 1 | 1 | 10 |
| 8 | Correct the Go version in build troubleshooting (2.3) | 1 | 1 | 1 | 10 |

## 4. Validation

- `go test ./...`: all packages passed except `internal/common/network`, where `TestLookupMX_KnownDomain` failed because the sandbox DNS resolver at `127.0.0.53` returned “server misbehaving”. This test requires external DNS; no application-code failure was observed.
- No source code was changed; this review report does not require a build or code lint.

## 5. Summary

The most urgent findings are the release workflow’s shell interpolation of tag names and SMTP credential authentication without TLS. Export permissions also expose mailbox contents to other local users under common Unix defaults. The HTTP/MCP service provides no TLS itself despite documented remote use, and serve guidance should stop putting API keys in command arguments. The global security policy and build troubleshooting also need updates to match supported features and the current Go requirement. No source code was changed as part of this review; findings are recommendations for follow-up work.
