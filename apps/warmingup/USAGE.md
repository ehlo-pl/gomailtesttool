# warmingup: how to use it

A walkthrough from empty mailboxes to a running warm-up. [README.md](README.md) is the
reference for every setting.

The example warms up `example.com` and sends through your own server `mail.example.com`.
Replace all `example.*` names and `you.seed@...` addresses with yours.

## 1. Before you start

* A mail server for the domain that accepts authenticated submission and **DKIM signs**.
* SPF, DKIM and DMARC records for the domain, and a forward-confirmed PTR record for the
  server's outgoing IP address.
* A bounce mailbox on the same domain (for example `bounces@example.com`).
* Mailboxes you own at the providers below. Start with the ones your recipients use.

## 2. Mailboxes to check

| Provider | Endpoint | Server | Port | Access | Sign-in secret | Prepare in the mailbox |
|---|---|---|---|---|---|---|
| Gmail | `gmail1` | `imap.gmail.com` | 993 | IMAP | app password | 2-Step Verification; Settings → Labels: Spam "Show in IMAP" |
| Outlook.com / Hotmail | `outlook1` | `outlook.office365.com` | 993 | IMAP + OAuth | `warmingup auth` | enable IMAP; Entra app registration (section 4) |
| Yahoo | `yahoo1` | `imap.mail.yahoo.com` | 993 | IMAP | app password | create the app password in account security |
| AOL | `aol1` | `imap.aol.com` | 993 | IMAP | app password | create the app password in account security |
| iCloud | `icloud1` | `imap.mail.me.com` | 993 | IMAP | app-specific password | two-factor authentication on |
| WP | `wp1` | `imap.wp.pl` | 993 | IMAP | password | enable IMAP access |
| Onet | `onet1` | `imap.poczta.onet.pl` | 993 | IMAP | password | enable IMAP access |
| Interia | `interia1` | `poczta.interia.pl` | 993 | IMAP | password | allow mail programs ("korzystam z programu pocztowego") |
| GMX | `gmx1` | `imap.gmx.com` (`gmx.net` addresses: `imap.gmx.net`) | 993 | IMAP | password, app password with 2FA | enable IMAP (off by default) |
| Fastmail | `fastmail1` | `api.fastmail.com` | 443 | JMAP | API token | token with read and write access to mail (not on the Basic plan) |

Also needed, but not a warm-up target:

| Mailbox | Endpoint | Access | Purpose |
|---|---|---|---|
| Sending account on your server | `site-mta` | SMTP (587, STARTTLS) | sends the messages |
| `bounces@example.com` | `bounce-example` | IMAP or POP3 | rejections and deferrals from providers |

**None of these providers has been tested with this tool yet.** Provider settings and plan
limits change; check the provider's help pages when a sign-in fails. `validate --connect`
(section 5) tells you which mailboxes work.

### Your checklist

Fill one row per mailbox you use.

| Endpoint | Address | Account ready | Secret in envfile / signed in | `validate --connect` ok | `once`: inbox or junk | SPF / DKIM / DMARC pass |
|---|---|---|---|---|---|---|
| `gmail1` | | ☐ | ☐ | ☐ | | ☐ |
| `outlook1` | | ☐ | ☐ | ☐ | | ☐ |
| `yahoo1` | | ☐ | ☐ | ☐ | | ☐ |
| `aol1` | | ☐ | ☐ | ☐ | | ☐ |
| `icloud1` | | ☐ | ☐ | ☐ | | ☐ |
| `wp1` | | ☐ | ☐ | ☐ | | ☐ |
| `onet1` | | ☐ | ☐ | ☐ | | ☐ |
| `interia1` | | ☐ | ☐ | ☐ | | ☐ |
| `gmx1` | | ☐ | ☐ | ☐ | | ☐ |
| `fastmail1` | | ☐ | ☐ | ☐ | | ☐ |
| `bounce-example` | | ☐ | ☐ | ☐ | – | – |

## 3. Write the configuration

Three kinds of file, in `/etc/warmingup/` (Linux) or `C:\ProgramData\warmingup\` (Windows):

```
warmingup.yaml            mailbox connections
warmingup.env             passwords and tokens
domains.d/example.com.yaml   one file per warmed domain
```

Copy the samples from [deploy/](deploy/) as described under
[Linux service](README.md#linux-service) or [Windows service](README.md#windows-service).
On Windows remove the `#` before `envfile:` in `warmingup.yaml`.

`warmingup.env`, one `NAME=value` per line:

```ini
SITE_MTA_PASSWORD=...
BOUNCE_PASSWORD=...
GMAIL1_APP_PASSWORD=...
YAHOO1_APP_PASSWORD=...
WP1_PASSWORD=...
FASTMAIL_API_TOKEN=...
```

