# Integration Testing Guide

## Overview

Integration tests make real network connections to email servers and APIs. They are separate from unit tests and must be run explicitly.

## Test Types

| Type | Description | Requires |
|------|-------------|---------|
| SMTP | Connect to real SMTP servers | Network, optional credentials |
| IMAP | Connect to real IMAP servers | Network, credentials |
| POP3 | Connect to real POP3 servers | Network, credentials |
| JMAP | Connect to real JMAP servers | Network, credentials / access token |
| EWS | Connect to on-premises Exchange EWS endpoints | Network, credentials / access token |
| Microsoft Graph | Exchange Online API operations | Azure AD app registration |

## Local SMTP Testing with Mailpit

For SMTP integration testing without a real mail server, use [Mailpit](https://github.com/axllent/mailpit):

```bash
# Start Mailpit
docker run -d --name mailpit -p 1025:1025 -p 8025:8025 axllent/mailpit
```

```powershell
# Test full SMTP pipeline against Mailpit
gomailtest smtp testconnect --host localhost --port 1025
gomailtest smtp testauth --host localhost --port 1025 --username test --password test
gomailtest smtp sendmail --host localhost --port 1025 \
  --from sender@test.local --to recipient@test.local \
  --subject "Integration Test" --body "Testing SMTP"
```

View captured emails at [http://localhost:8025](http://localhost:8025).

**Automated PowerShell test script:**

```powershell
# integration-smtp.ps1
$mailpitHost = "localhost"
$mailpitPort  = 1025

Write-Host "1. Testing connectivity..." -ForegroundColor Yellow
gomailtest smtp testconnect --host $mailpitHost --port $mailpitPort
if ($LASTEXITCODE -ne 0) { exit 1 }

Write-Host "2. Testing authentication..." -ForegroundColor Yellow
gomailtest smtp testauth --host $mailpitHost --port $mailpitPort `
    --username test --password test
if ($LASTEXITCODE -ne 0) { exit 1 }

Write-Host "3. Sending test email..." -ForegroundColor Yellow
gomailtest smtp sendmail --host $mailpitHost --port $mailpitPort `
    --from integration@local --to testrecipient@local `
    --subject "Integration Test $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')" `
    --body "Automated integration test"
if ($LASTEXITCODE -ne 0) { exit 1 }

Write-Host "`n✓ All SMTP integration tests passed!" -ForegroundColor Green
Write-Host "View emails at: http://localhost:8025" -ForegroundColor Cyan
```

## IMAP Integration Testing

```powershell
# Against a real IMAP server (Gmail example)
gomailtest imap testconnect --host imap.gmail.com --imaps
gomailtest imap testauth --host imap.gmail.com --imaps `
    --username user@gmail.com --password "app-password"
gomailtest imap listfolders --host imap.gmail.com --imaps `
    --username user@gmail.com --password "app-password"
```

## JMAP Integration Testing

```powershell
# Against Fastmail
gomailtest jmap testconnect --host jmap.fastmail.com
gomailtest jmap testauth --host jmap.fastmail.com `
    --username user@fastmail.com --accesstoken "fmu1-xxx..."
gomailtest jmap getmailboxes --host jmap.fastmail.com `
    --username user@fastmail.com --accesstoken "fmu1-xxx..."
```

## EWS Integration Testing

**Prerequisites:** Access to an on-premises Exchange Server with EWS enabled.

```powershell
# Test basic EWS connectivity (no credentials required)
gomailtest ews testconnect --host mail.example.com

# Test NTLM authentication
gomailtest ews testauth --host mail.example.com `
    --username "CORP\user" --password "secret"

# Test Basic authentication
gomailtest ews testauth --host mail.example.com `
    --username user@example.com --password "secret" --authmethod Basic

# Test Bearer (OAuth2) authentication
gomailtest ews testauth --host mail.example.com `
    --username user@example.com --accesstoken "eyJ0..."

# Get Inbox folder properties
gomailtest ews getfolder --host mail.example.com `
    --username "CORP\user" --password "secret"

# Run Autodiscover
gomailtest ews autodiscover --host mail.example.com `
    --username user@example.com
```

Environment variable shortcut:

```powershell
$env:EWSHOST     = "mail.example.com"
$env:EWSUSERNAME = "CORP\user"
$env:EWSPASSWORD = "secret"

gomailtest ews testconnect
gomailtest ews testauth
gomailtest ews getfolder
```

See [docs/protocols/ews.md](docs/protocols/ews.md) for full flag and environment variable reference.

## Microsoft Graph Integration Testing

**Prerequisites:** An Azure AD App Registration with appropriate Microsoft Graph permissions.

```powershell
# Set authentication
$env:MSGRAPHTENANTID = "your-tenant-id"
$env:MSGRAPHCLIENTID = "your-client-id"
$env:MSGRAPHSECRET   = "your-secret"
$env:MSGRAPHMAILBOX  = "user@example.com"

# Run operations
gomailtest msgraph getevents
gomailtest msgraph getinbox --count 5
gomailtest msgraph sendmail --to "test@example.com" --subject "Integration Test"
```

Required Azure AD permissions:
- `Mail.Send` — for sendmail
- `Calendars.ReadWrite` — for getevents, sendinvite, getschedule
- `Mail.Read` — for getinbox, exportinbox, searchandexport

See [docs/protocols/msgraph.md](docs/protocols/msgraph.md) for full permission details.

## Load Balancer / Address Override Testing

All protocols support `--address` to override the TCP connection address while keeping the original hostname for TLS SNI and certificate verification. Useful for testing specific backend nodes.

```powershell
# Connect to 192.168.1.10 but verify TLS cert for smtp.example.com
gomailtest smtp testconnect --host smtp.example.com --address 192.168.1.10 --port 25

# IMAP
gomailtest imap testconnect --host imap.example.com --address 192.168.1.20 --imaps

# JMAP
gomailtest jmap testconnect --host jmap.example.com --address 10.0.0.5
```

## Proxy Testing

```powershell
# Via flag
gomailtest smtp testconnect --host smtp.example.com --proxy "http://proxy.corp.com:8080"
gomailtest msgraph getevents --proxy "http://proxy.corp.com:8080"

# Via environment variable
$env:SMTPPROXY = "http://proxy.corp.com:8080"
gomailtest smtp testconnect --host smtp.example.com

# SOCKS5 proxy
gomailtest smtp testauth --host smtp.example.com `
    --proxy "socks5://user:pass@socks-proxy.corp.com:1080" `
    --username user@example.com --password secret
```

## Related Documentation

- [UNIT_TESTS.md](UNIT_TESTS.md) — Unit test documentation
- [docs/protocols/smtp.md](docs/protocols/smtp.md) — SMTP documentation (includes Mailpit guide)
- [docs/protocols/ews.md](docs/protocols/ews.md) — EWS documentation
- [BUILD.md](BUILD.md) — Build instructions

                          ..ooOO END OOoo..
