# Architecture Diagram - gomailtesttool Suite

## Overview

**gomailtesttool** is a unified CLI (`gomailtest`) for email infrastructure testing, with 7 protocol subcommands plus developer tooling and an HTTP/MCP server mode.

Released as **3 separate binaries** per platform (build-tag-selected at compile time):
- **`gomailtest`** — standard protocols: smtp, imap, pop3, jmap
- **`gomailtest-exchange`** — Microsoft/Exchange protocols: ews, msgraph
- **`gomailtest-gmail`** — Google Workspace: gmail

All three binaries always include `serve` and `devtools`.

Protocol subcommands:
- **smtp** - SMTP connectivity and TLS diagnostics
- **imap** - IMAP server testing with OAuth2
- **pop3** - POP3 server testing with OAuth2
- **jmap** - JMAP protocol testing
- **ews** - Exchange Web Services (on-premises Exchange 2007–2019)
- **msgraph** - Microsoft Graph API (Exchange Online)
- **gmail** - Google Workspace / Gmail API
- **serve** - HTTP/REST + MCP server for triggering send operations programmatically
- **devtools** - Release automation and environment management

## File Structure and Dependencies

```
gomailtesttool/
├── cmd/
│   └── gomailtest/                   # Single binary entry point (build-tag-selected protocols)
│       ├── main.go                   # main() → Execute()
│       ├── root.go                   # Cobra root command, registers subcommands
│       ├── protocols_smtp.go         # //go:build smtp || !custom
│       ├── protocols_imap.go         # //go:build imap || !custom
│       ├── protocols_pop3.go         # //go:build pop3 || !custom
│       ├── protocols_jmap.go         # //go:build jmap || !custom
│       ├── protocols_ews.go          # //go:build ews || !custom
│       ├── protocols_msgraph.go      # //go:build msgraph || !custom
│       └── protocols_gmail.go        # //go:build gmail || !custom
│
├── internal/
│   ├── common/                       # Cross-protocol utilities
│   │   ├── bootstrap/
│   │   │   └── bootstrap.go          # Signal context setup, config/logger init
│   │   ├── email/                    # Email address parsing and validation
│   │   │   └── email.go
│   │   ├── export/
│   │   │   └── export.go             # JSON export helpers
│   │   ├── logger/                   # Structured logging
│   │   │   ├── csv.go
│   │   │   ├── json.go
│   │   │   ├── logger.go
│   │   │   └── slog.go               # slog-based structured logger
│   │   ├── mime/                     # MIME type detection
│   │   │   └── mime.go
│   │   ├── network/                  # Dial helpers: ResolveForDial, LookupMX, ValidateIPVersionFlags
│   │   │   └── network.go
│   │   ├── ratelimit/
│   │   │   └── ratelimit.go
│   │   ├── retry/
│   │   │   └── retry.go              # Exponential backoff; protocol-specific classifier hook
│   │   ├── security/
│   │   │   └── masking.go
│   │   ├── template/                 # Email template rendering (--template / --template-vars)
│   │   │   └── template.go           # ParseVars, Render, IsEML, ParseEML
│   │   ├── timeslot/                 # Calendar availability analysis (findtimeslot)
│   │   │   └── timeslot.go
│   │   ├── tls/                      # TLS certificate display and validation
│   │   │   ├── certificate.go
│   │   │   ├── display.go
│   │   │   └── validation.go
│   │   ├── validation/
│   │   │   └── validation.go
│   │   └── version/
│   │       └── version.go            # Single source of truth for version
│   │
│   ├── serve/                        # HTTP + MCP server subcommand
│   │   ├── cmd.go                    # 'gomailtest serve' — startup, env loading, client init
│   │   ├── config.go                 # ServeConfig (Port, Listen, APIKey, EnableMCP)
│   │   ├── server.go                 # HTTP server, mux, API key middleware, /health
│   │   ├── send.go                   # Transport-agnostic send core (sendSMTP / sendMsgraph)
│   │   ├── smtp_handler.go           # POST /smtp/sendmail
│   │   ├── msgraph_handler.go        # POST /msgraph/sendmail
│   │   ├── ews_handler.go            # POST /ews/sendmail (501 placeholder)
│   │   ├── mcp.go                    # MCP server: tool registration, stdio/HTTP transports
│   │   └── server_test.go
│   │
│   ├── devtools/                     # Developer-facing subcommand
│   │   ├── cmd.go                    # 'gomailtest devtools' root
│   │   ├── env/
│   │   │   ├── env.go                # MSGRAPH* env var management
│   │   │   └── env_cmd.go
│   │   └── release/
│   │       ├── release.go            # Release orchestration
│   │       ├── release_cmd.go
│   │       ├── version.go            # Version bump logic
│   │       ├── changelog.go          # ChangeLog/{version}.md creation
│   │       ├── git.go                # git commit/tag/push
│   │       ├── gh.go                 # GitHub PR/release via gh CLI
│   │       ├── security_scan.go      # Pre-release secret scanning
│   │       ├── editor.go             # Interactive editor prompts
│   │       └── prompt.go             # User prompts
│   │
│   ├── protocols/                    # Protocol implementations
│   │   ├── smtp/
│   │   │   ├── cmd.go
│   │   │   ├── config.go
│   │   │   ├── smtp_client.go
│   │   │   ├── testconnect.go
│   │   │   ├── teststarttls.go
│   │   │   ├── testauth.go
│   │   │   ├── sendmail.go
│   │   │   ├── tls_display.go
│   │   │   └── utils.go
│   │   │
│   │   ├── imap/
│   │   │   ├── cmd.go
│   │   │   ├── config.go
│   │   │   ├── imap_client.go
│   │   │   ├── testconnect.go
│   │   │   ├── teststarttls.go
│   │   │   ├── testauth.go
│   │   │   ├── listfolders.go
│   │   │   ├── listmail.go
│   │   │   ├── exportmessages.go
│   │   │   └── utils.go
│   │   │
│   │   ├── pop3/
│   │   │   ├── cmd.go
│   │   │   ├── config.go
│   │   │   ├── pop3_client.go
│   │   │   ├── testconnect.go
│   │   │   ├── teststarttls.go
│   │   │   ├── testauth.go
│   │   │   ├── listmail.go
│   │   │   ├── exportmessages.go
│   │   │   └── utils.go
│   │   │
│   │   ├── jmap/
│   │   │   ├── cmd.go
│   │   │   ├── config.go
│   │   │   ├── jmap_client.go
│   │   │   ├── testconnect.go
│   │   │   ├── testauth.go
│   │   │   ├── listfolders.go
│   │   │   ├── listmail.go
│   │   │   ├── sendmail.go
│   │   │   ├── exportmessages.go
│   │   │   └── utils.go
│   │   │
│   │   ├── ews/
│   │   │   ├── cmd.go
│   │   │   ├── config.go
│   │   │   ├── ews_client.go
│   │   │   ├── soap_bodies.go
│   │   │   ├── testconnect.go
│   │   │   ├── testauth.go
│   │   │   ├── getfolder.go
│   │   │   ├── autodiscover.go
│   │   │   ├── listfolders.go
│   │   │   ├── listmail.go
│   │   │   ├── sendmail.go
│   │   │   ├── sendinvite.go
│   │   │   ├── getevents.go
│   │   │   ├── getschedule.go
│   │   │   ├── findtimeslot.go
│   │   │   ├── exportmessages.go
│   │   │   └── utils.go
│   │   │
│   │   ├── msgraph/
│   │   │   ├── cmd.go
│   │   │   ├── config.go
│   │   │   ├── auth.go
│   │   │   ├── handlers.go
│   │   │   ├── testconnect.go
│   │   │   ├── testauth.go
│   │   │   ├── getevents.go
│   │   │   ├── sendinvite.go
│   │   │   ├── getschedule.go
│   │   │   ├── findtimeslot.go
│   │   │   ├── sendmail.go
│   │   │   ├── getinbox.go
│   │   │   ├── listfolders.go
│   │   │   ├── listmail.go
│   │   │   ├── exportinbox.go
│   │   │   ├── exportmessages.go
│   │   │   ├── searchandexport.go
│   │   │   ├── exportbearertoken.go
│   │   │   ├── cert_windows.go       # Windows cert store (build: windows)
│   │   │   ├── cert_stub.go          # Cross-platform stub (build: !windows)
│   │   │   └── utils.go
│   │   │
│   │   └── gmail/
│   │       ├── cmd.go
│   │       ├── config.go
│   │       ├── auth.go               # Service account DWD, bearer token, OAuth loopback
│   │       ├── handlers.go
│   │       ├── testconnect.go
│   │       ├── testauth.go
│   │       ├── sendmail.go
│   │       ├── getinbox.go
│   │       ├── listfolders.go
│   │       ├── listmail.go
│   │       ├── exportmessages.go
│   │       ├── getevents.go
│   │       ├── sendinvite.go
│   │       ├── getschedule.go
│   │       ├── findtimeslot.go
│   │       ├── exportbearertoken.go
│   │       └── utils.go
│   │
│   ├── smtp/                         # SMTP protocol primitives
│   │   ├── exchange/
│   │   │   └── detection.go
│   │   └── protocol/
│   │       ├── capabilities.go
│   │       ├── commands.go
│   │       └── responses.go
│   │
│   ├── imap/protocol/
│   │   └── capabilities.go
│   │
│   ├── pop3/protocol/
│   │   ├── capabilities.go
│   │   ├── commands.go
│   │   └── responses.go
│   │
│   └── jmap/protocol/
│       ├── methods.go
│       ├── session.go
│       └── types.go
│
├── tests/
│   └── integration/
│       └── sendmail_test.go          # MS Graph integration tests (build: integration)
│
├── scripts/
│   └── check-integration-env.sh     # Validates MSGRAPH* env vars before integration tests
│
├── docs/
│   ├── config-examples/              # Per-protocol YAML config examples
│   ├── env-examples/                 # Per-protocol .env examples
│   ├── protocols/                    # Per-protocol user guides
│   │   ├── smtp.md, imap.md, pop3.md, jmap.md
│   │   ├── ews.md, msgraph.md, gmail.md
│   │   └── serve.md                  # HTTP/REST + MCP server docs
│   └── config-file.md
│
├── ChangeLog/                        # Per-version changelogs
├── Makefile                          # Primary build system
├── build-all.ps1                     # Windows build script
├── run-integration-tests.ps1         # Integration test runner
├── go.mod
└── go.sum
```

