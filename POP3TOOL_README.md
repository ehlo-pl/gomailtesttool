# POP3 Connectivity Testing Tool (pop3tool)

A command-line tool for testing POP3 server connectivity, authentication, and message retrieval. Part of the **gomailtesttool** suite.

## Overview

The **pop3tool** provides comprehensive POP3 protocol testing and diagnostics:

- **Connection Testing**: TCP connectivity and POP3 capability detection
- **Authentication Testing**: Support for USER/PASS, APOP, and XOAUTH2 mechanisms
- **Message Listing**: List messages in mailbox
- **TLS Support**: POP3S (implicit TLS) and STLS (explicit TLS)
- **Automatic Logging**: CSV/JSON logging for audit and troubleshooting

**Target Use Cases:**
- POP3 server connectivity validation
- Authentication mechanism testing
- Mailbox message enumeration
- Email client troubleshooting
- POP3 infrastructure diagnostics

## Features

- **3 Comprehensive Actions**:
  - `testconnect` - Test TCP connection and display POP3 capabilities
  - `testauth` - Test POP3 authentication
  - `listmail` - List messages in mailbox

- **No External Dependencies**: Pure Go implementation
- **Cross-Platform**: Windows, Linux, macOS
- **CSV/JSON Logging**: Automatic logging to `%TEMP%\_pop3tool_{action}_{date}.csv`
- **OAuth2 Support**: XOAUTH2 authentication for modern providers

## Installation

### Build from Source

```powershell
# Build pop3tool only
go build -C cmd/pop3tool -ldflags="-s -w" -o pop3tool.exe

# Or build all tools
.\build-all.ps1
```

See [BUILD.md](BUILD.md) for detailed build instructions.

## Quick Start

```powershell
# Test basic connectivity
.\pop3tool.exe -action testconnect -host pop.example.com -port 995 -pop3s

# Test authentication
.\pop3tool.exe -action testauth -host pop.example.com -port 995 -pop3s \
    -username user@example.com -password "yourpassword"

# List messages
.\pop3tool.exe -action listmail -host pop.example.com -port 995 -pop3s \
    -username user@example.com -password "yourpassword"
```

## Actions

### 1. testconnect - Basic Connectivity

Tests TCP connection and retrieves POP3 server capabilities.

**What it does:**
- Establishes TCP connection to POP3 server
- Reads server greeting
- Sends CAPA command (if supported)
- Parses and displays server capabilities
- Logs results to CSV

```powershell
# Test POP3S connection (port 995)
.\pop3tool.exe -action testconnect -host pop.gmail.com -port 995 -pop3s

# Test POP3 with STLS (port 110)
.\pop3tool.exe -action testconnect -host pop.example.com -port 110 -starttls
```

**Example Output:**
```
Testing POP3 connectivity to pop.gmail.com:995...

Connected successfully
Server greeting: +OK Gpop ready

Server Capabilities:
  USER
  RESP-CODES
  EXPIRE 0
  LOGIN-DELAY 300
  TOP
  UIDL
  X-GOOGLE-VERHOEVEN
  SASL PLAIN XOAUTH2

Connectivity test completed successfully
```

### 2. testauth - Authentication Testing

Tests POP3 authentication without retrieving messages.

**What it does:**
- Connects to POP3 server
- Upgrades to TLS if using STLS
- Attempts authentication with specified credentials
- Supports USER/PASS, APOP, and XOAUTH2 mechanisms
- Logs authentication result

```powershell
# Test with password (USER/PASS)
.\pop3tool.exe -action testauth -host pop.example.com -port 995 -pop3s \
    -username user@example.com -password "yourpassword"

# Test with OAuth2 token
.\pop3tool.exe -action testauth -host pop.gmail.com -port 995 -pop3s \
    -username user@gmail.com -accesstoken "ya29.xxx..."

# Specify authentication method
.\pop3tool.exe -action testauth -host pop.example.com -port 995 -pop3s \
    -username user@example.com -password "secret" -authmethod USER
```

**Example Output:**
```
Testing POP3 authentication on pop.example.com:995...

Connected (POP3S)

Attempting authentication with method: USER/PASS

Authentication successful
Logged in as: user@example.com

Authentication test completed successfully
```

