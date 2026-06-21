# Troubleshooting Guide

This guide helps diagnose and resolve common issues when using the Microsoft Graph EXO Mails/Calendar Golang Testing Tool.

---

## Authentication Errors

### "no valid authentication method provided"

**Cause:** None of `-secret`, `-pfx`, or `-thumbprint` were provided.

**Solution:**
```powershell
# Provide at least one authentication method:

# Option 1: Client Secret
.\msgraphtool.exe -tenantid "..." -clientid "..." -secret "..." -mailbox "user@example.com" -action getevents

# Option 2: PFX Certificate
.\msgraphtool.exe -tenantid "..." -clientid "..." -pfx ".\cert.pfx" -pfxpass "password" -mailbox "user@example.com" -action getevents

# Option 3: Windows Certificate Store (Windows only)
.\msgraphtool.exe -tenantid "..." -clientid "..." -thumbprint "ABC123..." -mailbox "user@example.com" -action getevents
```

---

### "you must specify exactly one authentication method"

**Cause:** Multiple authentication methods provided simultaneously (e.g., both `-secret` and `-pfx`).

**Solution:** Use only ONE authentication method per execution:
```powershell
# WRONG - Multiple auth methods
.\msgraphtool.exe -secret "..." -pfx "cert.pfx" -thumbprint "ABC123" ...

# CORRECT - Single auth method
.\msgraphtool.exe -secret "..." -tenantid "..." -clientid "..." -mailbox "..." -action getevents
```

---

### "failed to decode PFX" or "pkcs12: unknown digest algorithm"

**Cause:**
- PFX file is corrupted
- PFX password is incorrect
- PFX file uses unsupported encryption

**Solution:**
1. Verify the password is correct:
   ```powershell
   # Use verbose mode to see masked password
   .\msgraphtool.exe -verbose -pfx "cert.pfx" -pfxpass "password" ...
   ```

2. Re-export the certificate with standard encryption:
   ```powershell
   # Export from Windows Certificate Store with TripleDES-SHA1 encryption
   $cert = Get-ChildItem Cert:\CurrentUser\My | Where-Object {$_.Thumbprint -eq "YOUR_THUMBPRINT"}
   $password = ConvertTo-SecureString -String "YourPassword" -Force -AsPlainText
   Export-PfxCertificate -Cert $cert -FilePath ".\cert.pfx" -Password $password
   ```

3. Check file integrity:
   ```powershell
   Get-Item .\cert.pfx | Select-Object Name, Length, LastWriteTime
   ```

---

### "failed to export cert from store" (Windows Certificate Store)

**Cause:**
- Certificate not found in Windows Certificate Store
- Certificate doesn't have a private key
- Certificate has expired

**Solution:**
1. List all certificates and verify thumbprint:
   ```powershell
   Get-ChildItem Cert:\CurrentUser\My | Format-Table Thumbprint, Subject, NotAfter, HasPrivateKey
   ```

2. Ensure the certificate has a private key:
   ```powershell
   $cert = Get-ChildItem Cert:\CurrentUser\My | Where-Object {$_.Thumbprint -eq "YOUR_THUMBPRINT"}
   if ($cert.HasPrivateKey) {
       Write-Host "Certificate has private key"
   } else {
       Write-Host "ERROR: Certificate missing private key"
   }
   ```

3. Check certificate expiration:
   ```powershell
   $cert = Get-ChildItem Cert:\CurrentUser\My | Where-Object {$_.Thumbprint -eq "YOUR_THUMBPRINT"}
   Write-Host "Expires: $($cert.NotAfter)"
   if ($cert.NotAfter -lt (Get-Date)) {
       Write-Host "ERROR: Certificate has expired"
   }
   ```

4. If certificate is missing, re-import or generate new one:
   ```powershell
   # Generate new self-signed certificate for testing
   .\selfsignedcert.ps1
   ```

---

### "ClientAuthenticationError: AADSTS700016: Application not found"

**Cause:** Invalid Client ID or application not registered in Entra ID.

**Solution:**
1. Verify Client ID in Azure Portal:
   - Navigate to Entra ID → App Registrations
   - Find your application
   - Copy the "Application (client) ID"

2. Ensure you're using the correct Tenant ID:
   ```powershell
   # Verify both Tenant ID and Client ID are GUIDs (36 characters with dashes)
   .\msgraphtool.exe -verbose -tenantid "..." -clientid "..." -secret "..." -mailbox "..." -action getevents
   ```

---

### "ClientAuthenticationError: AADSTS7000215: Invalid client secret"

**Cause:** Client secret is incorrect or has expired.

