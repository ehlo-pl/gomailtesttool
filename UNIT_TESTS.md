# Unit Tests Documentation

## Overview

Unit tests for **gomailtesttool** are distributed across packages that follow the module structure. All tests run from the project root without needing to `cd` into subdirectories.

## Running Tests

### Run All Unit Tests

```bash
go test ./...

# With verbose output
go test -v ./...

# With coverage
go test -cover ./...

# With coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out   # Open in browser
go tool cover -func=coverage.out   # Text summary
```

### Run Specific Package

```bash
go test ./internal/protocols/smtp/
go test ./internal/protocols/imap/
go test ./internal/protocols/pop3/
go test ./internal/protocols/jmap/
go test ./internal/protocols/ews/
go test ./internal/protocols/msgraph/
go test ./internal/smtp/protocol/
go test ./internal/jmap/protocol/
```

### Run Specific Test

```bash
go test -v -run TestValidateConfiguration ./internal/protocols/smtp/
go test -v -run TestMaskSecret ./internal/protocols/msgraph/
```

## Test Package Structure

```
internal/
├── common/
│   ├── logger/         - CSV/slog logger tests
│   ├── ratelimit/      - Rate limiter tests
│   ├── security/       - Security function tests
│   └── validation/     - Input validation tests
├── protocols/
│   ├── ews/            - EWS config, TLS, auth resolution tests
│   ├── imap/           - IMAP Cobra config + validation tests
│   ├── jmap/           - JMAP Cobra config + validation tests
│   ├── msgraph/        - msgraph config, utils, handlers tests
│   ├── pop3/           - POP3 config + validation tests (planned)
│   └── smtp/           - SMTP config, utils, client tests
├── imap/protocol/      - IMAP wire protocol tests
├── jmap/protocol/      - JMAP session/mailbox parsing tests
├── pop3/protocol/      - POP3 wire protocol tests
└── smtp/
    └── protocol/       - SMTP protocol tests
```

## Key Test Areas

### Protocol Config Validation (`internal/protocols/*/config_test.go`)

Each protocol has a `TestValidateConfiguration` suite covering:
- Valid and invalid action names
- Host validation (required, format)
- Port validation (range 1-65535)
- Authentication method validation
- Credential requirements per action
- Log level and log format validation

### Security Functions (`internal/common/security/`)

- `sanitizeCRLF()` — SMTP command injection prevention
- `sanitizeEmailHeader()` — Email header injection prevention

### Credential Masking (`internal/protocols/*/utils_test.go`)

Each protocol has tests for:
- `maskUsername()` — Shows `us****om` format
- `maskPassword()` — Shows `pa****rd` format
- `maskAccessToken()` — Shows `ya29.a0A...ghij` format

### Rate Limiter (`internal/common/ratelimit/`)

- Token bucket algorithm
- Fractional rates (0.5 req/s = 1 request per 2 seconds)
- Context cancellation
- Zero rate (unlimited)

### Validation (`internal/common/validation/`)

- `ValidateEmail()` — RFC 5322 format
- `ValidateGUID()` — GUID/UUID format
- `ValidateHostname()` — Hostname and IP format
- `ValidatePort()` — Port range validation
- `ValidateProxyURL()` — Proxy URL format and scheme

### JMAP Protocol (`internal/jmap/protocol/`)

- Session parsing from JSON
- Mailbox/get response parsing
- Discovery URL construction
- Capability detection

### SMTP Protocol (`internal/smtp/protocol/`, `internal/protocols/smtp/`)

- SMTP client unit tests
- Send mail construction
- Config validation
- Utils (CRLF sanitization, masking)

### EWS Protocol (`internal/protocols/ews/`)

- `TestResolveAuthMethod` — verifies auth auto-detection (Bearer, NTLM, Basic)
- `TestConfigFromViper` — verifies defaults and value overrides from Viper
- `TestValidateConfiguration` — action validation, host/port validation, credential requirements per action
- `TestBuildTLSConfig` — TLS version and skip-verify flag handling
- `TestCertCSVFields` — certificate field extraction for CSV logging

Run:
```bash
go test ./internal/protocols/ews/
go test -v -run TestValidateConfiguration ./internal/protocols/ews/
go test -v -run TestResolveAuthMethod ./internal/protocols/ews/
```

## Test Patterns

### Table-Driven Tests

All protocol config tests use table-driven patterns:

```go
func TestValidateConfiguration_AuthMethod(t *testing.T) {
    tests := []struct {
        name       string
        authMethod string
        wantErr    bool
    }{
        {"auto", "auto", false},
        {"basic", "basic", false},
        {"uppercase AUTO", "AUTO", false}, // normalized by validateConfiguration
        {"invalid", "oauth", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            config := newTestConfig()
            config.AuthMethod = tt.authMethod
            err := validateConfiguration(config)
            if (err != nil) != tt.wantErr {
                t.Errorf("validateConfiguration() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

### Test Helper

Each protocol config_test.go exports a `newTestConfig()` helper:

```go
func newTestConfig() *Config {
    return &Config{
        Action:     ActionTestConnect,
        Host:       "smtp.example.com",
        Port:       25,
        AuthMethod: "auto",
        LogLevel:   "info",
        LogFormat:  "csv",
    }
}
```

## CI/CD Integration

### GitHub Actions

```yaml
name: Tests

on:
  push:
    branches: [ main ]
  pull_request:
    branches: [ main ]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'
          cache: true
      - run: go test ./...
      - run: go test -coverprofile=coverage.out ./...
      - run: go tool cover -func=coverage.out
```

### Pre-commit Hook

Create `.git/hooks/pre-commit`:

```bash
#!/bin/bash
echo "Running unit tests..."
go test ./...
if [ $? -ne 0 ]; then
    echo "Unit tests failed. Commit aborted."
    exit 1
fi
echo "All tests passed."
```

```bash
chmod +x .git/hooks/pre-commit
```

## Related Documentation

- [INTEGRATION_TESTS.md](INTEGRATION_TESTS.md) — Integration test guide
- [BUILD.md](BUILD.md) — Build instructions
- [docs/protocols/](docs/protocols/) — Per-protocol documentation
- [docs/protocols/ews.md](docs/protocols/ews.md) — EWS documentation

                          ..ooOO END OOoo..