## Command Structure

```
gomailtest
├── smtp
│   ├── testconnect
│   ├── teststarttls
│   ├── testauth
│   └── sendmail
├── imap
│   ├── testconnect
│   ├── teststarttls
│   ├── testauth
│   ├── listfolders
│   ├── listmail
│   └── exportmessages
├── pop3
│   ├── testconnect
│   ├── teststarttls
│   ├── testauth
│   ├── listmail
│   └── exportmessages
├── jmap
│   ├── testconnect
│   ├── testauth
│   ├── listfolders
│   ├── listmail
│   ├── sendmail
│   └── exportmessages
├── ews
│   ├── testconnect
│   ├── testauth
│   ├── getfolder
│   ├── autodiscover
│   ├── listfolders
│   ├── listmail
│   ├── sendmail
│   ├── sendinvite
│   ├── getevents
│   ├── getschedule
│   ├── findtimeslot
│   └── exportmessages
├── msgraph
│   ├── testconnect
│   ├── testauth
│   ├── sendmail
│   ├── sendinvite
│   ├── getinbox
│   ├── listfolders
│   ├── listmail
│   ├── getevents
│   ├── getschedule
│   ├── findtimeslot
│   ├── exportinbox
│   ├── exportmessages
│   ├── searchandexport
│   └── exportbearertoken
├── gmail
│   ├── testconnect
│   ├── testauth
│   ├── sendmail
│   ├── sendinvite
│   ├── getinbox
│   ├── listfolders
│   ├── listmail
│   ├── getevents
│   ├── getschedule
│   ├── findtimeslot
│   ├── exportmessages
│   └── exportbearertoken
├── serve
│   ├── (GET)  /health
│   ├── (POST) /smtp/sendmail
│   ├── (POST) /msgraph/sendmail
│   ├── (POST) /ews/sendmail        (501 — not yet implemented)
│   └── (POST) /mcp                 (Streamable HTTP MCP endpoint, behind X-API-Key)
└── devtools
    ├── env       (get/set/clear MSGRAPH* environment variables)
    └── release   (interactive: version bump → changelog → git tag → GitHub release)
```

