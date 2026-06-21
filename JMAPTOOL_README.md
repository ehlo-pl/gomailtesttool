# JMAP Testing Tool (jmaptool)

A command-line tool for testing JMAP (JSON Meta Application Protocol) server connectivity and operations. Part of the **gomailtesttool** suite.

## Overview

The **jmaptool** provides JMAP protocol testing and diagnostics:

- **Connection Testing**: JMAP server connectivity and session discovery
- **Authentication Testing**: Basic and Bearer token authentication
- **Mailbox Operations**: List mailboxes via JMAP API
- **Automatic Logging**: CSV/JSON logging for audit and troubleshooting

**Target Use Cases:**
- JMAP server connectivity validation
- JMAP authentication testing
- Mailbox discovery
- JMAP infrastructure diagnostics
- Modern email protocol testing

## What is JMAP?

JMAP (JSON Meta Application Protocol) is a modern, open standard for email access that aims to replace IMAP. Key features:

- **JSON-based**: Uses JSON over HTTP instead of complex text protocols
- **Efficient**: Reduces round-trips compared to IMAP
- **Modern**: Designed for mobile and web applications
- **Standardized**: RFC 8620 (JMAP core) and RFC 8621 (JMAP for Mail)

**JMAP Providers:**
- Fastmail
- Cyrus IMAP
- Apache James
- Others implementing RFC 8620/8621

## Features

- **3 Comprehensive Actions**:
  - `testconnect` - Test JMAP server connectivity and discover session
  - `testauth` - Test JMAP authentication
  - `getmailboxes` - Get list of mailboxes

- **No External Dependencies**: Pure Go implementation
- **Cross-Platform**: Windows, Linux, macOS
- **CSV/JSON Logging**: Automatic logging to `%TEMP%\_jmaptool_{action}_{date}.csv`
- **Multiple Auth Methods**: Basic authentication and Bearer tokens

## Installation

### Build from Source

```powershell
# Build jmaptool only
go build -C cmd/jmaptool -ldflags="-s -w" -o jmaptool.exe

# Or build all tools
.\build-all.ps1
```

See [BUILD.md](BUILD.md) for detailed build instructions.

## Quick Start

```powershell
# Test JMAP server connectivity
.\jmaptool.exe -action testconnect -host jmap.fastmail.com

# Test authentication with access token
.\jmaptool.exe -action testauth -host jmap.fastmail.com \
    -username user@example.com -accesstoken "your-api-token"

# List mailboxes
.\jmaptool.exe -action getmailboxes -host jmap.fastmail.com \
    -username user@example.com -accesstoken "your-api-token"
```

## Actions

### 1. testconnect - Server Connectivity

Tests JMAP server connectivity and discovers the JMAP session.

**What it does:**
- Connects to JMAP server via HTTPS
- Performs JMAP session discovery (/.well-known/jmap)
- Retrieves server capabilities
- Logs results to CSV

```powershell
# Test JMAP connectivity
.\jmaptool.exe -action testconnect -host jmap.fastmail.com

# With verbose output
.\jmaptool.exe -action testconnect -host jmap.fastmail.com -verbose
```

**Example Output:**
```
Testing JMAP connectivity to jmap.fastmail.com:443...

Discovering JMAP session...
Session URL: https://jmap.fastmail.com/.well-known/jmap

Server Capabilities:
  urn:ietf:params:jmap:core
  urn:ietf:params:jmap:mail
  urn:ietf:params:jmap:submission
  urn:ietf:params:jmap:vacationresponse

API URL: https://api.fastmail.com/jmap/api/
Download URL: https://api.fastmail.com/jmap/download/{accountId}/{blobId}/{name}
Upload URL: https://api.fastmail.com/jmap/upload/{accountId}/

Connectivity test completed successfully
```

### 2. testauth - Authentication Testing

Tests JMAP authentication.

**What it does:**
- Connects to JMAP server
- Attempts authentication with provided credentials
- Verifies session access
- Logs authentication result

