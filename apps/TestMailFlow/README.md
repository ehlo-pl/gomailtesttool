# TestMailFlow (`testmailflow`)

`testmailflow` tests mail flow end to end. It sends a small probe message over one channel and
verifies over another channel that it arrived:

```
            send (SMTP | Microsoft Graph)                 check (Graph | Gmail API | IMAP | POP3)
probe ───────────────────────────────────▶  mail system  ───────────────────────────────────▶ found?
        every <interval>                                    after 5 min, +2.5, +5, +10, +15 min
```

It runs as a native **Windows service** (or in the foreground / under systemd on Linux), keeps every
probe in a local SQLite database and reports through `status` / `report`, the **Windows Event Log**
and an optional **HTTP endpoint** (`/healthz`, `/metrics`).

> Original requirement: *a Windows (later Linux) service which tests mail flow: send mail to a specific
> address and check with another channel/method whether it arrived, e.g. send over SMTP and after
> 5 minutes (then a further 2.5, 5, 10 and 15 minutes) check over POP3 or Graph. Sender and the
> recipient used to collect results may differ.*

It reuses gomailtesttool's protocol code: the fetch sessions also used by `dmarcanalyzer`
(`internal/common/fetch`) and a new quiet send layer (`internal/common/send`).

## Build

```bash
make build-tmf                   # bin/testmailflow(.exe)
./build-all.ps1 -MailFlow        # Windows / PowerShell
go build -o bin/testmailflow ./apps/TestMailFlow/cmd/testmailflow
```

Pure Go, no CGO. Release ZIPs include the binary.

## Quick start

```powershell
# 1. Describe endpoints and flows (see "Configuration")
notepad C:\tmf\tmf.yaml

# 2. Check the file; --connect also logs in to the checked mailboxes
testmailflow validate --config C:\tmf\tmf.yaml --connect

# 3. Send one probe per flow and wait for the result
testmailflow once --config C:\tmf\tmf.yaml

# 4. Install and start the service (elevated prompt)
testmailflow service install --config C:\tmf\tmf.yaml
testmailflow service start

# 5. Look at the results
testmailflow status --config C:\tmf\tmf.yaml --datadir C:\ProgramData\testmailflow
testmailflow report --last 7d --datadir C:\ProgramData\testmailflow
```

## Configuration

One YAML file holds the global settings, named **endpoints** (connections) and **flows** (routes to test).

```yaml
testmailflow:
  http: 127.0.0.1:9925          # /healthz and /metrics; omit to disable
  retention: 90d                # keep finished probes this long

endpoints:
  relay:                        # on-premises relay, anonymous on port 25
    protocol: smtp
    host: smtp.contoso.com
    port: 25
  exo:                          # Exchange Online mailbox via Microsoft Graph
    protocol: msgraph
    tenantid: 00000000-0000-0000-0000-000000000000
    clientid: 00000000-0000-0000-0000-000000000000
    thumbprint: 0123456789ABCDEF0123456789ABCDEF01234567
    mailbox: probe@contoso.com
  popbox:                       # external mailbox
    protocol: pop3
    host: pop.example.net
    pop3s: true
    username: probe@example.net
    password: ${TMF_POPBOX_PASSWORD}

flows:
  - name: onprem-to-exo
    send:  { endpoint: relay, from: probe@onprem.contoso.com, to: probe@contoso.com }
    check: { endpoint: exo, folder: inbox, delete: true }
    interval: 15m
    checks: [5m, 2m30s, 5m, 10m, 15m]
    warn_after: 5m

  - name: exo-to-internet
    send:  { endpoint: exo, to: probe@example.net }      # from = the Graph mailbox
    check: { endpoint: popbox, delete: true }
```

### Endpoints

`protocol` selects the gomailtest protocol; all other keys are the **flag names of
`gomailtest <protocol>`** (`host`, `port`, `username`, `password`, `starttls`, `smtps`, `tenantid`,
`clientid`, `secret`, `pfx`, `thumbprint`, `mailbox`, `skipverify`, `proxy`, …).