## Build System

### Makefile (primary)

```
make build          → go build -ldflags="-s -w" -o bin/gomailtest ./cmd/gomailtest
make build-verbose  → same with -v flag
make test           → go test ./...
make integration-test → build + check env + go test -tags integration ./tests/integration/
make clean          → rm -f bin/gomailtest[.exe]
make help           → list targets
```

### build-all.ps1 (Windows convenience)

```
.\build-all.ps1           → build bin/gomailtest.exe (default tags)
.\build-all.ps1 -Verbose  → build with verbose Go output
```

### Build Tags System

Protocols are selectively compiled via build tags. The `custom` meta-tag enables selective
compilation; without it all protocols are included (default `go build`).

Valid protocol tags: `smtp`, `imap`, `pop3`, `jmap`, `ews`, `msgraph`, `gmail`

`serve` and `devtools` are always compiled regardless of tags.

### GitHub Actions CI/CD (.github/workflows/build.yml)

```
On: push tags (v*) | pull_request → main

test job (ubuntu / windows / macos):
  └── go test -v -race ./...
  └── coverage report (ubuntu only)

lint job (ubuntu, continue-on-error):
  └── golangci-lint

build job (on tag push, needs: test):
  Matrix: windows-latest (amd64), ubuntu-latest (amd64), macos-latest (arm64)
  ├── go build -tags "custom,smtp,pop3,imap,jmap"  -o bin/gomailtest[.exe]
  ├── go build -tags "custom,ews,msgraph"           -o bin/gomailtest-exchange[.exe]
  ├── go build -tags "custom,gmail"                 -o bin/gomailtest-gmail[.exe]
  ├── Verify all three binaries exist
  ├── Create ZIP: all 3 binaries + README.md + TOOLS.md + LICENSE
  │   → gomailtesttool-{os}-{arch}-{tag}.zip
  ├── Upload artifacts
  └── Create GitHub Release (softprops/action-gh-release)
```

