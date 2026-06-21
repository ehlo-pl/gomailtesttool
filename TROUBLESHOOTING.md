# Troubleshooting Guide

This guide helps diagnose and resolve common issues when using **gomailtest**.

> **Note on command style:** All examples below use the new `gomailtest msgraph <action> --flag` syntax. If you're using the legacy `msgraphtool` shim, replace `gomailtest msgraph <action>` with `msgraphtool -action <action>` and `--flag` with `-flag`.

---

## Authentication Errors

### "no valid authentication method provided"

**Cause:** None of `--secret`, `--pfx`, or `--thumbprint` were provided.

**Solution:**
```powershell
# Option 1: Client Secret
gomailtest msgraph getevents \
    --tenantid "..." --clientid "..." --secret "..." --mailbox "user@example.com"

# Option 2: PFX Certificate
gomailtest msgraph getevents \
    --tenantid "..." --clientid "..." \
    --pfx ".\cert.pfx" --pfxpass "password" \
    --mailbox "user@example.com"

# Option 3: Windows Certificate Store (Windows only)
gomailtest msgraph getevents \
    --tenantid "..." --clientid "..." \
    --thumbprint "ABC123..." --mailbox "user@example.com"
```

---

### "you must specify exactly one authentication method"

**Cause:** Multiple authentication methods provided simultaneously (e.g., both `--secret` and `--pfx`).

**Solution:** Use only ONE authentication method per execution:
```powershell
# WRONG - Multiple auth methods
gomailtest msgraph getevents --secret "..." --pfx "cert.pfx" --thumbprint "ABC123" ...

# CORRECT - Single auth method
gomailtest msgraph getevents --secret "..." --tenantid "..." --clientid "..." --mailbox "..."
```

---

### "failed to decode PFX" or "pkcs12: unknown digest algorithm"

**Cause:** PFX file is corrupted, password is incorrect, or unsupported encryption.

**Solution:**
1. Verify the password is correct:
   ```powershell
   gomailtest msgraph getevents --verbose --pfx "cert.pfx" --pfxpass "password" ...
   ```

2. Re-export the certificate with standard encryption:
   ```powershell
   $cert = Get-ChildItem Cert:\CurrentUser\My | Where-Object {$_.Thumbprint -eq "YOUR_THUMBPRINT"}
   $password = ConvertTo-SecureString -String "YourPassword" -Force -AsPlainText
   Export-PfxCertificate -Cert $cert -FilePath ".\cert.pfx" -Password $password
   ```

---

### "failed to export cert from store" (Windows Certificate Store)

**Cause:** Certificate not found, no private key, or expired.

**Solution:**
1. List all certificates and verify thumbprint:
   ```powershell
   Get-ChildItem Cert:\CurrentUser\My | Format-Table Thumbprint, Subject, NotAfter, HasPrivateKey
   ```

2. Ensure the certificate has a private key:
   ```powershell
   $cert = Get-ChildItem Cert:\CurrentUser\My | Where-Object {$_.Thumbprint -eq "YOUR_THUMBPRINT"}
   $cert.HasPrivateKey  # Should be True
   ```

---

### "ClientAuthenticationError: AADSTS700016: Application not found"

**Cause:** Invalid Client ID or application not registered in Entra ID.

**Solution:**
- Navigate to Entra ID → App Registrations → copy the "Application (client) ID"
- Verify both Tenant ID and Client ID are GUIDs (36 characters with dashes):
  ```powershell
  gomailtest msgraph getevents --verbose \
      --tenantid "..." --clientid "..." --secret "..." --mailbox "..."
  ```

---

### "ClientAuthenticationError: AADSTS7000215: Invalid client secret"

**Cause:** Client secret is incorrect or has expired.

**Solution:**
1. Generate a new client secret in Azure Portal → Entra ID → App Registrations → Your App → Certificates & secrets
2. Update your secret:
   ```powershell
   $env:MSGRAPHSECRET = "new-secret-value"
   gomailtest msgraph getevents --tenantid "..." --clientid "..." --mailbox "..."
   ```

---

## Permission Errors

### "Insufficient privileges to complete the operation" or "Access is denied"

**Cause:** App Registration missing required API permissions or admin consent not granted.

| Action | Required Permission |
|--------|---------------------|
| `getevents`, `sendinvite`, `getschedule` | Application Calendars.ReadWrite |
| `sendmail`, `getinbox`, `exportinbox`, `searchandexport` | Application Mail.ReadWrite |

This tool uses **Exchange Online RBAC permissions**, not Entra ID API permissions. Assign via `New-ServicePrincipal` / `New-ManagementRoleAssignment`. Wait 5-10 minutes for permissions to propagate.

---

### "ErrorAccessDenied: Access is denied. Check credentials and try again."

**Cause:** Authentication succeeded but mailbox access is denied.