| Role | Protocols | Notes |
|---|---|---|
| send | `smtp` | Opportunistic STARTTLS on ports 25/587/2525 (force with `starttls: true`, disable with `no-starttls: true`), `smtps: true` for implicit TLS, optional authentication, `use-mx: true` to deliver straight to the recipient's MX (no `host`). |
| send | `msgraph` | MIME `sendMail` as `mailbox`. Application permission **Mail.Send**. |
| check | `msgraph` | Application permission **Mail.Read** (**Mail.ReadWrite** with `delete: true`). |
| check | `gmail` | Service account (`credentials`) or `bearertoken`; `gmail.readonly` (`gmail.modify` with `delete: true`). |
| check | `imap`, `pop3` | Password or `accesstoken` (XOAUTH2). `imaps: true` / `pop3s: true` pick ports 993 / 995. POP3 needs UIDL. |

`${NAME}` is replaced by the environment variable `NAME` (only the braced form, so passwords may
contain `$`); a missing variable is an error. For the service, define such variables machine-wide.
Interactive sign-in (delegated Graph, Gmail OAuth) is rejected: a service cannot answer prompts.

### Flows

| Key | Default | Meaning |
|---|---|---|
| `name` | – | Letters, digits, `.`, `_`, `-`. Appears in the subject, logs, events and metrics. |
| `send.endpoint` / `send.from` / `send.to` | – | Sender connection, envelope/header sender, recipient(s). `from` defaults to the mailbox of a Graph endpoint. |
| `check.endpoint` / `check.folder` | folder: protocol default (INBOX) | Mailbox to look in; may be a different account and system than the sender. |
| `check.delete` | `false` | Remove found probe messages (Graph: Deleted Items, Gmail: Trash, IMAP: expunge, POP3: DELE). |
| `interval` | `15m` | Time between probes (minimum 30s). |
| `checks` | `[5m, 2m30s, 5m, 10m, 15m]` | Delays between check attempts. The probe is **lost** when the last attempt finds nothing (default 37m30s after sending). |
| `warn_after` | first `checks` value | A probe that needed longer is **late**. |

Durations: Go syntax (`30s`, `2m30s`, `1h`) plus `d` and `w`.

## How a probe is judged

| State | Meaning |
|---|---|
| `pending` | Sent, not yet found. |
| `delivered` | Found, latency ≤ `warn_after`. |
| `late` | Found, but slower than `warn_after` (or found after it had been reported lost). |
| `lost` | Not found by the last check. |
| `send_failed` | The sending server rejected the message or was unreachable. |

* The probe has subject `[TMF] <flow> <token>`, headers `X-TestMailFlow-Id` / `X-TestMailFlow-Flow` and
  `Auto-Submitted: auto-generated`. It is recognized by the header, or by the subject if a gateway strips
  custom headers (subject tags such as `[EXTERNAL]` are fine).
* **Latency** is the time from submission to the newest `Received` header of the delivered message, so it
  does not depend on when the check happened. Hop count and the receiving system's
  `Authentication-Results` verdicts (spf / dkim / dmarc) are stored with the probe.
* One mailbox session per check resolves all pending probes of that mailbox; messages already examined
  are never downloaded twice.
* **A mailbox that cannot be checked is not lost mail.** The check attempt is retried every minute, the
  probe stays `pending` and a separate "checker failing" event is raised.
* Probes are stored before they are sent, and pending probes survive a restart of the service.

## Commands

| Command | Purpose |
|---|---|
| `run` | Run all flows until stopped (what the service executes). `--http`, `--retention`. |
| `once [--flow NAME]… [--wait=false]` | Send one probe per flow now and follow the check schedule. Exit 0 in time, 1 late, 2 lost / send failed. |
| `status [--format text\|csv\|json]` | Health of every flow. Exit 0 ok, 1 degraded/unknown, 2 failing or stale (no recent probe). |
| `report [--last 7d \| --from … --to …] [--flow NAME] [--format …] [--out FILE]` | Sent / delivered / late / lost, success %, latency min/avg/p95/max, latest problems. |
| `validate [--connect]` | Check the config and print the plan with secrets masked. |
| `db stats` / `db vacuum` / `db prune --older-than 30d` | Maintenance. |
| `service install\|uninstall\|start\|stop\|status` | Windows service management (administrator). |

### Global flags

