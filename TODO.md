# TODO

Outstanding work items for gomailtesttool. Carried over from [CODE_REVIEW.md](CODE_REVIEW.md) (the referenced IMPROVEMENTS.md no longer exists in the repo).

## Email composition features

- [x] Attachments: File attachments with MIME type detection
  - `msgraph sendmail` (`--attachments`) and `smtp sendmail` (`--attachments`), both via shared `internal/common/email.LoadAttachments`
- [x] Inline Attachments: For embedding images in HTML emails (Content-ID/`cid:` references)
  - `msgraph sendmail` and `smtp sendmail` via `--inline-attachments`, referenced from `--bodyhtml`/`--bodyHTML` as `cid:<filename>`
- [x] Custom Headers: Add custom headers via repeatable `--header "Name: Value"`
  - `msgraph sendmail` and `smtp sendmail`; protected headers (From, To, Subject, Date, Message-ID, MIME-Version, Content-Type, etc.) cannot be overridden
- [x] Multipart Messages: Support for plain text and HTML bodies
  - `msgraph sendmail` (`--body` / `--bodyHTML`) and `smtp sendmail` (`--body` / `--bodyhtml`, `multipart/alternative` when both are set)
- [ ] `--template` / `--template-vars`: send from a full `.eml` file or an HTML body template with Go `text/template` variable substitution
  - `--template <file>`: accepts either a raw RFC 822 `.eml` file (all headers + body) or an HTML file used as `BodyHTML`; mutually exclusive with `--body`, `--bodyhtml`, and the existing per-protocol `--body-template`
  - For **EML mode**: `text/template` variable substitution is applied to the raw EML text first, then the complete message is injected via the active protocol — SMTP can raw-inject; EWS/JMAP/Msgraph/Gmail parse the rendered EML and map recognised fields to their native send API
  - For **HTML body mode**: file is processed through `text/template` and used as `BodyHTML`, extending the current Gmail/Msgraph-only `--body-template` to all sendmail protocols (SMTP, EWS, JMAP)
  - `--template-vars key=value` (repeatable, env `*TEMPLATEVARS`): supplies named variables accessible as `{{.Key}}` in the template; applies to both EML and HTML modes
  - Shared template loading + expansion in `internal/common/email` (or a new `internal/common/template` package); each protocol's `RunE` resolves the template before building the message

## Configuration

- [x] `--config <file>`: YAML config file providing default flag values for every protocol/action
  - Registered once on `rootCmd` (`cmd/gomailtest/root.go`), loaded via `bootstrap.LoadConfigFile` in every subcommand's `RunE`
  - Precedence: CLI flags > env vars > `--config` file > built-in defaults (see [docs/config-file.md](docs/config-file.md))
  - `serve` mode also supported via `bootstrap.LoadConfigFileSection`: top-level keys configure the server; nested `smtp:`/`msgraph:` sections provide defaults below `SMTP*`/`MSGRAPH*` env vars

## Connection / TLS

- [x] `--no-starttls` (SMTP, IMAP, POP3) and `--no-smtps`/`--no-imaps`/`--no-pop3s`: force a plain-text connection for one run
  - For SMTP, also suppresses the automatic STARTTLS upgrade in `sendmail`/`testauth` that otherwise triggers when the server advertises STARTTLS on common ports
  - Errors (mutual exclusion) if combined with the corresponding `--starttls`/`--smtps`/`--imaps`/`--pop3s` flag — catches conflicting defaults from `--config`/env vars
  - `smtp teststarttls` rejects `--no-starttls` unless `--smtps` is set (nothing to test otherwise)

## Security Fixes

- [x] Fix broken token masking in `internal/common/security/masking.go` (`MaskAccessToken`) which exposes full tokens <= 16 characters.
  - Tokens <= 8 chars are now fully masked (`****`); 9-16 chars show first/last 2 chars; > 16 chars unchanged. Same fix applied to the duplicate local `maskAccessToken` in `internal/protocols/jmap/utils.go` (the `internal/protocols/smtp` copy already masked <= 16 chars correctly).
- [x] Fix broken password masking in `internal/common/security/masking.go` (`MaskPassword`) which exposes too much of short passwords (e.g., 4 out of 5 chars).
  - Passwords <= 8 chars are now fully masked (`****`); longer passwords still show first/last 2 chars. Same fix applied to the duplicate local `maskPassword` in `internal/protocols/smtp/utils.go` and `internal/protocols/jmap/utils.go`.
- [x] Address API Key authentication bypass risk in `internal/serve/server.go` (empty string comparison).
  - `apiKeyMiddleware` now fails closed: an empty configured `APIKey` or missing `X-API-Key` header is rejected outright, even before the constant-time comparison.
- [x] Change default server bind interface from `0.0.0.0` to `127.0.0.1` for better local security posture in `internal/serve/cmd.go`.
  - `--listen` now defaults to `127.0.0.1`; use `0.0.0.0` to bind all interfaces.

## Architecture & Code Quality Improvements

- [x] Address goroutine leak in the SMTP protocol's response handling when timeouts occur (`internal/smtp/protocol/responses.go`), and unify it with POP3's standard `SetReadDeadline` approach.
  - `ReadResponseWithTimeout` now takes the underlying `net.Conn` and uses `SetReadDeadline` (cleared afterwards) instead of spawning a goroutine with a channel/`time.After`.