**Solution:**
1. Generate a new client secret in Azure Portal:
   - Navigate to Entra ID → App Registrations → Your App
   - Go to "Certificates & secrets"
   - Click "New client secret"
   - Copy the secret value immediately (it won't be shown again)

2. Update your secret:
   ```powershell
   $env:MSGRAPHSECRET = "new-secret-value"
   .\msgraphtool.exe -tenantid "..." -clientid "..." -mailbox "..." -action getevents
   ```

---

## Permission Errors

### "Insufficient privileges to complete the operation" or "Access is denied"

**Cause:** App Registration missing required API permissions or admin consent not granted.

**Solution:**
1. Assign required Exchange Online RBAC permissions:

   **Important**: This tool uses Exchange Online RBAC permissions, NOT Entra ID API permissions.

   | Action | Required Permission |
   |--------|---------------------|
   | `getevents` | **Application Calendars.ReadWrite** |
   | `sendmail` | **Application Mail.ReadWrite** |
   | `sendinvite` | **Application Calendars.ReadWrite** |
   | `getinbox` | **Application Mail.ReadWrite** |
   | `getschedule` | **Application Calendars.ReadWrite** |
   | `exportinbox` | **Application Mail.ReadWrite** |
   | `searchandexport` | **Application Mail.ReadWrite** |

2. Assign permissions via PowerShell:
   - **Recommended Role**: Exchange Administrator (from PIM) - following the Principle of Least Privilege
   - **Documentation**: [Exchange Online Application RBAC](https://learn.microsoft.com/en-us/exchange/permissions-exo/application-rbac)
   - Use `New-ServicePrincipal` or `New-ManagementRoleAssignment` cmdlets
   - Wait 5-10 minutes for permissions to propagate

   **Note**: While Global Administrator can assign these permissions, Exchange Administrator is recommended for least privilege access.

3. Verify permissions are granted:
   - Check that "Status" column shows green checkmark "Granted for [Your Organization]"

---

### "ErrorAccessDenied: Access is denied. Check credentials and try again."

**Cause:** Authentication succeeded but mailbox access is denied.

**Solution:**
1. Verify the mailbox address is correct:
   ```powershell
   # Check exact email address
   .\msgraphtool.exe -verbose -tenantid "..." -clientid "..." -secret "..." -mailbox "user@example.com" -action getevents
   ```

2. Ensure the mailbox exists and is licensed:
   - Verify in Microsoft 365 Admin Center → Users → Active users
   - Check that the user has an Exchange Online license

3. Check if mailbox requires specific permissions:
   - Some organizations restrict application access to specific mailboxes
   - Contact your Exchange administrator

---

## Network and Proxy Errors

### "dial tcp: i/o timeout" or "connection timeout"

**Cause:** Network connectivity issues or firewall blocking outbound HTTPS to Microsoft Graph API.

**Solution:**
1. Test connectivity to Microsoft Graph:
   ```powershell
   Test-NetConnection graph.microsoft.com -Port 443
   ```

2. Check firewall rules:
   - Ensure outbound HTTPS (port 443) is allowed
   - Whitelist `*.graph.microsoft.com` and `login.microsoftonline.com`

3. If behind a corporate proxy, configure proxy:
   ```powershell
   # Option 1: Command-line flag
   .\msgraphtool.exe -proxy "http://proxy.company.com:8080" ...

   # Option 2: Environment variable
   $env:MSGRAPHPROXY = "http://proxy.company.com:8080"
   .\msgraphtool.exe -tenantid "..." -clientid "..." -secret "..." -mailbox "..." -action getevents
   ```

---

### "proxyconnect tcp: dial tcp: lookup proxy.company.com: no such host"

**Cause:** Proxy URL is incorrect or proxy server is unreachable.

**Solution:**
1. Verify proxy URL format:
   ```powershell
   # Correct format: http://hostname:port or http://ip:port
   $env:MSGRAPHPROXY = "http://proxy.company.com:8080"

   # WRONG formats:
   # proxy.company.com:8080  (missing http://)
   # https://proxy.company.com:8080  (HTTPS not supported for proxy URL)
   ```

2. Test proxy connectivity:
   ```powershell
   Test-NetConnection proxy.company.com -Port 8080
   ```

3. Check proxy authentication requirements:
   - If proxy requires authentication, configure system proxy settings
   - The tool uses Go's standard proxy environment variables

---

## CSV Logging Issues

### "Could not create CSV log file: access is denied"

**Cause:** Permissions issue in temp directory or file is locked by another process.

**Solution:**
1. Check temp directory:
   ```powershell
   echo $env:TEMP
   # Should show: C:\Users\<Username>\AppData\Local\Temp
   ```

2. Verify write permissions:
   ```powershell
   $testFile = "$env:TEMP\_test.txt"
   "test" | Out-File $testFile
   Remove-Item $testFile
   ```

3. Check if CSV file is open in Excel or another program:
   ```powershell
   # Close all Excel instances and try again
   ```

4. Check disk space:
   ```powershell
   Get-PSDrive C | Select-Object Used, Free
   ```

---

### CSV file is empty or missing rows

**Cause:** CSV file was opened in Excel while the tool was writing, or tool terminated abnormally.

**Solution:**
1. Close Excel before running the tool
2. Let the tool complete execution (it flushes on exit)
3. Check for error messages in console output
4. Run with verbose mode to see detailed logging:
   ```powershell
   .\msgraphtool.exe -verbose -tenantid "..." -clientid "..." -secret "..." -mailbox "..." -action getevents
   ```

---

## Input Validation Errors

### "Tenant ID should be a GUID (36 characters)"

**Cause:** Tenant ID is not in proper GUID format.

**Solution:**
1. Get your Tenant ID from Azure Portal:
   - Navigate to Entra ID → Overview
   - Copy "Tenant ID" (format: `12345678-1234-1234-1234-123456789012`)

2. Verify format:
   ```powershell
   # Correct: 36 characters with dashes at positions 8, 13, 18, 23
   -tenantid "12345678-1234-1234-1234-123456789012"

   # WRONG:
   -tenantid "tenant-123"
   -tenantid "12345678"
   ```

---

### "invalid email format: missing @"

**Cause:** Mailbox or recipient email address is missing @ symbol.

**Solution:**
```powershell
# Correct email format
-mailbox "user@example.com"

# WRONG:
-mailbox "user"
-mailbox "user.example.com"
```

---

### "Start time is not in valid RFC3339 format"

**Cause:** Calendar invite start/end times are not in RFC3339 format.

**Solution:**
```powershell
# Correct RFC3339 format (ISO 8601 with timezone)
-start "2026-01-15T14:00:00Z"
-end "2026-01-15T15:00:00Z"

# With timezone offset
-start "2026-01-15T14:00:00+01:00"

# WRONG formats:
-start "2026-01-15 14:00:00"  # Missing 'T' separator
-start "2026-01-15T14:00:00"  # Missing timezone
-start "01/15/2026 2:00 PM"   # Wrong format entirely
```

---

## Common Usage Errors

### Calendar event not created but no error shown

**Cause:** Event was created but not visible due to time zone or calendar view issues.

**Solution:**
1. Use verbose mode to see event ID:
   ```powershell
   .\msgraphtool.exe -verbose -tenantid "..." -clientid "..." -secret "..." -mailbox "..." -action sendinvite
   ```

2. Verify event creation with getevents:
   ```powershell
   .\msgraphtool.exe -tenantid "..." -clientid "..." -secret "..." -mailbox "..." -action getevents -count 10
   ```

3. Check calendar permissions in Outlook

---

### Email sent but not received

**Cause:**
- Email caught by spam filter
- Recipient address incorrect
- Exchange transport rules blocking delivery

**Solution:**
1. Check CSV log for confirmation:
   ```powershell
   # Open CSV file
   $csvFile = Get-ChildItem "$env:TEMP\_msgraphtool_sendmail_*.csv" | Sort-Object LastWriteTime -Descending | Select-Object -First 1
   Import-Csv $csvFile.FullName | Format-Table
   ```

2. Check sender's Sent Items folder in Outlook

3. Verify recipient email address:
   ```powershell
   # Use verbose mode to see final configuration
   .\msgraphtool.exe -verbose -to "recipient@example.com" ...
   ```

4. Review Exchange message trace in Microsoft 365 Admin Center:
   - Go to Exchange admin center → Mail flow → Message trace

---

### "missing required parameter: tenantid"

**Cause:** Required flag not provided and not set via environment variable.

**Solution:**
```powershell
# Option 1: Provide all required flags
.\msgraphtool.exe -tenantid "..." -clientid "..." -secret "..." -mailbox "..." -action getevents

# Option 2: Use environment variables
$env:MSGRAPHTENANTID = "12345678-1234-1234-1234-123456789012"
$env:MSGRAPHCLIENTID = "abcdefgh-5678-9012-abcd-ef1234567890"
$env:MSGRAPHSECRET = "your-secret"
$env:MSGRAPHMAILBOX = "user@example.com"

# Then run with minimal flags
.\msgraphtool.exe -action getevents
```

---

## Export Directory and File Issues

### "Could not create export directory: access is denied"

**Cause:** Permissions issue in temp directory or disk full.

**Solution:**
1. Check temp directory permissions:
   ```powershell
   echo $env:TEMP
   # Should show: C:\Users\<Username>\AppData\Local\Temp
   ```

2. Verify write permissions:
   ```powershell
   $exportDir = "$env:TEMP\export"
   New-Item -Path $exportDir -ItemType Directory -Force
   "test" | Out-File "$exportDir\test.txt"
   Remove-Item "$exportDir\test.txt"
   Remove-Item $exportDir
   ```

3. Check disk space:
   ```powershell
   Get-PSDrive C | Select-Object Used, Free
   ```

4. Run as administrator if necessary (though shouldn't be required for %TEMP%)

---

### "Export file already exists" or duplicate JSON files

**Cause:** File naming conflict or rapid successive exports.

**Solution:**

The tool creates date-stamped directories and timestamped filenames:
```
%TEMP%\export\2026-01-07\
├── message_1_2026-01-07T10-30-45.json
├── message_2_2026-01-07T10-30-46.json
```

If you see conflicts:
1. Wait 1 second between export operations
2. Check if previous export is still running
3. Manually delete old export directories if needed:
   ```powershell
   Remove-Item "$env:TEMP\export" -Recurse -Force
   ```

---

### "Message ID not found" (searchandexport action)

**Cause:** Invalid Message ID format or message doesn't exist in mailbox.

**Solution:**

1. **Verify Message ID format:**
   - Must be enclosed in angle brackets: `<message-id@example.com>`
   - Obtained from email headers (Internet Message ID)

2. **Get valid Message ID from getinbox:**
   ```powershell
   # List recent messages with verbose mode
   .\msgraphtool.exe -action getinbox -count 10 -verbose
   ```

3. **Get Message ID from Outlook:**
   - Open the email
   - File → Properties
   - Look for "Internet headers" section
   - Find the "Message-ID:" field (format: `<ABC123@server.com>`)

4. **Example valid search:**
   ```powershell
   .\msgraphtool.exe -action searchandexport \
       -tenantid "..." -clientid "..." -secret "..." \
       -mailbox "user@example.com" \
       -messageid "<CABcD123XYZ@mail.gmail.com>"
   ```

5. **Verify mailbox access:**
   - Ensure you're searching the correct mailbox
   - Message might have been deleted or moved to another folder

---

### JSON export files are too large or taking too long

**Cause:** Exporting too many messages at once.

**Solution:**

1. **Reduce count:**
   ```powershell
   # Export in smaller batches
   .\msgraphtool.exe -action exportinbox -count 10
   ```

2. **Monitor export progress:**
   ```powershell
   # Use verbose mode to see progress
   .\msgraphtool.exe -action exportinbox -count 50 -verbose
   ```

3. **Check exported file sizes:**
   ```powershell
   $exportDir = Get-ChildItem "$env:TEMP\export" -Recurse -File | Sort-Object LastWriteTime -Descending
   $exportDir | Select-Object Name, Length, LastWriteTime
   ```

---

## Verbose Mode Debugging

Enable verbose mode to see detailed diagnostic information:

```powershell
.\msgraphtool.exe -verbose -tenantid "..." -clientid "..." -secret "..." -mailbox "..." -action getevents
```

**Verbose output includes:**
- Environment variables (with secrets masked)
- Final configuration values
- Authentication method details
- JWT token information (expiration, truncated token)
- Graph API endpoints being called
- Request parameters and responses

**Example verbose output:**
```
==================================================
Environment Variables Set:
==================================================
MSGRAPHCLIENTID = abcd****890
MSGRAPHSECRET = secr********cret
MSGRAPHTENANTID = 1234****9012

==================================================
Final Configuration:
==================================================
Tenant ID: 1234****9012
Client ID: abcd****890
Mailbox: user@example.com
Action: getevents
Authentication: Client Secret (secr********cret)
```

---

## Getting Help

If you continue to experience issues:

1. **Check documentation:**
   - README.md - Usage examples and features
   - AGENTS.md - Architecture and technical details
   - SECURITY.md - Security best practices

2. **Run with verbose mode:**
   ```powershell
   .\msgraphtool.exe -verbose -version
   ```

3. **Check CSV logs:**
   ```powershell
   # View recent logs
   Get-ChildItem "$env:TEMP\_msgraphtool_*.csv" | Sort-Object LastWriteTime -Descending | Select-Object -First 5
   ```

4. **Report issues:**
   - GitHub: https://github.com/ehlo-pl/gomailtesttool/issues
   - Include: Version, command used, error message, verbose output (with secrets redacted)

---

*Last Updated: 2026-01-07 - Version 1.21.0*

                          ..ooOO END OOoo..