| Flag | Env | Default |
|---|---|---|
| `--config` | `TMFCONFIG` | – |
| `--datadir` | `TMFDATADIR` | Per user: `%LOCALAPPDATA%\testmailflow`, `~/.local/share/testmailflow`. The installed service uses `%ProgramData%\testmailflow`. |
| `--db` | `TMFDB` | `<datadir>/testmailflow.db` |
| `--verbose`, `--loglevel` | `TMFVERBOSE`, `TMFLOGLEVEL` | `INFO` |
| `--http` (`run`) | `TMFHTTP` | disabled |

`status` and `report` read the database, so point them at the service's data directory
(`--datadir C:\ProgramData\testmailflow`).

## Windows service

```powershell
testmailflow service install --config C:\tmf\tmf.yaml [--datadir D:\tmf-data] [--user "DOMAIN\svc-tmf"]
testmailflow service start
testmailflow service status
testmailflow service stop
testmailflow service uninstall      # keeps the data directory
```

* Installed as `testmailflow`, automatic (delayed) start, restarted by the SCM after a failure (1, 5, 15 min).
* Runs as LocalSystem unless `--user` is given; the password is read from the `TMFSERVICEPASSWORD`
  environment variable (never from the command line). gMSA accounts (`DOMAIN\name$`) need no password.
* The account needs read access to the config file, write access to the data directory and, for
  `thumbprint` authentication, access to the certificate's private key. Protect the config file with NTFS
  permissions if it contains secrets.
* Log: `<datadir>\logs\testmailflow-YYYY-MM-DD.log`.

### Event Log

Source `testmailflow` in the Application log. Events are written when a flow **changes state**, not for
every probe.

| ID | Level | Meaning |
|---|---|---|
| 100 / 101 | Information (101 Error when stopping on a failure) | Service started / stopped |
| 102 | Information | Flow recovered (probe arrived in time again) |
| 103 | Information | Checked mailbox reachable again |
| 200 | Warning | Probe arrived late |
| 300 | Error | Probe lost |
| 301 | Error | Probe could not be sent |
| 302 | Error | Checked mailbox cannot be read (credentials, connectivity) |

```powershell
Get-WinEvent -ProviderName testmailflow -MaxEvents 20
```

### Linux (systemd)

```ini
[Service]
ExecStart=/usr/local/bin/testmailflow run --config /etc/testmailflow.yaml --datadir /var/lib/testmailflow
Restart=on-failure
User=testmailflow
```

## HTTP endpoint

Enabled with `http:` in the config, `--http` or `TMFHTTP`. Bind it to loopback unless a monitoring
system must reach it; it is unauthenticated and read-only.

* `GET /healthz` – JSON with every flow's health; **200** when nothing is failing, **503** otherwise.
* `GET /metrics` – Prometheus text format: `tmf_flow_up`, `tmf_flow_health`, `tmf_probes_total{flow,state}`,
  `tmf_pending_probes`, `tmf_last_latency_seconds`, `tmf_last_success_timestamp_seconds`.

## Database schema (SQLite)

| Table | Content |
|---|---|
| `probes` | flow, token, Message-ID, sent time, send result (server response, host, TLS), state, check attempts, arrival time, latency, hops, authentication results, errors |
| `seen_keys` | message keys already examined per check endpoint |
| `events` | state changes (the same ones written to the Event Log) |

Timestamps are Unix seconds (UTC).

## Code layout

```
apps/TestMailFlow/
  cmd/testmailflow/       main
  internal/cli/           cobra commands, endpoint → protocol config mapping
  internal/config/        global config (TMF* env vars), YAML endpoints and flows, validation
  internal/probe/         probe message build and recognition
  internal/engine/        scheduler and probe state machine
  internal/store/         SQLite schema and queries
  internal/report/        status, statistics, text / CSV / JSON renderers
  internal/httpd/         /healthz, /metrics
  internal/eventlog/      Windows Event Log sink (no-op elsewhere)
  internal/svc/           Windows service host and SCM management
internal/common/send/     protocol-neutral send.Sender API
internal/protocols/{smtp,msgraph}/sendraw.go   quiet senders
internal/common/fetch/ + internal/protocols/*/fetchraw.go   mailbox reading (shared with dmarcanalyzer)
```