**Solution:**
```powershell
# Verify mailbox address
gomailtest msgraph getevents --verbose \
    --tenantid "..." --clientid "..." --secret "..." --mailbox "user@example.com"
```

- Ensure the mailbox exists and is licensed in Microsoft 365 Admin Center

---

## Network and Proxy Errors

### "dial tcp: i/o timeout" or "connection timeout"

**Cause:** Firewall blocking outbound HTTPS to Microsoft Graph API.

**Solution:**
1. Test connectivity:
   ```powershell
   Test-NetConnection graph.microsoft.com -Port 443
   ```

2. If behind a corporate proxy:
   ```powershell
   # Via flag
   gomailtest msgraph getevents --proxy "http://proxy.company.com:8080" ...

   # Via environment variable
   $env:MSGRAPHPROXY = "http://proxy.company.com:8080"
   gomailtest msgraph getevents ...
   ```

---

### "proxyconnect tcp: dial tcp: lookup proxy.company.com: no such host"

**Cause:** Proxy URL is incorrect or unreachable.

**Solution:**
```powershell
# Correct format: http://hostname:port
$env:MSGRAPHPROXY = "http://proxy.company.com:8080"

# WRONG:
# proxy.company.com:8080  (missing http://)
```

---

## CSV Logging Issues

### "Could not create CSV log file: access is denied"

**Solution:**
```powershell
# Check temp directory
echo $env:TEMP  # Should show C:\Users\<User>\AppData\Local\Temp

# Verify write permissions
"test" | Out-File "$env:TEMP\_test.txt" && Remove-Item "$env:TEMP\_test.txt"
```

Also close any Excel instances that may have the CSV open.

---

## Input Validation Errors

### "Tenant ID should be a GUID (36 characters)"

**Solution:**
```powershell
# Correct format: 36 characters with dashes at positions 8, 13, 18, 23
--tenantid "12345678-1234-1234-1234-123456789012"
```

---

### "Start time is not in valid RFC3339 format"

**Solution:**
```powershell
# Correct RFC3339 format
--start "2026-01-15T14:00:00Z"
--end "2026-01-15T15:00:00Z"

# With timezone offset
--start "2026-01-15T14:00:00+01:00"
```

---

### "missing required parameter: tenantid"

**Solution:**
```powershell
# Option 1: Provide flags directly
gomailtest msgraph getevents \
    --tenantid "..." --clientid "..." --secret "..." --mailbox "..."

# Option 2: Use environment variables
$env:MSGRAPHTENANTID = "12345678-1234-1234-1234-123456789012"
$env:MSGRAPHCLIENTID = "abcdefgh-5678-9012-abcd-ef1234567890"
$env:MSGRAPHSECRET   = "your-secret"
$env:MSGRAPHMAILBOX  = "user@example.com"

gomailtest msgraph getevents
```

---

## Export and Search Issues

### "Message ID not found" (searchandexport)

**Cause:** Invalid Message ID format or message doesn't exist in mailbox.

**Solution:**
1. Message ID must be in angle brackets: `<message-id@example.com>`
2. Get valid IDs via:
   ```powershell
   gomailtest msgraph getinbox --count 10 --verbose
   ```
3. In Outlook: File → Properties → Internet headers → find "Message-ID:" field

**Example:**
```powershell
gomailtest msgraph searchandexport \
    --tenantid "..." --clientid "..." --secret "..." \
    --mailbox "user@example.com" \
    --messageid "<CABcD123XYZ@mail.gmail.com>"
```

---

### Email sent but not received

**Solution:**
1. Check CSV log for confirmation:
   ```powershell
   $csv = Get-ChildItem "$env:TEMP\_msgraphtool_sendmail_*.csv" |
       Sort-Object LastWriteTime -Descending | Select-Object -First 1
   Import-Csv $csv.FullName | Format-Table
   ```

2. Check sender's Sent Items folder in Outlook

3. Review Exchange message trace in Microsoft 365 Admin Center → Mail flow → Message trace

---

## Verbose Mode Debugging

Enable verbose mode to see detailed diagnostic information:

```powershell
gomailtest msgraph getevents --verbose \
    --tenantid "..." --clientid "..." --secret "..." --mailbox "..."
```

**Verbose output includes:**
- Environment variables (with secrets masked)
- Final configuration values
- Authentication method details
- JWT token information (expiration, truncated token)
- Graph API endpoints being called

---

## Getting Help

```powershell
# Show all protocols
gomailtest --help

# Show msgraph subcommands and flags
gomailtest msgraph --help

# Show help for a specific action
gomailtest msgraph sendmail --help

# Show version
gomailtest --version

# View recent logs
Get-ChildItem "$env:TEMP\_msgraphtool_*.csv" |
    Sort-Object LastWriteTime -Descending | Select-Object -First 5
```

**Report issues:** https://github.com/ehlo-pl/gomailtesttool/issues  
Include: version, full command used, error message, verbose output (with secrets redacted).

                          ..ooOO END OOoo..