The sample `warmingup.yaml` has `gmail1`, `yahoo1`, `outlook1`, `wp1` and `fastmail1`.
Add the others under `endpoints:` as needed:

```yaml
  aol1:
    protocol: imap
    host: imap.aol.com
    imaps: true
    username: you.seed@aol.com
    password: ${AOL1_APP_PASSWORD}

  icloud1:
    protocol: imap
    host: imap.mail.me.com
    imaps: true
    username: you.seed@icloud.com   # if rejected: the part before @, plus "address:"
    password: ${ICLOUD1_APP_PASSWORD}

  onet1:
    protocol: imap
    host: imap.poczta.onet.pl
    imaps: true
    username: you.seed@onet.pl
    password: ${ONET1_PASSWORD}

  interia1:
    protocol: imap
    host: poczta.interia.pl
    imaps: true
    username: you.seed@interia.pl
    password: ${INTERIA1_PASSWORD}

  gmx1:
    protocol: imap
    host: imap.gmx.com
    imaps: true
    username: you.seed@gmx.com
    password: ${GMX1_PASSWORD}
```

The domain file, `domains.d/example.com.yaml`. List only mailboxes that are ready:

```yaml
domain: example.com
sender:
  endpoint: site-mta
  from: "Example News <news@example.com>"
  ehloname: mail.example.com
bounce:
  endpoint: bounce-example
  address: bounces@example.com
targets: [gmail1, yahoo1, outlook1, wp1, fastmail1]

campaign:
  start: 2026-12-01 08:00     # set your own date
timezone: Europe/Warsaw
business_hours: { start: "08:00", end: "17:00", days: [mon, tue, wed, thu, fri] }

speed:
  ramp_days: 30
  start_per_day: 1
  business_per_hour: 3        # peak
  off_hours_per_day: [1, 2]   # peak

dkim_selector: mail           # your DKIM selector
```

For a second domain add a second file. For a slower start, raise `ramp_days` or lower
`business_per_hour`.

## 4. Outlook.com only: app registration

1. In the Microsoft Entra admin center register an application that accepts **personal
   Microsoft accounts**.
2. Under Authentication allow **public client flows**. No client secret is needed.
3. Put the application (client) ID into the `outlook1` endpoint as `clientid:`.
4. In the Outlook.com mailbox enable IMAP.

## 5. Run it, step by step

