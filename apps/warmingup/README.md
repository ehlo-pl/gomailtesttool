# warmingup

`warmingup` warms up sending domains. It sends a slowly growing number of ordinary looking
messages through your own mail server to mailboxes you own, records where each one lands,
and stops increasing the volume when results turn bad.

```
            SMTP (your MTA, DKIM signs)              IMAP | JMAP
warmingup ───────────────────────────────▶ providers ◀──────────── placement, headers, mark read, rescue
    ▲                                          │
    └────────────── bounce mailbox ◀───────────┘  delivery status notifications
```

It runs on Linux (systemd) and on Windows (native service).

**New here? Follow the walkthrough in [USAGE.md](USAGE.md)**: mailboxes to prepare, example
configuration and example output.

> Original requirement: *using methods from TestMailFlow build a simple Linux based warming service.*

**What it does not do:** a handful of your own mailboxes with automated engagement mainly
*measures* placement. It builds little reputation by itself and providers may discount it.
Real, engaged recipients still matter.

## Build

```bash
make build-warmup                 # bin/warmingup(.exe)
./build-all.ps1 -Warmup           # Windows / PowerShell
GOOS=linux GOARCH=amd64 go build -o bin/warmingup ./apps/warmingup/cmd/warmingup
```

## Quick start

Linux paths are shown; for Windows see [Windows service](#windows-service).

```bash
warmingup validate --config /etc/warmingup/warmingup.yaml            # files and DNS records
warmingup auth outlook1 --config /etc/warmingup/warmingup.yaml       # Outlook.com only, once
warmingup validate --config /etc/warmingup/warmingup.yaml --connect  # sign in to every mailbox
warmingup plan     --config /etc/warmingup/warmingup.yaml            # projected volume
warmingup once     --config /etc/warmingup/warmingup.yaml --target gmail1   # one real message
systemctl enable --now warmingup
```

Run `once` against **every** target before starting the service. Check that the message is
found, that SPF, DKIM and DMARC pass, and that the sender IP has a forward-confirmed PTR record.

## Configuration

One global file with the mailbox connections, and one file per warmed domain.
Examples: [deploy/warmingup.yaml](deploy/warmingup.yaml),
[deploy/domains.d/example.com.yaml](deploy/domains.d/example.com.yaml).
Unknown keys are errors. `${NAME}` in endpoint settings is replaced by the environment
variable `NAME`, or else by `NAME` from the file named in `warmingup: envfile:`
(`NAME=value` lines, `#` comments, UTF-8). Changes apply after a restart; the ramp state is kept.

Global `warmingup:` keys: `datadir` (default `/var/lib/warmingup`, on Windows
`%ProgramData%\warmingup`), `domains_dir` (default `domains.d`), `envfile`, `loglevel`.
Relative paths are relative to the config file.

### Endpoints (global file)

Settings use the flag names of `gomailtest <protocol>`.

| Role | `protocol` | Notes |
|---|---|---|
| sender | `smtp` | Your own MTA or smart host. It must DKIM sign; warmingup does not. |
| target | `imap` | `imaps: true` or `starttls: true`. `junk:` names the junk folder when the server does not flag one. `address:` when the username is not the mail address. `engage: false` leaves the mail untouched. |
| target | `jmap` | `accesstoken:` (API token), `address:`. |
| bounce | `imap`, `pop3` | Mailbox of the envelope sender. Nothing is deleted. |

### Provider access

You create the mailboxes.

| Provider | Host | What you set up |
|---|---|---|
| Gmail | `imap.gmail.com` | 2-Step Verification, then an app password. The Spam label needs "Show in IMAP"; `junk: "[Gmail]/Spam"`. |
| Yahoo, AOL, iCloud | `imap.mail.yahoo.com`, `imap.aol.com`, `imap.mail.me.com` | App password. |
| WP, Onet, Interia, GMX | provider's IMAP host | Enable IMAP; check that your plan includes it. Usually needs `junk:`. |
| Outlook.com | `outlook.office365.com` | Enable IMAP. Register an Entra application that accepts personal Microsoft accounts and allows public client flows. Set `oauth: microsoft`, `clientid:`, then run `warmingup auth <endpoint>`. |
| Fastmail | `api.fastmail.com` (jmap) | API token (not offered on the Basic plan). Confirm it first with `gomailtest jmap testauth`. |

The Outlook.com token is stored in `<datadir>/tokens/<endpoint>.json`, and the service must
be able to rewrite it. On Linux run `auth` as the service user (see
[Linux service](#linux-service)); on Windows run it from an elevated prompt.

### Domain file

| Key | Default | Meaning |
|---|---|---|
| `domain`, `enabled` | –, `true` | Warmed domain. |
| `sender.endpoint` / `from` / `ehloname` | – | SMTP endpoint, header From, name announced in EHLO. |
| `bounce.endpoint` / `address` | none | Envelope sender and where its bounces are read. Must be on the From domain (or a parent or subdomain), and your server must not rewrite it (`once` warns). **Strongly recommended.** |
| `targets` | – | Target endpoint names. |
| `campaign.start` / `end` | – / open | `2006-01-02 15:04` in `timezone`. Nothing is sent outside; sent mail is still checked. |
| `timezone` | `Europe/Warsaw` | |
| `business_hours` | 08:00–17:00 mon–fri | `{start, end, days}` or `none` for one flat rate. |
| `holidays` | – | Dates treated like weekends. |
| `speed.ramp_days` / `start_per_day` / `curve` | `30` / `1` / `geometric` | Or `linear`. |
| `speed.business_per_hour` / `off_hours_per_day` | `3` / `[1, 2]` | Peak rate. |
| `speed.per_day` | – | Peak when `business_hours: none`. |
| `speed.min_gap` / `max_per_day` | `10m` / none | |
| `content.sources` / `categories` | `wikipedia, poetry, rfc` | As `gomailtest ... sendmail --body-source`. |
| `hold.window` / `min_samples` / `bad_ratio` / `late_ratio` / `dsn` / `send_failures` | `10` / `5` / `0.3` / `0.5` / `3` / `3` | See below. |
| `engage.min_delay` / `max_delay` | `10m` / `4h` | Random wait before marking read. |
| `warn_after` | `5m` | Slower delivery is "late". |
| `dkim_selector` | – | Lets `validate` check the key record. |

## Schedule

With the defaults the daily total on business days is about 1, 2, 5, 11 and 29 on ramp days
1, 8, 15, 22 and 30: at the peak 3 messages per business hour plus 1–2 outside. Weekends and
holidays send only the 1–2 off-hours messages. Send times are random, one recipient each,
spread evenly over the targets.

* The ramp day advances once per calendar day on which the service runs, weekends included.
  Days without the service do not count.
* Send times missed by more than 15 minutes are dropped, never sent in a burst.
* Content already sent is not sent again. When no content can be fetched, nothing is sent.
* Targets shared by several domains receive the sum; `plan` shows it.

## Results and holds

| State | Meaning |
|---|---|
| `pending` | Sent, not yet found. |
| `delivered` | Found within `warn_after`. |
| `late` | Found, but slower (including after having been reported lost). |
| `lost` | In neither inbox nor junk 37.5 minutes after sending. Looked for once more after 24 h. |
| `send_failed` | Your MTA rejected the message or was unreachable. |

Placement (`inbox` or `junk`) is recorded **before** anything is moved, with the full header block.

A **target is held** when more than `bad_ratio` of its last `window` messages went to junk or
were lost, more than `late_ratio` were late, or `dsn` of its last `window` messages have a bounce
or deferral report. A held target keeps its volume; the others keep ramping. The **domain is
held** after `send_failures` failed submissions in a row. Clear with
`warmingup resume [target] --domain example.com`.

Your MTA accepts a message long before Gmail or Outlook decide about it. Without a bounce
mailbox their deferrals are invisible, and delay notifications arrive only if your MTA sends them.

## Commands

| Command | Purpose |
|---|---|
| `run` | The service. |
| `once --target X [--wait=false]` | One real message now. Exit 0 inbox in time, 1 junk or late, 2 lost or failed. |
| `plan [--days 30]` | Projected volume and load per mailbox. |
| `status` | Ramp days, holds, recent placement. Exit 1 target held, 2 domain held. |
| `report [--last 7d] [--format text\|csv\|json]` | Per domain, day and target; latest problems. |
| `validate [--connect]` | Configuration, SPF / DKIM / DMARC, mailbox sign-in. |
| `auth <endpoint>` | Outlook.com device code sign-in. |
| `resume [target]` | Clear holds. |
| `service install\|uninstall\|start\|stop\|status` | Windows service. |

Global flags: `--config` (`WARMUPCONFIG`), `--datadir` (`WARMUPDATADIR`, overrides the
config file), `--loglevel` (`WARMUPLOGLEVEL`), `--domain`.

## Linux service

```bash
sudo useradd --system --home /var/lib/warmingup --shell /usr/sbin/nologin warmingup
sudo install -m 0755 bin/warmingup /usr/local/bin/
sudo install -d -m 0750 -g warmingup /etc/warmingup /etc/warmingup/domains.d
sudo install -m 0640 -g warmingup apps/warmingup/deploy/warmingup.yaml /etc/warmingup/
sudo install -m 0640 -g warmingup apps/warmingup/deploy/domains.d/example.com.yaml /etc/warmingup/domains.d/
sudo install -m 0600 /dev/null /etc/warmingup/warmingup.env     # NAME=value secrets
# The data directory must exist before 'auth' or 'once' run ahead of the first service start.
sudo install -d -m 0700 -o warmingup -g warmingup /var/lib/warmingup
sudo install -m 0644 apps/warmingup/deploy/warmingup.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now warmingup
journalctl -u warmingup -f
```

Edit the copied files before starting the service. `plan`, `status`, `report`, `resume` and
`validate` (without `--connect`) work without the secrets. `auth`, `once` and
`validate --connect` need them; run those as the service user:

```bash
sudo systemd-run --pty --uid=warmingup -p EnvironmentFile=/etc/warmingup/warmingup.env \
  /usr/local/bin/warmingup once --config /etc/warmingup/warmingup.yaml --target gmail1
```

## Windows service

From an **elevated** PowerShell in the repository, after `./build-all.ps1 -Warmup`:

```powershell
$app  = "$env:ProgramFiles\warmingup"      # only administrators can replace the program
$conf = "$env:ProgramData\warmingup"       # configuration, secrets, database, tokens, logs
New-Item -ItemType Directory -Force $app, "$conf\domains.d" | Out-Null
# Before any secret is written: only SYSTEM and Administrators (by default all users can read).
icacls $conf /inheritance:r /grant:r "*S-1-5-18:(OI)(CI)F" "*S-1-5-32-544:(OI)(CI)F"
Copy-Item bin\warmingup.exe $app
Copy-Item apps\warmingup\deploy\warmingup.yaml $conf
Copy-Item apps\warmingup\deploy\domains.d\example.com.yaml "$conf\domains.d"
notepad "$conf\warmingup.env"              # NAME=value, one per line
notepad "$conf\warmingup.yaml"             # enable "envfile: warmingup.env", set your servers
notepad "$conf\domains.d\example.com.yaml"

$w = "$app\warmingup.exe"; $c = "$conf\warmingup.yaml"
& $w validate --config $c
& $w auth outlook1 --config $c             # Outlook.com only
& $w validate --config $c --connect
& $w once --config $c --target gmail1      # repeat for every target
& $w service install --config $c
& $w service start
```

* **Account.** The service runs as `LocalSystem`. For less privilege use
  `--user "NT AUTHORITY\LocalService"`; for a domain account set its password in
  `WARMUPSERVICEPASSWORD` before `service install`.
* **Permissions.** `service install` limits the data directory to SYSTEM, Administrators and
  the service account, so later commands (`status`, `report`, `once`, ...) need an elevated
  prompt. A data directory outside `%ProgramData%` that already holds other files is left
  unchanged, with a warning.
* **Secrets.** The service does not see your user environment variables. Use `envfile`.
* **Logs.** `<datadir>\logs\warmingup-YYYY-MM-DD.log` (dates in UTC; old files are not
  removed). The Application event log, source `warmingup`: 100 started, 101 stopped,
  200 target held, 300 domain held.
* **Updates.** `service stop`, replace the program, `service start`. Restart the service after
  configuration changes.
* **Sleep.** Nothing is sent while the computer sleeps; use an always-on machine.