## Application Flow

```
gomailtest <subcommand> [flags]
          │
          ▼
cmd/gomailtest/root.go
  rootCmd.AddCommand(smtp.NewCmd())      ← protocols_smtp.go    (build: smtp || !custom)
  rootCmd.AddCommand(imap.NewCmd())      ← protocols_imap.go    (build: imap || !custom)
  rootCmd.AddCommand(pop3.NewCmd())      ← protocols_pop3.go    (build: pop3 || !custom)
  rootCmd.AddCommand(jmap.NewCmd())      ← protocols_jmap.go    (build: jmap || !custom)
  rootCmd.AddCommand(ews.NewCmd())       ← protocols_ews.go     (build: ews || !custom)
  rootCmd.AddCommand(msgraph.NewCmd())   ← protocols_msgraph.go (build: msgraph || !custom)
  rootCmd.AddCommand(gmail.NewCmd())     ← protocols_gmail.go   (build: gmail || !custom)
  rootCmd.AddCommand(serve.NewCmd())     ← always included
  rootCmd.AddCommand(devtools.NewCmd())  ← always included
          │
          ▼
internal/protocols/<protocol>/cmd.go   ← flags, validation, dispatch
          │
          ▼
internal/protocols/<protocol>/<action>.go  ← operation logic
          │
          ├─► internal/common/logger/    ← CSV/JSON/slog output
          ├─► internal/common/retry/     ← exponential backoff
          ├─► internal/common/ratelimit/ ← token bucket
          ├─► internal/common/network/   ← dial helpers, MX lookup, IPv4/IPv6 resolution
          ├─► internal/common/template/  ← --template / --template-vars rendering
          ├─► internal/common/timeslot/  ← findtimeslot availability logic
          └─► internal/<protocol>/protocol/ ← protocol primitives
```

## Protocol Implementations

### smtp (internal/protocols/smtp/)

```
cmd.go
  └─► testconnect.go     — TCP connectivity test
  └─► teststarttls.go    — TLS handshake, cert validation, cipher strength, Exchange detection
  └─► testauth.go        — PLAIN, LOGIN, CRAM-MD5, XOAUTH2
  └─► sendmail.go        — send test email; --template/.eml raw injection; --use-mx
  └─► smtp_client.go     — SMTP client logic
  └─► tls_display.go     — TLS diagnostic output
```

### imap (internal/protocols/imap/)

```
cmd.go
  └─► testconnect.go     — TCP/TLS connectivity
  └─► teststarttls.go    — STARTTLS upgrade and TLS diagnostics
  └─► testauth.go        — PLAIN, LOGIN, XOAUTH2
  └─► listfolders.go     — list IMAP folders
  └─► listmail.go        — list messages in a folder
  └─► exportmessages.go  — export messages to JSON
  └─► imap_client.go     — IMAP client logic
```

### pop3 (internal/protocols/pop3/)

