# IMAP Connectivity Testing Tool (imaptool)

A command-line tool for testing IMAP server connectivity, authentication, and mailbox operations. Part of the **gomailtesttool** suite.

## Overview

The **imaptool** provides comprehensive IMAP protocol testing and diagnostics:

- **Connection Testing**: TCP connectivity and IMAP capability detection
- **Authentication Testing**: Support for PLAIN, LOGIN, and XOAUTH2 mechanisms
- **Folder Operations**: List mailbox folders
- **TLS Support**: IMAPS (implicit TLS) and STARTTLS (explicit TLS)
- **Automatic Logging**: CSV/JSON logging for audit and troubleshooting

**Target Use Cases:**
- IMAP server connectivity validation
- Authentication mechanism testing
- Mailbox folder enumeration
- Email client troubleshooting
- IMAP infrastructure diagnostics

## Features

- **3 Comprehensive Actions**:
  - `testconnect` - Test TCP connection and display IMAP capabilities
  - `testauth` - Test IMAP authentication
  - `listfolders` - List mailbox folders

- **No External Dependencies**: Pure Go implementation
- **Cross-Platform**: Windows, Linux, macOS
- **CSV/JSON Logging**: Automatic logging to `%TEMP%\_imaptool_{action}_{date}.csv`
- **OAuth2 Support**: XOAUTH2 authentication for modern providers

## Installation

### Build from Source

```powershell
# Build imaptool only
go build -C cmd/imaptool -ldflags="-s -w" -o imaptool.exe

# Or build all tools
.\build-all.ps1
```

See [BUILD.md](BUILD.md) for detailed build instructions.

## Quick Start

```powershell
# Test basic connectivity
.\imaptool.exe -action testconnect -host imap.example.com -port 993 -imaps

# Test authentication
.\imaptool.exe -action testauth -host imap.example.com -port 993 -imaps \
    -username user@example.com -password "yourpassword"

# List mailbox folders
.\imaptool.exe -action listfolders -host imap.example.com -port 993 -imaps \
    -username user@example.com -password "yourpassword"
```

## Actions

### 1. testconnect - Basic Connectivity

Tests TCP connection and retrieves IMAP server capabilities.

**What it does:**
- Establishes TCP connection to IMAP server
- Reads server greeting
- Sends CAPABILITY command
- Parses and displays server capabilities
- Logs results to CSV

```powershell
# Test IMAPS connection (port 993)
.\imaptool.exe -action testconnect -host imap.gmail.com -port 993 -imaps

# Test IMAP with STARTTLS (port 143)
.\imaptool.exe -action testconnect -host imap.example.com -port 143 -starttls
```

**Example Output:**
```
Testing IMAP connectivity to imap.gmail.com:993...

Connected successfully
Server greeting: * OK Gimap ready

Server Capabilities:
  IMAP4rev1
  UNSELECT
  IDLE
  NAMESPACE
  QUOTA
  ID
  XLIST
  CHILDREN
  X-GM-EXT-1
  XYZZY
  SASL-IR
  AUTH=XOAUTH2
  AUTH=PLAIN
  AUTH=OAUTHBEARER

Connectivity test completed successfully
```

### 2. testauth - Authentication Testing

Tests IMAP authentication without performing mailbox operations.

**What it does:**
- Connects to IMAP server
- Upgrades to TLS if using STARTTLS
- Detects supported AUTH mechanisms
- Attempts authentication with specified credentials
- Supports PLAIN, LOGIN, and XOAUTH2 mechanisms
- Logs authentication result

```powershell
# Test with password
.\imaptool.exe -action testauth -host imap.example.com -port 993 -imaps \
    -username user@example.com -password "yourpassword"

# Test with OAuth2 token
.\imaptool.exe -action testauth -host imap.gmail.com -port 993 -imaps \
    -username user@gmail.com -accesstoken "ya29.xxx..."

# Specify authentication method
.\imaptool.exe -action testauth -host imap.example.com -port 993 -imaps \
    -username user@example.com -password "secret" -authmethod PLAIN
```