- [x] Fix Microsoft Graph retry handling so throttling/service OData errors are actually retried instead of failing on the first attempt.
  - `internal/common/retry.RetryWithBackoffFunc` now accepts a protocol-specific classifier that can mark errors retryable and request a specific delay (for example from `Retry-After`), while `RetryWithBackoff` keeps the generic network-error classifier.
  - `internal/protocols/msgraph/retryWithBackoff` now uses `isRetryableGraphError` to retry typed `*odataerrors.ODataError` responses for HTTP 429/503/504 and Graph codes like `TooManyRequests`, `activityLimitReached`, `ServiceUnavailable`, and `GatewayTimeout`, honoring `Retry-After` when present and capping it at 5 minutes.
  - Added regression/unit coverage in `internal/common/retry/retry_test.go` and `internal/protocols/msgraph/retry_classification_test.go`, including retry-after handling, classifier overrides, Graph throttling retries, and `IsSMTPRetryableError` coverage.

## Configuration follow-ups

- [x] add priority flags on send mail (SMTP, EWS, Graph)
  - `--priority high|normal|low` added to `smtp sendmail` (and `/smtp/sendmail` JSON `priority` field): `high`/`low` add `X-Priority`, `Importance`, and `Priority` headers; `normal` (default) adds none. New header names are protected (cannot be overridden via `--header`).
  - `--priority high|normal|low` added to `msgraph sendmail` (and `/msgraph/sendmail` JSON `priority` field), mapped to the Graph `Importance` field via `models.ParseImportance`/`SetImportance`. This mapping is correct by inspection but not exercised by a test against a live/mock Graph client.
  - EWS has no `sendmail` action (confirmed not present), so it's out of scope here.
- [x] add --ipv4 and --ipv6 commands to all modes (to ask resolver to connect AAAA or A records)
  - New shared `internal/common/network` package: `ResolveForDial` resolves `--host`/`--address` to a single A (`--ipv4`) or AAAA (`--ipv6`) record (or validates a literal IP against the requested family), and `ValidateIPVersionFlags` rejects combining both. Added `--ipv4`/`--ipv6` to SMTP, IMAP, POP3, JMAP, and EWS (config struct, flags, env vars `*IPV4`/`*IPV6`, validation, and dial wiring); see `docs/protocols/*.md`.
  - MS Graph is intentionally excluded: it has no `--host`/dial configuration — the Azure SDK manages connections to Microsoft's cloud endpoints internally, so forcing an address family isn't meaningful there.
- [x] assure that pointing server in IPv6 is possible
  - Fixed `internal/serve/server.go` to build the listen address with `net.JoinHostPort` instead of `fmt.Sprintf("%s:%d", ...)`, so IPv6 literals like `::1` are correctly bracketed (`[::1]:8080`) — verified the server binds and serves over `http://[::1]:<port>`.
  - Also fixed IPv6-unsafe URL construction for JMAP (`GetDiscoveryURL`) and EWS (EWS/autodiscover URLs), which previously produced malformed URLs like `https://::1:443` for IPv6 hosts; both now use `net.JoinHostPort`/bracketing.
- [x] for SMTP allow --use-MX instead --host
  - New `--use-mx` flag (mutually exclusive with `--address`): treats `--host` as a domain, resolves its MX record via the new `network.LookupMX` helper, and connects to the resolved MX hostname. The resolved MX hostname is also used for TLS SNI/certificate validation in `sendmail`, `teststarttls`, and `testauth` via the new `SMTPClient.GetHost()` accessor (`testconnect`/SMTPS path already used `c.host` internally).
  - Verified end-to-end against `gmail.com`: `smtp teststarttls --host gmail.com --use-mx` resolves to `gmail-smtp-in.l.google.com` and passes full certificate verification (no `--skipverify` needed).
  - MX is resolved for `--host` (the domain you're testing), not for `--to` recipient domains — i.e. "use the MX of the host I gave you" rather than "use the MX for where this mail would actually be delivered". Flag if recipient-domain MX was intended instead.
  - Follow-up for `sendmail`: `--use-mx` is now also mutually exclusive with `--host` (in addition to `--address`); the MX lookup domain is instead derived from the first `--to` recipient, so it resolves the MX of where the mail is actually being delivered. `testconnect`/`teststarttls`/`testauth` keep the original `--host`-as-domain behavior.
- [x] clarify in --help output difference between --address (additional parameter, not obligatory and --host - most case needed to connect some official service FQDN/name)
  - `--host` help text now states it's the required service hostname used for the connection, TLS SNI/cert checks, and auth; `--address` help text now says it's an optional override of the dialed IP/host (e.g. behind a load balancer) while `--host` is still used for SNI/cert checks/auth. Updated for SMTP, IMAP, POP3, JMAP (`internal/protocols/*/config.go` and `docs/protocols/*.md`); EWS/MS Graph have no `--address` flag.

## Serve mode

- [x] expose serve mode over MCP in addition to the HTTP/S REST server (3.5.0)
  - Same sendmail capabilities as the REST endpoints are exposed as MCP tools (`smtp_sendmail`, `msgraph_sendmail`, `list_backends`) over two transports from one `mcp.Server`: **Streamable HTTP** mounted at `POST /mcp` on the existing server (behind `X-API-Key`, enabled by default via `--mcp`), and **stdio** via `serve --mcp-stdio` for local AI clients (no API key, HTTP server not started).
  - HTTP handlers and MCP tools now share a transport-agnostic send core (`internal/serve/send.go`: `sendSMTP`/`sendMsgraph`); existing REST behavior is unchanged (`server_test.go` passes as-is). Uses `github.com/modelcontextprotocol/go-sdk` (v1.6.1, requires Go 1.25). Plan: `migration/3.5.MCP.md`; docs: `docs/protocols/serve.md`.
  - stdout hygiene: in stdio mode `os.Stdout` is re-pointed to stderr before logging init (the reused send functions and CSV logger print progress to stdout), and the MCP JSON-RPC channel uses the preserved real stdout via `mcp.IOTransport`.