```
cmd.go
  └─► testconnect.go     — TCP/TLS connectivity
  └─► teststarttls.go    — STLS upgrade and TLS diagnostics
  └─► testauth.go        — USER/PASS, APOP, XOAUTH2
  └─► listmail.go        — retrieve message list
  └─► exportmessages.go  — export messages to JSON
  └─► pop3_client.go     — POP3 client logic
```

### jmap (internal/protocols/jmap/)

```
cmd.go
  └─► testconnect.go     — JMAP session discovery
  └─► testauth.go        — Basic, Bearer
  └─► listfolders.go     — list JMAP mailboxes
  └─► listmail.go        — list messages
  └─► sendmail.go        — send email via JMAP
  └─► exportmessages.go  — export messages to JSON
  └─► jmap_client.go     — HTTP-based JMAP client
```

### ews (internal/protocols/ews/)

```
cmd.go
  └─► testconnect.go   — HTTP/TLS probe; HTTP 401 confirms server alive; TLS version/cipher/cert
  └─► testauth.go      — NTLM, Basic, Bearer (OAuth2); verifies via GetFolder(Inbox)
  └─► getfolder.go     — retrieve Inbox folder properties (display name, total/unread count, folder ID)
  └─► autodiscover.go  — POST GetUserSettings; resolves EWS URLs, user display name, AD server
  └─► listfolders.go   — list EWS folders
  └─► listmail.go      — list messages
  └─► sendmail.go      — send email via EWS
  └─► sendinvite.go    — send calendar invitation
  └─► getevents.go     — list calendar events
  └─► getschedule.go   — get free/busy schedule
  └─► findtimeslot.go  — find available meeting time slots
  └─► exportmessages.go — export messages to JSON
  └─► ews_client.go    — HTTP/SOAP client with NTLM transport (go-ntlmssp), Basic, Bearer auth
  └─► soap_bodies.go   — SOAP request body builders
```

Auth method auto-detection:
- Bearer if `--accesstoken` provided
- NTLM if `--username` contains `\` or `--domain` set
- Basic otherwise

### msgraph (internal/protocols/msgraph/)

```
cmd.go → handlers.go
  ├─► Connectivity
  │   ├─► handleTestConnect()      (testconnect)
  │   └─► handleTestAuth()         (testauth)
  ├─► Calendar
  │   ├─► handleGetEvents()        (getevents)
  │   ├─► handleSendInvite()       (sendinvite)
  │   ├─► handleGetSchedule()      (getschedule)
  │   └─► handleFindTimeSlot()     (findtimeslot)
  ├─► Mail
  │   ├─► handleSendMail()         (sendmail)
  │   ├─► handleGetInbox()         (getinbox)
  │   ├─► handleListFolders()      (listfolders)
  │   └─► handleListMail()         (listmail)
  └─► Export
      ├─► handleExportInbox()        (exportinbox)
      ├─► handleExportMessages()     (exportmessages)
      ├─► handleSearchAndExport()    (searchandexport)
      └─► handleExportBearerToken()  (exportbearertoken)

auth.go → NewGraphServiceClient() → getCredential()
  ├─► azidentity.NewClientSecretCredential()         (--secret / MSGRAPHSECRET)
  ├─► azidentity.NewClientCertificateCredential()
  │   ├─► From PFX file (--pfx + --pfxpass)
  │   └─► From Windows Cert Store (--thumbprint) → cert_windows.go
  ├─► azidentity.NewBearerTokenCredential()          (--bearertoken)
  └─► azidentity delegated flows                      (--delegated --authflow devicecode|browser)

Retry: isRetryableGraphError classifies *odataerrors.ODataError (HTTP 429/503/504,
Graph codes TooManyRequests/activityLimitReached/ServiceUnavailable/GatewayTimeout),
honors Retry-After header (capped at 5 min).

NewGraphServiceClient() is also called by internal/serve/cmd.go at server startup.
```

### gmail (internal/protocols/gmail/)

```
cmd.go → handlers.go
  ├─► Connectivity
  │   ├─► handleTestConnect()      (testconnect)
  │   └─► handleTestAuth()         (testauth)
  ├─► Calendar
  │   ├─► handleGetEvents()        (getevents)
  │   ├─► handleSendInvite()       (sendinvite)
  │   ├─► handleGetSchedule()      (getschedule)
  │   └─► handleFindTimeSlot()     (findtimeslot)
  ├─► Mail
  │   ├─► handleSendMail()         (sendmail)
  │   ├─► handleGetInbox()         (getinbox)
  │   ├─► handleListFolders()      (listfolders)
  │   └─► handleListMail()         (listmail)
  └─► Export
      ├─► handleExportMessages()     (exportmessages)
      └─► handleExportBearerToken()  (exportbearertoken)

