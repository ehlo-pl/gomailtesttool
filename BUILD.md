# Build Instructions

## Prerequisites

1. **Go 1.24+**: [Download Go](https://golang.org/dl/)
2. **Git**: [Download Git](https://git-scm.com/downloads)

## Quick Build (All Tools)

```powershell
# Build all tools at once
.\build-all.ps1
```

This creates `bin/gomailtest.exe`.

## Primary Binary

The main binary is `gomailtest`. All protocol commands live under it:

```powershell
# Standard build
go build -o bin/gomailtest.exe ./cmd/gomailtest

# Optimized build (recommended for production)
go build -ldflags="-s -w" -o bin/gomailtest.exe ./cmd/gomailtest
```

## Cross-Platform Builds

### Build for Linux

```powershell
$env:GOOS="linux"; $env:GOARCH="amd64"
go build -ldflags="-s -w" -o bin/gomailtest ./cmd/gomailtest
Remove-Item Env:\GOOS; Remove-Item Env:\GOARCH
```

**Note:** Windows Certificate Store authentication (`--thumbprint` flag) is only available on Windows builds.

### Build for macOS

```powershell
# Intel
$env:GOOS="darwin"; $env:GOARCH="amd64"
go build -ldflags="-s -w" -o bin/gomailtest ./cmd/gomailtest
Remove-Item Env:\GOOS; Remove-Item Env:\GOARCH

# Apple Silicon
$env:GOOS="darwin"; $env:GOARCH="arm64"
go build -ldflags="-s -w" -o bin/gomailtest ./cmd/gomailtest
Remove-Item Env:\GOOS; Remove-Item Env:\GOARCH
```

## Project Structure

```
gomailtesttool/
├── bin/                          # Build output directory
├── cmd/
│   └── gomailtest/               # Unified CLI entry point
├── internal/
│   ├── common/                   # Shared packages (bootstrap, logger, version, validation)
│   ├── protocols/                # Protocol implementations (Cobra commands + logic)
│   │   ├── imap/
│   │   ├── jmap/
│   │   ├── msgraph/
│   │   ├── pop3/
│   │   └── smtp/
│   ├── imap/protocol/            # IMAP wire protocol
│   ├── jmap/protocol/            # JMAP types and session parsing
│   ├── pop3/protocol/            # POP3 wire protocol
│   └── smtp/                     # SMTP protocol, TLS, Exchange detection
├── docs/protocols/               # Per-protocol documentation
├── build-all.ps1                 # Build script for all binaries
└── go.mod
```

## Verification

```powershell
# Check version
.\bin\gomailtest.exe --version

# Show available protocols
.\bin\gomailtest.exe --help
```

## Run Without Building

```powershell
# Run directly
go run ./cmd/gomailtest smtp testconnect --host smtp.example.com --port 25

go run ./cmd/gomailtest msgraph getinbox \
    --tenantid "..." --clientid "..." --secret "..." --mailbox "user@example.com"
```

## Run Tests

```powershell
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests for a specific protocol
go test ./internal/protocols/smtp/
go test ./internal/smtp/protocol/
```

## Code Linting

```powershell
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
golangci-lint run
```

## Protocol-Subset Builds

By default, `go build` compiles **all** protocols into the binary. You can produce
a smaller binary containing only the protocols you need by using Go build tags.

### How it works

A special `custom` meta-tag enables selective compilation. Each protocol has its
own build constraint (`smtp || !custom`, `imap || !custom`, etc.), so:

- **No tags** (default): every protocol is included.
- **`-tags custom`**: only protocols whose tag is explicitly listed are included.

Valid protocol tags: `smtp`, `imap`, `pop3`, `jmap`, `ews`, `gmail`, `msgraph`

> The `devtools` and `serve` sub-commands are always compiled in regardless of
> the protocol selection.

### Using Make

```bash
# SMTP + IMAP + POP3 → bin/gomailtest-smtp-imap-pop3
make build-custom PROTOCOLS="smtp imap pop3"

# SMTP only → bin/gomailtest-smtp
make build-smtp-only

# Microsoft Graph only → bin/gomailtest-msgraph
make build-msgraph-only

# SMTP + IMAP + POP3 standard subset → bin/gomailtest-smtp-imap-pop3
make build-standard-only
```

### Using PowerShell

```powershell
# SMTP + IMAP only → bin/gomailtest-smtp-imap.exe
.\build-all.ps1 -Protocols "smtp imap"

# Microsoft Graph + Gmail → bin/gomailtest-msgraph-gmail.exe
.\build-all.ps1 -Protocols "msgraph gmail"
```

### Direct `go build`

```bash
# SMTP + JMAP + EWS
go build -tags "custom smtp jmap ews" -ldflags="-s -w" -trimpath \
    -o bin/gomailtest-smtp-jmap-ews ./cmd/gomailtest
```

### Binary naming convention

The output binary is named `gomailtest-<protocol1>-<protocol2>-...` (protocols
in the order provided). The default full build keeps the name `gomailtest`.

## Build Flags Explained

| Flag | Description |
|------|-------------|
| `-o <file>` | Output executable name |
| `-ldflags="-s -w"` | Strip debug info and symbol table (~31% smaller binary) |
| `-trimpath` | Remove source paths for reproducible builds (~0.3% additional reduction) |
| `-v` | Verbose build output |
| `-race` | Enable race detector (development only) |

## Binary Size Optimization

The build is optimized for size and reproducibility:
- **Current size:** ~47.46 MB (stripped + trimpath)
- **Unstripped size:** ~69 MB (for comparison)
- **Size reduction:** ~31% via stripping debug symbols

For detailed analysis, see [BINARY_OPTIMIZATION_REPORT.md](BINARY_OPTIMIZATION_REPORT.md).

### Why These Sizes?

The tool integrates 8 email protocols (SMTP, IMAP, POP3, JMAP, Gmail API, Microsoft Graph, EWS) plus cloud SDKs:
- Microsoft Graph SDK + Azure SDK: ~12-18% of binary
- Google APIs + gRPC: ~10-15% of binary
- Protocol libraries and authentication (OAuth, Kerberos, NTLM): ~15-20%
- Standard library and utilities: ~30-40%

All dependencies are necessary for functionality. Further reduction would require sacrificing features.

## Troubleshooting

**"go: command not found"**
- Ensure Go is installed and in your PATH (`go version` to verify)

**"package X is not in GOROOT"**
- Run `go mod download` from project root
- Ensure Go 1.24 or later

**"Access Denied" on Windows**
- Close any running instances of the tool
- Build to a different output path temporarily

**Module cache issues:**
```powershell
go clean -modcache
go mod download
```

## Additional Resources

- [README.md](README.md) — Overview and quick start
- [docs/protocols/](docs/protocols/) — Per-protocol documentation
- [SECURITY.md](SECURITY.md) — Security best practices
- [RELEASE.md](RELEASE.md) — Release and versioning policy

                          ..ooOO END OOoo..