```powershell
# Test with Bearer token
.\jmaptool.exe -action testauth -host jmap.fastmail.com \
    -username user@example.com -accesstoken "your-api-token"

# Test with Basic authentication
.\jmaptool.exe -action testauth -host jmap.example.com \
    -username user@example.com -password "yourpassword" \
    -authmethod basic

# Auto-detect authentication method
.\jmaptool.exe -action testauth -host jmap.fastmail.com \
    -username user@example.com -accesstoken "token" \
    -authmethod auto
```

**Example Output:**
```
Testing JMAP authentication on jmap.fastmail.com:443...

Authentication method: Bearer token
Authenticating as: user@example.com

Authentication successful
Account ID: u1234567

Primary accounts:
  Mail: u1234567

Authentication test completed successfully
```

### 3. getmailboxes - List Mailboxes

Lists all mailboxes in the authenticated account.

**What it does:**
- Connects and authenticates to JMAP server
- Sends Mailbox/get request
- Displays mailbox hierarchy
- Logs mailbox information

```powershell
# List mailboxes
.\jmaptool.exe -action getmailboxes -host jmap.fastmail.com \
    -username user@example.com -accesstoken "your-api-token"
```

**Example Output:**
```
Listing mailboxes for user@example.com on jmap.fastmail.com:443...

Authenticated successfully

Mailboxes:
  Inbox (role: inbox)
    - Total: 42 messages
    - Unread: 5 messages
  Sent (role: sent)
    - Total: 156 messages
    - Unread: 0 messages
  Drafts (role: drafts)
    - Total: 3 messages
    - Unread: 0 messages
  Trash (role: trash)
    - Total: 12 messages
    - Unread: 0 messages
  Archive
    - Total: 1,234 messages
    - Unread: 0 messages

Total mailboxes: 5

Mailbox listing completed successfully
```

## Command-Line Flags

### Core Flags

| Flag | Description | Environment Variable | Default |
|------|-------------|---------------------|---------|
| `-action` | Action to perform (required) | `JMAPACTION` | - |
| `-host` | JMAP server hostname (required) | `JMAPHOST` | - |
| `-port` | JMAP server port | `JMAPPORT` | 443 |

### Authentication Flags

| Flag | Description | Environment Variable |
|------|-------------|---------------------|
| `-username` | Username for authentication | `JMAPUSERNAME` |
| `-password` | Password for Basic authentication | `JMAPPASSWORD` |
| `-accesstoken` | Access token for Bearer authentication | `JMAPACCESSTOKEN` |
| `-authmethod` | Auth method: auto, basic, bearer | `JMAPAUTHMETHOD` |

### TLS Flags

| Flag | Description | Environment Variable | Default |
|------|-------------|---------------------|---------|
| `-skipverify` | Skip TLS certificate verification | `JMAPSKIPVERIFY` | false |

### Runtime Flags

| Flag | Description | Environment Variable | Default |
|------|-------------|---------------------|---------|
| `-verbose` | Enable verbose output | `JMAPVERBOSE` | false |
| `-loglevel` | Log level: debug, info, warn, error | `JMAPLOGLEVEL` | info |
| `-logformat` | Log format: csv, json | `JMAPLOGFORMAT` | csv |
| `-version` | Show version information | - | - |

## Environment Variables

All flags can be set via environment variables with the `JMAP` prefix:

```powershell
# Windows PowerShell
$env:JMAPHOST = "jmap.fastmail.com"
$env:JMAPUSERNAME = "user@example.com"
$env:JMAPACCESSTOKEN = "your-api-token"

.\jmaptool.exe -action getmailboxes

# Linux/macOS Bash
export JMAPHOST="jmap.fastmail.com"
export JMAPUSERNAME="user@example.com"
export JMAPACCESSTOKEN="your-api-token"

./jmaptool -action getmailboxes
```

**Note:** Command-line flags take precedence over environment variables.

## Authentication Methods

### Bearer Token (Recommended)

Most JMAP providers (like Fastmail) use API tokens for authentication:

```powershell
.\jmaptool.exe -action testauth -host jmap.fastmail.com \
    -username user@example.com \
    -accesstoken "fmu1-xxxxxxxx-xxxxxxxxxxxx"
```