### 3. listmail - List Messages

Lists messages in the authenticated mailbox.

**What it does:**
- Connects and authenticates to POP3 server
- Sends STAT and LIST commands
- Displays message count and sizes
- Logs message information

```powershell
# List messages (default max 100)
.\pop3tool.exe -action listmail -host pop.example.com -port 995 -pop3s \
    -username user@example.com -password "yourpassword"

# List up to 50 messages
.\pop3tool.exe -action listmail -host pop.example.com -port 995 -pop3s \
    -username user@example.com -password "yourpassword" -maxmessages 50
```

**Example Output:**
```
Listing messages for user@example.com on pop.example.com:995...

Connected and authenticated

Mailbox Statistics:
  Total messages: 42
  Total size: 15,234,567 bytes

Message List (showing first 42):
  1. 12,345 bytes
  2. 8,901 bytes
  3. 23,456 bytes
  ...

Message listing completed successfully
```

## Command-Line Flags

### Core Flags

| Flag | Description | Environment Variable | Default |
|------|-------------|---------------------|---------|
| `-action` | Action to perform (required) | `POP3ACTION` | - |
| `-host` | POP3 server hostname (required) | `POP3HOST` | - |
| `-port` | POP3 server port | `POP3PORT` | 110 |
| `-timeout` | Connection timeout (seconds) | `POP3TIMEOUT` | 30 |

### Authentication Flags

| Flag | Description | Environment Variable |
|------|-------------|---------------------|
| `-username` | Username for authentication | `POP3USERNAME` |
| `-password` | Password for authentication | `POP3PASSWORD` |
| `-accesstoken` | OAuth2 access token for XOAUTH2 | `POP3ACCESSTOKEN` |
| `-authmethod` | Auth method: auto, USER, APOP, XOAUTH2 | `POP3AUTHMETHOD` |

### List Options

| Flag | Description | Environment Variable | Default |
|------|-------------|---------------------|---------|
| `-maxmessages` | Maximum messages to list | `POP3MAXMESSAGES` | 100 |

### TLS Flags

| Flag | Description | Environment Variable | Default |
|------|-------------|---------------------|---------|
| `-pop3s` | Use POP3S (implicit TLS on port 995) | `POP3POP3S` | false |
| `-starttls` | Force STLS upgrade | `POP3STARTTLS` | false |
| `-skipverify` | Skip TLS certificate verification | `POP3SKIPVERIFY` | false |
| `-tlsversion` | TLS version: 1.2, 1.3 | `POP3TLSVERSION` | 1.2 |

### Network Flags

| Flag | Description | Environment Variable | Default |
|------|-------------|---------------------|---------|
| `-proxy` | Proxy URL | `POP3PROXY` | - |
| `-maxretries` | Maximum retry attempts | `POP3MAXRETRIES` | 3 |
| `-retrydelay` | Retry delay (milliseconds) | `POP3RETRYDELAY` | 2000 |
| `-ratelimit` | Rate limit (requests/second, 0=unlimited) | `POP3RATELIMIT` | 0 |

### Runtime Flags

| Flag | Description | Environment Variable | Default |
|------|-------------|---------------------|---------|
| `-verbose` | Enable verbose output | - | false |
| `-loglevel` | Log level: DEBUG, INFO, WARN, ERROR | - | INFO |
| `-output` | Output format: text, json | `POP3OUTPUT` | text |
| `-logformat` | Log file format: csv, json | `POP3LOGFORMAT` | csv |
| `-version` | Show version information | - | - |

## Environment Variables

All flags can be set via environment variables with the `POP3` prefix:

```powershell
# Windows PowerShell
$env:POP3HOST = "pop.example.com"
$env:POP3PORT = "995"
$env:POP3POP3S = "true"
$env:POP3USERNAME = "user@example.com"
$env:POP3PASSWORD = "yourpassword"

.\pop3tool.exe -action testauth

# Linux/macOS Bash
export POP3HOST="pop.example.com"
export POP3PORT="995"
export POP3POP3S="true"
export POP3USERNAME="user@example.com"
export POP3PASSWORD="yourpassword"

./pop3tool -action testauth
```

**Note:** Command-line flags take precedence over environment variables.

