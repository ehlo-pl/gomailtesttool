# Integration Testing Guide

This guide explains how to run integration tests for the Microsoft Graph EXO Mails/Calendar Golang Testing Tool.

## Overview

The project provides two types of integration tests:

1. **Interactive Test Tool** (`integration_test_tool.go`) - Manual, interactive testing with user prompts
2. **Automated Integration Tests** (`msgraphtool_integration_test.go`) - Automated Go tests using `testing` package

Both types:
- ✅ Make real API calls to Microsoft Graph
- ✅ Use actual Azure AD authentication
- ✅ Send real emails and create real calendar events
- ✅ **Excluded from regular builds** (require `-tags=integration` flag)
- ✅ Use shared business logic from `shared.go` (no code duplication)

## Architecture

The codebase uses **build tags** to separate integration tests from the main application:

```
src/
├── shared.go                           # NO build tag - shared by all builds
├── msgraphtool.go         # //go:build !integration - main CLI app
├── integration_test_tool.go            # //go:build integration - interactive tests
├── msgraphtool_integration_test.go  # //go:build integration - automated tests
├── cert_windows.go                     # //go:build windows - Windows cert store
└── cert_stub.go                        # //go:build !windows - stub for other platforms
```

**Build Modes:**
- `go build ./src` → Builds **main CLI app only** (excludes integration tests)
- `go build -tags=integration ./src` → Would fail (multiple `main()` functions)
- `go run -tags=integration ./src/integration_test_tool.go` → Runs interactive test tool
- `go test -tags=integration ./src` → Runs automated integration tests

## Prerequisites

### 1. Entra ID Application Registration
Requires the following **Exchange Online RBAC permissions** (NOT Entra ID API permissions):
- **Application Mail.ReadWrite** - For sendmail, getinbox actions
- **Application Calendars.ReadWrite** - For getevents, sendinvite, getschedule actions