auth.go → getCredential()
  ├─► Service account JSON + domain-wide delegation (--credentials + --mailbox)
  ├─► Pre-obtained OAuth2 bearer token              (--bearertoken)
  └─► Interactive loopback OAuth2 flow              (--oauth)
```

### serve (internal/serve/)

```
cmd.go → server.go
  ├── Startup
  │   ├── Requires --api-key / SERVE_API_KEY (fails fast if absent, unless --mcp-stdio)
  │   ├── Loads SMTP base config from SMTP* env vars via smtp.ConfigFromViper()
  │   │   └── SMTPHOST absent → SMTP endpoint returns 503 (server still starts)
  │   ├── Loads MS Graph base config from MSGRAPH* env vars via msgraph.ConfigFromViper()
  │   │   └── Missing TenantID/ClientID → Graph endpoint returns 503
  │   └── msgraph.NewGraphServiceClient() — created once, reused across requests
  │
  ├── Middleware
  │   └── X-API-Key header check on all routes except GET /health
  │
  ├── HTTP Endpoints
  │   ├── GET  /health           → {"status":"ok","version":"4.x.x"}
  │   ├── POST /smtp/sendmail    → smtp_handler.go → send.go → smtp.SendMail()
  │   ├── POST /msgraph/sendmail → msgraph_handler.go → send.go → msgraph.SendEmail()
  │   ├── POST /ews/sendmail     → ews_handler.go → 501 Not Implemented
  │   └── POST /mcp              → mcp.go → Streamable HTTP MCP transport (behind X-API-Key)
  │
  └── MCP (Model Context Protocol)
      ├── Streamable HTTP: POST /mcp (default, --mcp=true, env SERVE_MCP)
      │   └── Behind X-API-Key header; mounted on the same HTTP server
      ├── stdio: --mcp-stdio (env SERVE_MCP_STDIO=true)
      │   └── No HTTP server started; no API key required; stdout re-pointed to stderr
      └── MCP Tools exposed (via send.go core — same logic as REST endpoints):
          ├── smtp_sendmail
          ├── msgraph_sendmail
          └── list_backends
```

Credential model: credentials loaded from env vars at startup; request bodies carry
only message content (to, subject, body, etc.) — no credentials in HTTP requests.

## Shared Internal Packages

```
internal/common/
  ├── bootstrap/     — signal context (SIGINT/SIGTERM), config file loading, logger init
  ├── email/         — email address parsing and validation
  ├── export/        — JSON export helpers (date-stamped directories)
  ├── logger/        — CSV action logs, JSON export, slog structured logger
  ├── mime/          — MIME type detection for attachments
  ├── network/       — ResolveForDial (A/AAAA), LookupMX, ValidateIPVersionFlags
  ├── ratelimit/     — token bucket algorithm
  ├── retry/         — exponential backoff (50ms → 10s cap); protocol-specific classifier hook
  ├── security/      — credential masking (MaskPassword, MaskAccessToken, maskGUID)
  ├── template/      — --template: ParseVars, Render (text/template), IsEML, ParseEML
  ├── timeslot/      — findtimeslot availability analysis (shared by EWS/msgraph/gmail)
  ├── tls/           — TLS certificate display, validation, cipher strength reporting
  ├── validation/    — email, GUID, RFC3339, proxy URL, path, OData injection prevention
  └── version/       — single const Version = "4.0.1"
```

## devtools Subcommand

```
gomailtest devtools env
  ├── get      — print current MSGRAPH* env vars (secrets masked)
  ├── set      — persist MSGRAPH* vars to shell profile / user env
  └── clear    — remove MSGRAPH* vars

gomailtest devtools release
  ├── Step 1: Git status check (working tree must be clean)
  ├── Step 2: Security scan (Azure secrets, GUIDs, emails in source files)
  ├── Step 3: Version bump (update internal/common/version/version.go)
  ├── Step 4: Changelog creation (ChangeLog/{version}.md)
  ├── Step 5: git commit + push
  ├── Step 6: git tag v{version} + push tags
  └── Step 7: GitHub PR + Release via gh CLI