**Example Output:**
```
Testing IMAP authentication on imap.example.com:993...

Connected (IMAPS)
Server supports AUTH mechanisms: PLAIN, LOGIN, XOAUTH2

Attempting authentication with method: PLAIN

Authentication successful
Logged in as: user@example.com

Authentication test completed successfully
```

### 3. listfolders - List Mailbox Folders

Lists all folders in the authenticated mailbox.

**What it does:**
- Connects and authenticates to IMAP server
- Sends LIST command to enumerate folders
- Displays folder hierarchy
- Logs folder information

```powershell
# List folders
.\imaptool.exe -action listfolders -host imap.example.com -port 993 -imaps \
    -username user@example.com -password "yourpassword"
```

**Example Output:**
```
Listing folders for user@example.com on imap.example.com:993...

Connected and authenticated

Mailbox Folders:
  INBOX
  Sent
  Drafts
  Trash
  Spam
  [Gmail]/All Mail
  [Gmail]/Starred
  [Gmail]/Important

Total folders: 8

Folder listing completed successfully
```

## Command-Line Flags

### Core Flags

| Flag | Description | Environment Variable | Default |
|------|-------------|---------------------|---------|
| `-action` | Action to perform (required) | `IMAPACTION` | - |
| `-host` | IMAP server hostname (required) | `IMAPHOST` | - |
| `-port` | IMAP server port | `IMAPPORT` | 143 |
| `-timeout` | Connection timeout (seconds) | `IMAPTIMEOUT` | 30 |

### Authentication Flags

| Flag | Description | Environment Variable |
|------|-------------|---------------------|
| `-username` | Username for authentication | `IMAPUSERNAME` |
| `-password` | Password for authentication | `IMAPPASSWORD` |
| `-accesstoken` | OAuth2 access token for XOAUTH2 | `IMAPACCESSTOKEN` |
| `-authmethod` | Auth method: auto, PLAIN, LOGIN, XOAUTH2 | `IMAPAUTHMETHOD` |

### TLS Flags

| Flag | Description | Environment Variable | Default |
|------|-------------|---------------------|---------|
| `-imaps` | Use IMAPS (implicit TLS on port 993) | `IMAPIMAPS` | false |
| `-starttls` | Force STARTTLS upgrade | `IMAPSTARTTLS` | false |
| `-skipverify` | Skip TLS certificate verification | `IMAPSKIPVERIFY` | false |
| `-tlsversion` | TLS version: 1.2, 1.3 | `IMAPTLSVERSION` | 1.2 |

### Network Flags

| Flag | Description | Environment Variable | Default |
|------|-------------|---------------------|---------|
| `-proxy` | Proxy URL | `IMAPPROXY` | - |
| `-maxretries` | Maximum retry attempts | `IMAPMAXRETRIES` | 3 |
| `-retrydelay` | Retry delay (milliseconds) | `IMAPRETRYDELAY` | 2000 |
| `-ratelimit` | Rate limit (requests/second, 0=unlimited) | `IMAPRATELIMIT` | 0 |

### Runtime Flags

| Flag | Description | Environment Variable | Default |
|------|-------------|---------------------|---------|
| `-verbose` | Enable verbose output | - | false |
| `-loglevel` | Log level: DEBUG, INFO, WARN, ERROR | - | INFO |
| `-output` | Output format: text, json | `IMAPOUTPUT` | text |
| `-logformat` | Log file format: csv, json | `IMAPLOGFORMAT` | csv |
| `-version` | Show version information | - | - |

## Environment Variables

All flags can be set via environment variables with the `IMAP` prefix:

```powershell
# Windows PowerShell
$env:IMAPHOST = "imap.example.com"
$env:IMAPPORT = "993"
$env:IMAPIMAPS = "true"
$env:IMAPUSERNAME = "user@example.com"
$env:IMAPPASSWORD = "yourpassword"

.\imaptool.exe -action testauth

# Linux/macOS Bash
export IMAPHOST="imap.example.com"
export IMAPPORT="993"
export IMAPIMAPS="true"
export IMAPUSERNAME="user@example.com"
export IMAPPASSWORD="yourpassword"

./imaptool -action testauth
```

**Note:** Command-line flags take precedence over environment variables.

## Common IMAP Ports

