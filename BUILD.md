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

## Note on Legacy Binaries

The individual shim binaries (`smtptool`, `imaptool`, `pop3tool`, `jmaptool`, `msgraphtool`) that existed in v3.0.x have been removed in v3.1.0. Use `gomailtest <protocol> <action>` instead.

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

## Build Flags Explained

| Flag | Description |
|------|-------------|
| `-o <file>` | Output executable name |
| `-ldflags="-s -w"` | Strip debug info and symbol table (~30% smaller binary) |
| `-v` | Verbose build output |
| `-race` | Enable race detector (development only) |

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