```

## Certificate Authentication Flow (Windows)

```
cert_windows.go (build: windows)
  └─► getCertFromStore(thumbprint)
      ├─► syscall.LoadDLL("crypt32.dll")
      ├─► CertOpenStore(CERT_SYSTEM_STORE_CURRENT_USER)
      ├─► CertFindCertificateInStore(by thumbprint)
      ├─► PFXExportCertStoreEx() → in-memory buffer only
      ├─► pkcs12.DecodeChain()
      └─► returns: crypto.PrivateKey + x509.Certificate
          (no temp files, automatic cleanup via defer)

cert_stub.go (build: !windows)
  └─► getCertFromStore() → always returns unsupported error
```

## Test Suite Architecture

```
Unit tests (go test ./...):
  ├── internal/protocols/smtp/          config_test.go, smtp_client_test.go,
  │                                     sendmail_test.go, sendmail_mime_test.go,
  │                                     sendmail_template_test.go
  ├── internal/protocols/imap/          config_test.go
  ├── internal/protocols/pop3/          config_test.go
  ├── internal/protocols/jmap/          config_test.go, utils_test.go
  ├── internal/protocols/ews/           config_test.go
  ├── internal/protocols/msgraph/       config_test.go, utils_test.go,
  │                                     retry_classification_test.go,
  │                                     empty_result_retry_test.go,
  │                                     handlers_attachments_test.go
  ├── internal/protocols/gmail/         config_test.go
  ├── internal/serve/                   server_test.go, mcp_test.go
  │                                     (middleware, health, EWS 501, SMTP/Graph validation, MCP tools)
  ├── internal/common/bootstrap/        precedence_check_test.go
  ├── internal/common/logger/           json_test.go
  ├── internal/common/email/            email_test.go
  ├── internal/common/mime/             mime_test.go
  ├── internal/common/network/          network_test.go
  ├── internal/common/ratelimit/        ratelimit_test.go
  ├── internal/common/retry/            retry_test.go
  ├── internal/common/security/         masking_test.go
  ├── internal/common/tls/              certificate_test.go, validation_test.go
  ├── internal/common/timeslot/         timeslot_test.go
  ├── internal/common/validation/       validation_test.go, proxy_test.go
  ├── internal/smtp/protocol/           commands_test.go, responses_test.go
  ├── internal/imap/protocol/           capabilities_test.go
  ├── internal/pop3/protocol/           capabilities_test.go, commands_test.go
  └── internal/jmap/protocol/           methods_test.go, session_test.go, types_test.go

Integration tests (go test -tags integration ./tests/integration/):
  └── tests/integration/sendmail_test.go
      └── Requires MSGRAPH* env vars (validated by scripts/check-integration-env.sh)
          └── make integration-test  (or: .\run-integration-tests.ps1)
```

## Data Flow Example: Send Email via msgraph

```
gomailtest msgraph sendmail -mailbox user@example.com -to dest@example.com -subject "Test"
          │
          ▼
internal/protocols/msgraph/cmd.go    — parse flags, validate config
          │
          ▼
internal/protocols/msgraph/auth.go   — getCredential() → azcore.TokenCredential
          │                             msgraphsdk.NewGraphServiceClientWithCredentials()
          ▼
internal/protocols/msgraph/handlers.go — handleSendMail()
  ├── createRecipients(["dest@example.com"])
  ├── createFileAttachments([]) → getAttachmentContentBase64()
  ├── build models.Message
  └── client.Users().ByUserId().SendMail().Post()
          │
          ▼
internal/common/retry/retry.go       — retryWithBackoff()
  ├── isRetryableGraphError() → HTTP 429/503/504, OData throttle codes
  └── exponential backoff: 50ms → 100ms → 200ms → ... → 10s cap
          │
          ▼