## Common POP3 Ports

| Port | Usage | TLS |
|------|-------|-----|
| 110 | POP3 (standard) | Optional STLS |
| 995 | POP3S (implicit TLS) | Implicit TLS |

**Recommendations:**
- **Port 995 with `-pop3s`**: Use for most modern POP3 servers
- **Port 110 with `-starttls`**: Use when explicit TLS upgrade is required

## POP3S vs STLS

| Method | Port | Flag | Description |
|--------|------|------|-------------|
| **POP3S** | 995 | `-pop3s` | Implicit TLS - encryption starts immediately |
| **STLS** | 110 | `-starttls` | Explicit TLS - plain connection upgrades after STLS |

```powershell
# POP3S (implicit TLS on port 995)
.\pop3tool.exe -action testconnect -host pop.gmail.com -pop3s

# STLS (explicit TLS on port 110)
.\pop3tool.exe -action testconnect -host pop.example.com -port 110 -starttls
```

**Note:** Cannot use both `-pop3s` and `-starttls` together.

## Common Provider Configurations

| Provider | Host | Port | Method |
|----------|------|------|--------|
| Gmail | pop.gmail.com | 995 | POP3S |
| Microsoft 365 | outlook.office365.com | 995 | POP3S |
| Yahoo | pop.mail.yahoo.com | 995 | POP3S |

```powershell
# Gmail
.\pop3tool.exe -action testauth -host pop.gmail.com -pop3s \
    -username user@gmail.com -password "app-password"

# Microsoft 365
.\pop3tool.exe -action testauth -host outlook.office365.com -pop3s \
    -username user@company.com -password "password"

# Gmail with OAuth2
.\pop3tool.exe -action testauth -host pop.gmail.com -pop3s \
    -username user@gmail.com -accesstoken "ya29.xxx..."
```

## CSV Logging

All operations are automatically logged to action-specific CSV files:

**Location:** `%TEMP%\_pop3tool_{action}_{date}.csv`

**Examples:**
- `C:\Users\username\AppData\Local\Temp\_pop3tool_testconnect_2026-01-31.csv`
- `C:\Users\username\AppData\Local\Temp\_pop3tool_testauth_2026-01-31.csv`

## Troubleshooting

### Connection Issues

**"Connection refused"**
- Verify server hostname and port
- Check if POP3S (995) or POP3 (110) is required
- Verify firewall rules allow outbound connections

**"Connection timeout"**
- Increase `-timeout` value
- Check network connectivity
- Verify DNS resolution

### TLS Issues

**"Cannot use both -pop3s and -starttls"**
- Choose one TLS method:
  - `-pop3s` for port 995 (implicit TLS)
  - `-starttls` for port 110 (explicit TLS)

**"TLS handshake failed"**
- Try `-skipverify` temporarily (insecure, for testing only)
- Check server certificate validity
- Try different TLS version: `-tlsversion 1.3`

### Authentication Issues

**"Authentication failed"**
- Verify username and password
- For Gmail: Use app-specific password (not regular password)
- Some servers require full email address as username

**"XOAUTH2 authentication requires -accesstoken"**
- Provide OAuth2 access token via `-accesstoken` flag
- Ensure token is valid and not expired

## Security Best Practices

1. **Credential Management**:
   - Use environment variables for passwords
   - Never commit credentials to version control
   - Use OAuth2 tokens when available

2. **TLS Configuration**:
   - Always use TLS (POP3S or STLS)
   - Never use `-skipverify` in production
   - Use TLS 1.2 or higher

3. **Logging**:
   - CSV logs may contain usernames
   - Secure log files appropriately
   - Passwords are NOT logged

## Related Documentation

- **Build Instructions**: [BUILD.md](BUILD.md)
- **SMTP Tool**: [SMTP_TOOL_README.md](SMTP_TOOL_README.md)
- **IMAP Tool**: [IMAPTOOL_README.md](IMAPTOOL_README.md)
- **Security Policy**: [SECURITY.md](SECURITY.md)

## Support

**Repository:** [https://github.com/ehlo-pl/gomailtesttool](https://github.com/ehlo-pl/gomailtesttool)

                          ..ooOO END OOoo..