**Getting a Fastmail API Token:**
1. Log in to Fastmail web interface
2. Go to Settings -> Password & Security -> API Tokens
3. Create a new token with appropriate permissions
4. Use the token with `-accesstoken` flag

### Basic Authentication

Some JMAP servers support HTTP Basic authentication:

```powershell
.\jmaptool.exe -action testauth -host jmap.example.com \
    -username user@example.com \
    -password "yourpassword" \
    -authmethod basic
```

### Auto Detection

Let the tool choose the best authentication method:

```powershell
# If accesstoken is provided, uses Bearer; otherwise uses Basic
.\jmaptool.exe -action testauth -host jmap.example.com \
    -username user@example.com \
    -accesstoken "token" \
    -authmethod auto
```

## CSV Logging

All operations are automatically logged to action-specific CSV files:

**Location:** `%TEMP%\_jmaptool_{action}_{date}.csv`

**Examples:**
- `C:\Users\username\AppData\Local\Temp\_jmaptool_testconnect_2026-01-31.csv`
- `C:\Users\username\AppData\Local\Temp\_jmaptool_getmailboxes_2026-01-31.csv`

## JMAP Providers

### Fastmail

```powershell
# Test connectivity
.\jmaptool.exe -action testconnect -host jmap.fastmail.com

# Authenticate and list mailboxes
.\jmaptool.exe -action getmailboxes -host jmap.fastmail.com \
    -username user@fastmail.com \
    -accesstoken "fmu1-xxxxxxxx-xxxxxxxxxxxx"
```

### Self-Hosted (Cyrus IMAP, Apache James)

```powershell
# Test connectivity to self-hosted server
.\jmaptool.exe -action testconnect -host jmap.yourdomain.com

# With self-signed certificate (testing only)
.\jmaptool.exe -action testauth -host jmap.yourdomain.com \
    -username user@yourdomain.com \
    -password "password" \
    -authmethod basic \
    -skipverify
```

## Troubleshooting

### Connection Issues

**"Connection refused"**
- Verify server hostname
- JMAP typically uses port 443 (HTTPS)
- Verify firewall rules allow outbound HTTPS

**"TLS handshake failed"**
- Try `-skipverify` temporarily (insecure, for testing only)
- Check server certificate validity

### Authentication Issues

**"Authentication failed"**
- Verify username (usually full email address)
- Check API token validity and permissions
- Ensure token hasn't expired

**"Either password or accesstoken is required"**
- Provide either `-password` or `-accesstoken`
- For Fastmail: use API token with `-accesstoken`

### Discovery Issues

**"JMAP session discovery failed"**
- Server may not support JMAP
- Check if `/.well-known/jmap` endpoint is accessible
- Try accessing `https://host/.well-known/jmap` in browser

## Security Best Practices

1. **Credential Management**:
   - Use environment variables for tokens
   - Never commit credentials to version control
   - Use API tokens with minimal required permissions

2. **TLS Configuration**:
   - Never use `-skipverify` in production
   - JMAP always uses HTTPS

3. **Token Security**:
   - Rotate API tokens regularly
   - Revoke unused tokens
   - Use separate tokens for different applications

## Related Documentation

- **Build Instructions**: [BUILD.md](BUILD.md)
- **IMAP Tool**: [IMAPTOOL_README.md](IMAPTOOL_README.md)
- **SMTP Tool**: [SMTP_TOOL_README.md](SMTP_TOOL_README.md)
- **Security Policy**: [SECURITY.md](SECURITY.md)

## JMAP Resources

- **RFC 8620**: JMAP Core - [https://tools.ietf.org/html/rfc8620](https://tools.ietf.org/html/rfc8620)
- **RFC 8621**: JMAP for Mail - [https://tools.ietf.org/html/rfc8621](https://tools.ietf.org/html/rfc8621)
- **JMAP Website**: [https://jmap.io](https://jmap.io)

## Support

**Repository:** [https://github.com/ehlo-pl/gomailtesttool](https://github.com/ehlo-pl/gomailtesttool)

                          ..ooOO END OOoo..