Linux: run every command as the service user, with the secrets loaded (install first as in
[README.md](README.md#linux-service)):

```bash
w="sudo systemd-run --pty --uid=warmingup -p EnvironmentFile=/etc/warmingup/warmingup.env /usr/local/bin/warmingup"
c="/etc/warmingup/warmingup.yaml"
```

Windows (elevated PowerShell):

```powershell
$w = "$env:ProgramFiles\warmingup\warmingup.exe"; $c = "$env:ProgramData\warmingup\warmingup.yaml"
```

The commands below use the PowerShell form; on Linux type `$w validate --config $c`.
### Step 1: check files and DNS

```powershell
& $w validate --config $c
```

```text
example.com (C:\ProgramData\warmingup\domains.d\example.com.yaml, enabled: true)
  from "Example News" <news@example.com>, envelope bounces@example.com, peak 29 per day, start 2026-12-01 08:00 CET
  SPF    v=spf1 ip4:192.0.2.10 -all
  DMARC  v=DMARC1; p=quarantine
  DKIM   key published at mail._domainkey.example.com
Configuration is valid.
```

(DNS values are illustrative.) Fix every `WARNING` before going on.

### Step 2: sign in to the mailboxes

```powershell
& $w auth outlook1 --config $c        # shows a web address and a code
& $w validate --config $c --connect
```

Each mailbox adds a line like this:

```text
  target gmail1         ok: you.seed@gmail.com, inbox "INBOX", junk "[Gmail]/Spam"
  bounce bounce-example ok
```

If a junk folder is not found, the error lists the folder names: set `junk:` on that
endpoint to the right one.

### Step 3: look at the plan

```powershell
& $w plan --config $c --days 30
```

Selected rows from the sample configuration (5 targets):

```text
DATE        DAY  RAMP  BUSINESS  OTHER  TOTAL
2026-12-01  Tue  1     1         0      1
2026-12-05  Sat  5     0         2      2
2026-12-08  Tue  8     2         0      2
2026-12-15  Tue  15    5         0      5
2026-12-22  Tue  22    11        0      11
2026-12-29  Tue  29    24        2      26
2026-12-30  Wed  30    27        1      28

Highest load per mailbox and day (all listed domains, upper bound):
  you.seed@gmail.com                       6
```

### Step 4: one real message to every mailbox

```powershell
& $w once --config $c --target gmail1
```

This sends immediately and waits; the first look is after 5 minutes. Illustrative result:

```text
Sent "..." to you.seed@gmail.com
  Message-ID: <...@example.com>
  Server:     250 2.0.0 Ok: queued
Waiting for the message (first look after 5 minutes)...
  State:      delivered
  Placement:  inbox (INBOX)
  Latency:    41s
  Auth:       spf=pass dkim=pass dmarc=pass
  Sender IP:  192.0.2.10  PTR mail.example.com (forward-confirmed)
```

Repeat for each target and fill the checklist. A message that does not arrive is reported
lost only after about 40 minutes; you can test several targets at once in separate windows.
Do not start the service while SPF, DKIM, DMARC or the PTR check fail, or while a message is
reported as lost.
| Exit code | Meaning |
|---|---|
| 0 | inbox, in time |
| 1 | junk, or late |
| 2 | lost, or sending failed |

### Step 5: start the service

```powershell
& $w service install --config $c      # Windows
& $w service start
```

```bash
sudo systemctl enable --now warmingup  # Linux
```

Nothing is sent before `campaign.start`.

## 6. Watch the results

```powershell
& $w status --config $c
```

The examples show made-up results on the morning of 2026-12-08, a week after the start:

```text
example.com  ramp day 8, today: 0 sent, 2 pending, 0 missed, 0 failed, 0 skipped
  gmail1           ramp day 8   last 2: 2 inbox, 0 junk, 0 lost
  yahoo1           ramp day 8   last 2: 1 inbox, 1 junk, 0 lost
  outlook1         ramp day 8   last 2: 2 inbox, 0 junk, 0 lost
  wp1              ramp day 8   last 2: 2 inbox, 0 junk, 0 lost
  fastmail1        ramp day 8   last 1: 1 inbox, 0 junk, 0 lost
```

```powershell
& $w report --config $c --last 7d                 # --format csv or json to export
```

```text
DOMAIN       DAY         TARGET     SENT  INBOX  JUNK  LOST  LATE  PENDING  SEND_FAILED  BOUNCED  SPF_PASS  DKIM_PASS  DMARC_PASS  AVG_LATENCY_S
example.com  2026-12-01  gmail1     1     1      0     0     0     0        0            0        1         1          1           41
example.com  2026-12-02  yahoo1     1     1      0     0     0     0        0            0        1         1          1           88
example.com  2026-12-03  outlook1   1     1      0     0     0     0        0            0        1         1          1           62
example.com  2026-12-04  wp1        1     1      0     0     0     0        0            0        1         1          1           120
example.com  2026-12-05  fastmail1  1     1      0     0     0     0        0            0        1         1          1           18
example.com  2026-12-05  gmail1     1     1      0     0     0     0        0            0        1         1          1           37
example.com  2026-12-06  yahoo1     1     0      1     0     0     0        0            0        1         1          1           95
example.com  2026-12-07  outlook1   1     1      0     0     0     0        0            0        1         1          1           55
example.com  2026-12-07  wp1        1     1      0     0     1     0        0            0        1         1          1           660

Latest problems (1 total):
  12-06 14:10  example.com -> yahoo1: junk: spf=pass dkim=pass dmarc=pass
```
For Gmail, "inbox" includes the Promotions and other category tabs; the tool cannot tell
them apart. Look at the mailbox now and then.

### When something is held

`status` exits with 1 when a target is held and 2 when the whole domain is held, and shows
the reason. A held target keeps its volume and stops growing; a held domain stops growing
until you act. Find the cause (`report`, bounce texts, DNS), fix it, then:

```powershell
& $w resume yahoo1 --config $c --domain example.com
```

### Routine

| When | Do |
|---|---|
| Daily, first week | `status`; open one or two mailboxes and read the mail |
| Weekly | `report --last 7d`; check bounce reasons and the junk share |
| After any change to DNS or the mail server | `validate`, then `once` to one target |
| After editing the configuration | restart the service |

## 7. Problems

| Symptom | Likely cause |
|---|---|
| `validate --connect` fails to sign in | normal password instead of app password; IMAP not enabled |
| `no folder has the \Junk attribute` | set `junk:` to one of the listed folders |
| `once` reports lost, but the mail is in the mailbox | junk folder wrong, or the server's Message-ID search does not work; report it |
| Warning about the envelope sender | your server rewrites Return-Path; bounces will not be seen |
| `not signed in` for Outlook.com | run `auth` again, with the same `--datadir` as the service |
| `access denied` on Windows | use an elevated PowerShell |
| Nothing is sent | before `campaign.start`, after `campaign.end`, `enabled: false`, or no content could be fetched (see the log) |

Logs: `journalctl -u warmingup -f` on Linux; `<datadir>\logs\` and the Application event log
on Windows.

## What this does and does not achieve

A handful of your own mailboxes mainly shows **where your mail lands**. It builds little
reputation by itself. Use the results to fix problems, and grow real, engaged recipients
alongside it.
