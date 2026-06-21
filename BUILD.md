# Build Instructions

This document provides instructions for building the tools in this repository:
- **msgraphtool**: Microsoft Graph API tool for Exchange Online
- **smtptool**: SMTP connectivity testing tool
- **imaptool**: IMAP connectivity testing tool
- **pop3tool**: POP3 connectivity testing tool
- **jmaptool**: JMAP protocol testing tool

## Prerequisites

1. **Go 1.24+**: [Download Go](https://golang.org/dl/)
2. **Git**: [Download Git](https://git-scm.com/downloads)

## Quick Build (All Tools)

The easiest way to build all tools is using the build script:

```powershell
# From project root
.\build-all.ps1
```

This creates all executables in the `bin/` directory:
- `bin/msgraphtool.exe`
- `bin/smtptool.exe`
- `bin/imaptool.exe`
- `bin/pop3tool.exe`
- `bin/jmaptool.exe`

## Individual Tool Builds

### Microsoft Graph Tool

```powershell
# Standard build (outputs to bin/)
go build -C cmd/msgraphtool -o bin/msgraphtool.exe

# Optimized build (recommended for production)
go build -C cmd/msgraphtool -ldflags="-s -w" -o bin/msgraphtool.exe
```

### SMTP Tool

```powershell
# Standard build (outputs to bin/)
go build -C cmd/smtptool -o bin/smtptool.exe

# Optimized build (recommended for production)
go build -C cmd/smtptool -ldflags="-s -w" -o bin/smtptool.exe
```

## Cross-Platform Builds

All tools support Windows, Linux, and macOS.

### Build for Linux

```powershell
# Microsoft Graph Tool
$env:GOOS="linux"; $env:GOARCH="amd64"
go build -C cmd/msgraphtool -ldflags="-s -w" -o bin/msgraphtool
Remove-Item Env:\GOOS; Remove-Item Env:\GOARCH

# SMTP Tool
$env:GOOS="linux"; $env:GOARCH="amd64"
go build -C cmd/smtptool -ldflags="-s -w" -o bin/smtptool
Remove-Item Env:\GOOS; Remove-Item Env:\GOARCH
```

**Note:** Windows Certificate Store authentication (`-thumbprint` flag) is only available on Windows builds.

### Build for macOS

```powershell
# Microsoft Graph Tool
$env:GOOS="darwin"; $env:GOARCH="amd64"
go build -C cmd/msgraphtool -ldflags="-s -w" -o bin/msgraphtool
Remove-Item Env:\GOOS; Remove-Item Env:\GOARCH

# SMTP Tool (Apple Silicon)
$env:GOOS="darwin"; $env:GOARCH="arm64"
go build -C cmd/smtptool -ldflags="-s -w" -o bin/smtptool
Remove-Item Env:\GOOS; Remove-Item Env:\GOARCH
```

## Project Structure

The repository now uses a modular structure:

```
gomailtesttool/
├── bin/                 # Build output directory (executables)
├── cmd/
│   ├── msgraphtool/     # Microsoft Graph tool source
│   ├── smtptool/        # SMTP tool source
│   ├── imaptool/        # IMAP tool source
│   ├── pop3tool/        # POP3 tool source
│   └── jmaptool/        # JMAP tool source
├── internal/
│   ├── common/          # Shared packages (logger, retry, version, validation)
│   ├── imap/            # IMAP-specific packages
│   ├── jmap/            # JMAP-specific packages
│   ├── pop3/            # POP3-specific packages
│   └── smtp/            # SMTP-specific code (protocol, TLS, Exchange)
├── build-all.ps1        # Build script for all tools
└── go.mod               # Root module
```

## Verification

After building, verify the executables:

```powershell
# Check versions
.\bin\msgraphtool.exe -version
.\bin\smtptool.exe -version

# All tools should display the same version from internal/common/version/version.go
```

## Release Process

> **Complete Guide:** See **[RELEASE.md](RELEASE.md)** for the full release and versioning policy.

To prepare a release:

```powershell
.\build-all.ps1
```

Then follow the version bump, changelog, commit, and tag steps in **[RELEASE.md](RELEASE.md)**.

## Troubleshooting

### Common Issues

**"go: command not found"**
- Ensure Go is installed and in your PATH
- Run `go version` to verify

**"package X is not in GOROOT"**
- Run `go mod download` from project root
- Ensure you're using Go 1.24 or later

**"Access Denied" on Windows**
- Close any running instances of the tools
- Build to a different filename temporarily

**Build fails with import errors**
- Ensure you're building from project root
- Check that `internal/` packages exist
- Run `go mod tidy` to clean up dependencies

### Module Cache Issues

If you encounter module resolution issues:

```powershell
# Clean module cache
go clean -modcache

# Re-download dependencies
go mod download
```

## Development

### Run Without Building

```powershell
# Microsoft Graph Tool
cd cmd/msgraphtool
go run . -action getinbox -tenantid "..." -clientid "..." -secret "..." -mailbox "user@example.com"

# SMTP Tool
cd cmd/smtptool
go run . -action testconnect -host smtp.example.com -port 25
```

### Run Tests

```powershell
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests in a specific package
go test ./internal/smtp/protocol/
```

### Code Linting

```powershell
# Install golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run linter
golangci-lint run
```

## Build Flags Explained

- `-C <dir>`: Change to directory before building
- `-o <file>`: Output executable name
- `-ldflags="-s -w"`: Strip debug info and symbol table (reduces binary size by ~30%)
- `-v`: Verbose build output
- `-race`: Enable race detector (development only, increases binary size)

## Binary Sizes

Typical optimized build sizes (Windows):
- **msgraphtool.exe**: ~15-20 MB (includes Graph SDK)
- **smtptool.exe**: ~8-10 MB (stdlib only, no external dependencies)

## Additional Resources

- **Usage Examples**: See [EXAMPLES.md](EXAMPLES.md) for Graph tool examples
- **SMTP Tool Guide**: See [SMTP_TOOL_README.md](SMTP_TOOL_README.md) for SMTP tool documentation
- **Security**: See [SECURITY.md](SECURITY.md) for security best practices
- **Project Overview**: See [AGENTS.md](AGENTS.md) for complete project documentation

                          ..ooOO END OOoo..