**Important Notes**:
- These are Exchange Online RBAC permissions assigned via PowerShell
- **Documentation**: [Exchange Online Application RBAC](https://learn.microsoft.com/en-us/exchange/permissions-exo/application-rbac)
- **Recommended Role**: Exchange Administrator (from PIM) to assign permissions
- While Global Administrator can assign permissions, Exchange Administrator is recommended following the Principle of Least Privilege

### 2. Authentication
- **Client Secret** (certificate authentication not used in integration tests for simplicity)

### 3. Test Mailbox
- A dedicated test mailbox you have access to
- **Do NOT use production mailboxes!**

## Setup

### Set Environment Variables

**PowerShell:**
```powershell
$env:MSGRAPHTENANTID = "12345678-1234-1234-1234-123456789012"
$env:MSGRAPHCLIENTID = "abcdefgh-5678-9012-abcd-ef1234567890"
$env:MSGRAPHSECRET = "your-client-secret-here"
$env:MSGRAPHMAILBOX = "test-user@example.com"
```

**Command Prompt:**
```cmd
set MSGRAPHTENANTID=12345678-1234-1234-1234-123456789012
set MSGRAPHCLIENTID=abcdefgh-5678-9012-abcd-ef1234567890
set MSGRAPHSECRET=your-client-secret-here
set MSGRAPHMAILBOX=test-user@example.com
```

**Linux/macOS:**
```bash
export MSGRAPHTENANTID="12345678-1234-1234-1234-123456789012"
export MSGRAPHCLIENTID="abcdefgh-5678-9012-abcd-ef1234567890"
export MSGRAPHSECRET="your-client-secret-here"
export MSGRAPHMAILBOX="test-user@example.com"
```

### Verify Environment Variables

```powershell
# PowerShell
if ($env:MSGRAPHTENANTID -and $env:MSGRAPHCLIENTID -and $env:MSGRAPHSECRET -and $env:MSGRAPHMAILBOX) {
    Write-Host "✅ All environment variables are set" -ForegroundColor Green
} else {
    Write-Host "❌ Missing environment variables" -ForegroundColor Red
}
```

## Option 1: Interactive Test Tool

The interactive test tool provides a guided testing experience with user prompts.

### Run Interactive Tests

```powershell
cd src
go run -tags=integration integration_test_tool.go
```

**Features:**
- ✅ Interactive prompts before write operations
- ✅ Real-time test results display
- ✅ Progress indicators
- ✅ Pass/fail summary

**What Gets Tested:**
1. **Get Events** (auto-runs) - Retrieves calendar events
2. **Send Mail** (prompts for confirmation) - Sends test email to self
3. **Send Calendar Invite** (prompts for confirmation) - Creates calendar event
4. **Get Inbox** (auto-runs) - Retrieves inbox messages

### Example Output

```
=================================================================
Microsoft Graph EXO Mails/Calendar Golang Testing Tool - Integration Test Suite
=================================================================

Configuration loaded:
  Tenant ID: 1234****-****-****-****9012
  Client ID: abcd****-****-****-****7890
  Secret:    your********cret
  Mailbox:   test-user@example.com

Proceed with integration tests? (y/n): y

Creating Microsoft Graph client...
✅ Graph client created successfully

─────────────────────────────────────────────────────────────────
Test 1: Get Events
─────────────────────────────────────────────────────────────────
Retrieving 3 upcoming calendar events from test-user@example.com...
Upcoming events for test-user@example.com:
- Team Meeting (ID: AAMkAD...)
- Project Review (ID: AAMkAE...)

Total events retrieved: 2
✅ PASSED: Successfully retrieved calendar events

─────────────────────────────────────────────────────────────────
Test 2: Send Mail
─────────────────────────────────────────────────────────────────
Send a test email to yourself? (y/n): y
Sending test email to test-user@example.com...
  Subject: Integration Test - 2026-01-05T14:30:00Z
Email sent successfully from test-user@example.com.
To: [test-user@example.com]
Cc: []
Bcc: []
Subject: Integration Test - 2026-01-05T14:30:00Z
Body Type: Text
✅ PASSED: Email sent successfully
  Check your inbox to verify delivery

=================================================================
Integration Test Results Summary
=================================================================
  ✅ Get Events:          PASSED
  ✅ Send Mail:           PASSED
  ✅ Send Invite:         PASSED
  ✅ Get Inbox:           PASSED
=================================================================

Pass Rate: 4/4 (100%)
✅ All integration tests passed!
```

## Option 2: Automated Integration Tests

Standard Go tests using the `testing` package.

### Run Automated Tests

**Read-only tests (safe):**
```powershell
cd src
go test -tags=integration -v
```

**All tests including write operations:**
```powershell
$env:MSGRAPH_INTEGRATION_WRITE = "true"
go test -tags=integration -v ./src
```

**Run specific test:**
```powershell
go test -tags=integration -v -run TestIntegration_ListEvents ./src
```

### Available Tests

| Test | Type | Description |
|------|------|-------------|
| `TestIntegration_Prerequisites` | Check | Verifies environment variables are set |
| `TestIntegration_GraphClientCreation` | Read | Tests Graph client creation |
| `TestIntegration_ListEvents` | Read | Retrieves calendar events |
| `TestIntegration_ListInbox` | Read | Retrieves inbox messages |
| `TestIntegration_SendEmail` | **Write** | Sends test email (requires `MSGRAPH_INTEGRATION_WRITE=true`) |
| `TestIntegration_CreateCalendarEvent` | **Write** | Creates calendar event (requires `MSGRAPH_INTEGRATION_WRITE=true`) |
| `TestIntegration_ValidateConfiguration` | Unit | Tests configuration validation logic |

**⚠️ Write Tests:**
- Require `MSGRAPH_INTEGRATION_WRITE=true` environment variable
- Send real emails and create real calendar events
- Automatically skipped if the environment variable is not set

### Example Output

```
=== RUN   TestIntegration_Prerequisites
--- PASS: TestIntegration_Prerequisites (0.00s)
=== RUN   TestIntegration_GraphClientCreation
    msgraphtool_integration_test.go:59: ✅ Graph client created successfully
--- PASS: TestIntegration_GraphClientCreation (1.23s)
=== RUN   TestIntegration_ListEvents
    msgraphtool_integration_test.go:72: Retrieving 3 upcoming calendar events from test-user@example.com
Upcoming events for test-user@example.com:
- Team Meeting (ID: AAMkAD...)

Total events retrieved: 1
    msgraphtool_integration_test.go:78: ✅ Successfully retrieved calendar events
--- PASS: TestIntegration_ListEvents (0.82s)
=== RUN   TestIntegration_ListInbox
    msgraphtool_integration_test.go:89: Retrieving 3 newest inbox messages from test-user@example.com
...
--- PASS: TestIntegration_ListInbox (0.95s)
=== RUN   TestIntegration_SendEmail
--- SKIP: TestIntegration_SendEmail (0.00s)
    msgraphtool_integration_test.go:104: Skipping write operation test - set MSGRAPH_INTEGRATION_WRITE=true to enable
...
PASS
ok      msgraphtool        3.456s
```

## Troubleshooting

### Authentication Errors

**Error:** "Failed to create Graph client: authentication setup failed"

**Solutions:**
- Verify all environment variables are set correctly
- Check that the client secret hasn't expired
- Ensure the App Registration has the correct permissions
- Verify admin consent has been granted

### Permission Errors

**Error:** "Insufficient privileges to complete the operation"

**Solutions:**
- Check Exchange Online RBAC permissions for Entra ID Application Registration:
  - **Application Mail.ReadWrite**
  - **Application Calendars.ReadWrite**
- Verify Exchange Administrator role is assigned (PIM) - recommended for least privilege
- Permissions must be assigned via PowerShell (see [Exchange Online Application RBAC](https://learn.microsoft.com/en-us/exchange/permissions-exo/application-rbac))
- Grant Admin Consent for these permissions
- Wait 5-10 minutes for permissions to propagate

### Mailbox Access Denied

**Error:** "Access is denied. Check credentials and try again."

**Solutions:**
- Verify the mailbox address is correct
- Ensure the mailbox exists and is licensed
- Check if Application Access Policies restrict this app
- Verify the service principal has access to the mailbox

### Network Errors

**Error:** "dial tcp: i/o timeout" or "connection timeout"

**Solutions:**
- Check network connectivity to graph.microsoft.com
- If behind a proxy, set `MSGRAPHPROXY` environment variable:
  ```powershell
  $env:MSGRAPHPROXY = "http://proxy.company.com:8080"
  ```

## Cleanup

After testing, you may want to:

1. **Delete test emails** from your inbox
2. **Delete test calendar events** from your calendar
3. **Clear environment variables** (for security):

```powershell
Remove-Item Env:\MSGRAPHSECRET
Remove-Item Env:\MSGRAPHTENANTID
Remove-Item Env:\MSGRAPHCLIENTID
Remove-Item Env:\MSGRAPHMAILBOX
Remove-Item Env:\MSGRAPH_INTEGRATION_WRITE
```

## Security Best Practices

⚠️ **Important Security Notes:**

1. **Never commit credentials** to source control
2. **Use environment variables** for all sensitive data
3. **Clear secrets after testing:**
   ```powershell
   Remove-Item Env:\MSGRAPHSECRET
   ```
4. **Use dedicated test mailbox** - don't use production mailboxes
5. **Rotate secrets regularly** - generate new client secrets every 90 days
6. **Monitor API usage** - check Azure AD sign-in logs for test activity

## CI/CD Integration

To run integration tests in CI/CD pipelines:

### GitHub Actions Example

```yaml
name: Integration Tests
on:
  workflow_dispatch:  # Manual trigger only (don't run on every commit)

jobs:
  integration-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Setup Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.25'

      - name: Run Integration Tests
        env:
          MSGRAPHTENANTID: ${{ secrets.MSGRAPH_TENANT_ID }}
          MSGRAPHCLIENTID: ${{ secrets.MSGRAPH_CLIENT_ID }}
          MSGRAPHSECRET: ${{ secrets.MSGRAPH_SECRET }}
          MSGRAPHMAILBOX: ${{ secrets.MSGRAPH_TEST_MAILBOX }}
          MSGRAPH_INTEGRATION_WRITE: "true"
        run: |
          cd src
          go test -tags=integration -v
```

**Important:**
- Use **workflow_dispatch** or manual triggers (not on every push)
- Store credentials as **GitHub Secrets**
- Use a dedicated test tenant
- Monitor API quota usage

## Comparison: Interactive vs Automated

| Feature | Interactive Tool | Automated Tests |
|---------|------------------|-----------------|
| **Run Command** | `go run -tags=integration integration_test_tool.go` | `go test -tags=integration -v` |
| **User Prompts** | ✅ Yes | ❌ No |
| **CI/CD Friendly** | ❌ No | ✅ Yes |
| **Write Protection** | ✅ Prompts for confirmation | ✅ Requires `MSGRAPH_INTEGRATION_WRITE=true` |
| **Output Format** | Pretty formatted | Standard Go test output |
| **Use Case** | Manual testing, demos | Automated regression testing, CI/CD |

## Limitations

- **Only tests client secret authentication** - certificate auth not included in integration tests
- **Requires manual verification** - check inbox/calendar to confirm operations
- **No automatic cleanup** - test emails and events must be deleted manually
- **Network dependent** - requires internet access to graph.microsoft.com
- **API quota consumption** - each test run consumes API calls against your quota

## Support

For issues or questions:
- See main [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for common errors
- Check [SECURITY_PRACTICES.md](SECURITY_PRACTICES.md) for security guidance
- Review [README.md](README.md) for general usage information
- Report issues at: https://github.com/ehlo-pl/gomailtesttool/issues

---

*Integration Testing Guide - Version 1.16.5 - 2026-01-05*

                          ..ooOO END OOoo..