internal/common/logger/csv.go        — append to %TEMP%\_msgraphtool_sendmail_{date}.csv
```

## Key Design Patterns

### 1. Cobra Subcommand Pattern

Each protocol registers a `NewCmd()` that returns a `*cobra.Command` with its own subcommands:

```go
func NewCmd() *cobra.Command {
    cmd := &cobra.Command{Use: "smtp", Short: "SMTP testing"}
    cmd.AddCommand(newTestConnectCmd())
    cmd.AddCommand(newTestStartTLSCmd())
    // ...
    return cmd
}
```

### 2. Table-Driven Tests

```go
tests := []struct {
    name     string
    input    string
    expected string
    wantErr  bool
}{ /* cases */ }
```

### 3. Retry with Exponential Backoff

```go
retryWithBackoff(ctx, maxRetries, baseDelay, classifier, operation func() error)
// classifier: nil → generic network errors; non-nil → protocol-specific (e.g. Graph OData errors)
```

### 4. Platform-Specific Builds

```go
// cert_windows.go — //go:build windows  (Windows Certificate Store access)
// cert_stub.go    — //go:build !windows (returns unsupported error)
```

### 5. CSV Logging Pattern

Action-specific files prevent schema conflicts:

```
%TEMP%\_msgraphtool_sendmail_{date}.csv
%TEMP%\_msgraphtool_getevents_{date}.csv
%TEMP%\_smtptool_testconnect_{date}.csv
%TEMP%\_imaptool_listfolders_{date}.csv
%TEMP%\_pop3tool_listmail_{date}.csv
%TEMP%\_jmaptool_getmailboxes_{date}.csv
%TEMP%\_ewstool_testconnect_{date}.csv
%TEMP%\_ewstool_testauth_{date}.csv
%TEMP%\_ewstool_getfolder_{date}.csv
%TEMP%\_ewstool_autodiscover_{date}.csv
%TEMP%\_servetool_smtp-sendmail_{date}.csv
%TEMP%\_servetool_msgraph-sendmail_{date}.csv
```

### 6. HTTP/MCP Serve Pattern

`gomailtest serve` exposes send operations as REST and MCP endpoints:

```
Startup:  load SMTP*/MSGRAPH* env vars → build base configs → init Graph client once
Request:  X-API-Key middleware → decode JSON body → validate → call send.go core → JSON response

POST /smtp/sendmail    body: {to, from?, subject, body}
POST /msgraph/sendmail body: {to, cc?, bcc?, subject, body?, bodyHTML?, attachments?}
POST /mcp              MCP Streamable HTTP — tools: smtp_sendmail, msgraph_sendmail, list_backends
GET  /health           → {"status":"ok","version":"4.x.x"}

MCP stdio:  gomailtest serve --mcp-stdio  (subprocess mode, no HTTP server, no API key)
```

Credentials never appear in request bodies. A missing credential set causes graceful 503
degradation for that endpoint only — the server continues serving other endpoints.

### 7. JSON Export Pattern

Export actions create date-stamped directories:

```
%TEMP%\export\{date}\
  message_1_{timestamp}.json
  message_2_{timestamp}.json
  message_search_{timestamp}.json
```

### 8. Build Tags Pattern

Protocol registration files use build constraints so each binary includes only the
selected protocols while sharing the same entry point:

```go
// protocols_gmail.go
//go:build gmail || !custom

package main
import "github.com/ehlo-pl/gomailtesttool/internal/protocols/gmail"
func init() { rootCmd.AddCommand(gmail.NewCmd()) }
```

---

## Project Statistics

**Version:** 4.0.1 (Latest)
**Last Updated:** 2026-07-19

### Codebase Metrics
- **Binaries:** 3 per platform (`gomailtest`, `gomailtest-exchange`, `gomailtest-gmail`)
- **Protocol subcommands:** 7 (smtp, imap, pop3, jmap, ews, msgraph, gmail) + serve mode
- **Supported Platforms:** Windows (amd64), Linux (amd64), macOS (arm64)
- **Integration Tests:** MS Graph sendmail (tests/integration/)

### Architecture Evolution
- **v1.x:** Single msgraphtool binary
- **v2.0+:** Multi-tool suite (5 separate binaries) with shared internal packages
- **v3.0+:** Unified `gomailtest` binary with cobra subcommands; protocol logic in `internal/protocols/`; `devtools` subcommand replaces PS1 release scripts
- **v3.3+:** Added `ews` subcommand for on-premises Exchange Web Services (NTLM/Basic/Bearer, Autodiscover)
- **v3.3+:** Added `serve` subcommand — HTTP/REST server for triggering sends via API (stdlib `net/http`)
- **v3.5+:** `serve` extended with MCP (Model Context Protocol) over Streamable HTTP and stdio transports
- **v3.6+:** Added `findtimeslot`, `sendinvite`, `getschedule`, `listmail`, `exportmessages` across EWS/msgraph; IMAP/POP3/JMAP gained `teststarttls`, `exportmessages`, `sendmail`
- **v4.0+:** Added `gmail` subcommand (Google Workspace / Gmail API); build-tag system splits protocols into 3 per-platform release binaries

                          ..ooOO END OOoo..