| Port | Usage | TLS |
|------|-------|-----|
| 143 | IMAP (standard) | Optional STARTTLS |
| 993 | IMAPS (implicit TLS) | Implicit TLS |

**Recommendations:**
- **Port 993 with `-imaps`**: Use for most modern IMAP servers
- **Port 143 with `-starttls`**: Use when explicit TLS upgrade is required

## IMAPS vs STARTTLS

| Method | Port | Flag | Description |
|--------|------|------|-------------|
| **IMAPS** | 993 | `-imaps` | Implicit TLS - encryption starts immediately |
| **STARTTLS** | 143 | `-starttls` | Explicit TLS - plain connection upgrades after STARTTLS |

```powershell
# IMAPS (implicit TLS on port 993)
.\imaptool.exe -action testconnect -host imap.gmail.com -imaps

# STARTTLS (explicit TLS on port 143)
.\imaptool.exe -action testconnect -host imap.example.com -port 143 -starttls
```

**Note:** Cannot use both `-imaps` and `-starttls` together.

## Common Provider Configurations

| Provider | Host | Port | Method |
|----------|------|------|--------|
| Gmail | imap.gmail.com | 993 | IMAPS |
| Microsoft 365 | outlook.office365.com | 993 | IMAPS |
| Yahoo | imap.mail.yahoo.com | 993 | IMAPS |
| iCloud | imap.mail.me.com | 993 | IMAPS |

```powershell
# Gmail
.\imaptool.exe -action testauth -host imap.gmail.com -imaps \
    -username user@gmail.com -password "app-password"

# Microsoft 365
.\imaptool.exe -action testauth -host outlook.office365.com -imaps \
    -username user@company.com -password "password"

# Gmail with OAuth2
.\imaptool.exe -action testauth -host imap.gmail.com -imaps \
    -username user@gmail.com -accesstoken "ya29.xxx..."
```

## CSV Logging

All operations are automatically logged to action-specific CSV files:

**Location:** `%TEMP%\_imaptool_{action}_{date}.csv`

**Examples:**
- `C:\Users\username\AppData\Local\Temp\_imaptool_testconnect_2026-01-31.csv`
- `C:\Users\username\AppData\Local\Temp\_imaptool_testauth_2026-01-31.csv`

## Troubleshooting

### Connection Issues

**"Connection refused"**
- Verify server hostname and port
- Check if IMAPS (993) or IMAP (143) is required
- Verify firewall rules allow outbound connections

**"Connection timeout"**
- Increase `-timeout` value
- Check network connectivity
- Verify DNS resolution

### TLS Issues

**"Cannot use both -imaps and -starttls"**
- Choose one TLS method:
  - `-imaps` for port 993 (implicit TLS)
  - `-starttls` for port 143 (explicit TLS)

**"TLS handshake failed"**
- Try `-skipverify` temporarily (insecure, for testing only)
- Check server certificate validity
- Try different TLS version: `-tlsversion 1.3`

### Authentication Issues

**"Authentication failed"**
- Verify username and password
- For Gmail: Use app-specific password (not regular password)
- Try different auth method: `-authmethod PLAIN` or `-authmethod LOGIN`

**"XOAUTH2 authentication requires -accesstoken"**
- Provide OAuth2 access token via `-accesstoken` flag
- Ensure token is valid and not expired

## Security Best Practices

1. **Credential Management**:
   - Use environment variables for passwords
   - Never commit credentials to version control
   - Use OAuth2 tokens when available

2. **TLS Configuration**:
   - Always use TLS (IMAPS or STARTTLS)
   - Never use `-skipverify` in production
   - Use TLS 1.2 or higher

3. **Logging**:
   - CSV logs may contain usernames
   - Secure log files appropriately
   - Passwords are NOT logged

## Related Documentation

- **Build Instructions**: [BUILD.md](BUILD.md)
- **SMTP Tool**: [SMTP_TOOL_README.md](SMTP_TOOL_README.md)
- **POP3 Tool**: [POP3TOOL_README.md](POP3TOOL_README.md)
- **Security Policy**: [SECURITY.md](SECURITY.md)

## Support

**Repository:** [https://github.com/ehlo-pl/gomailtesttool](https://github.com/ehlo-pl/gomailtesttool)

                          ..ooOO END OOoo..
