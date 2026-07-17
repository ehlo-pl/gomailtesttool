# Config File (`--config`)

Every `gomailtest <protocol> <action>` command accepts a `--config <path>` flag
pointing to a YAML file that provides default values for flags.

```powershell
gomailtest smtp testauth --config ./smtp-prod.yaml
```

## Precedence

Values are resolved in this order (highest wins):

1. Explicit CLI flags (e.g. `--port 587`)
2. Environment variables (e.g. `SMTPPORT`)
3. `--config` YAML file
4. Built-in flag defaults

So a config file sets convenient defaults for a given environment, while
still letting you override individual values from the command line or
environment variables without editing the file.

## File format

Keys in the YAML file are **flag names** (without the leading `--`), not
environment variable names. Use the flag tables in each protocol's docs
(e.g. [docs/protocols/smtp.md](protocols/smtp.md),
[docs/protocols/msgraph.md](protocols/msgraph.md)) for the full list of
available keys. Hyphenated flags (e.g. `--inline-attachments`,
`--body-template`) use the hyphenated form as the YAML key.

### Example: SMTP

```yaml
# smtp-prod.yaml
host: smtp.example.com
port: 587
starttls: true
username: user@example.com
password: secret
authmethod: auto
from: sender@example.com
to: recipient@example.com
subject: "Test Email"
body: "This is a test message"
bodyhtml: "<p>This is a <strong>test</strong> message</p>"
attachments: ./report.pdf,./data.csv
inline-attachments: ./logo.png
header:
  - "X-Custom-Header: example-value"
```

Repeatable flags (`--header`, `--template-vars`) are written as YAML lists,
as shown above. A `--template` file can also be configured here, e.g.:

```yaml
template: ./welcome.eml
template-vars:
  - "Name=World"
  - "Rcpt=user@example.com"
```

```powershell
gomailtest smtp sendmail --config ./smtp-prod.yaml
```

### Example: Microsoft Graph

```yaml
# msgraph-prod.yaml
tenantid: 00000000-0000-0000-0000-000000000000
clientid: 00000000-0000-0000-0000-000000000000
secret: your-client-secret
mailbox: user@example.com
to: recipient@example.com
subject: "Test Email"
bodyHTML: "<p>Hello from gomailtest</p>"
attachments: ./report.pdf
```

```powershell
gomailtest msgraph sendmail --config ./msgraph-prod.yaml
```

## `serve` mode

`gomailtest serve` also accepts `--config`. Top-level keys (`port`, `listen`,
`api-key`) configure the HTTP server itself; nested `smtp:` and `msgraph:`
sections provide defaults for the SMTP and Microsoft Graph base configuration
that would otherwise come only from `SMTP*`/`MSGRAPH*` environment variables
(see [docs/protocols/serve.md](protocols/serve.md) and
[.env.example](../.env.example)). Environment variables still take precedence
over the config file sections.

```yaml
# serve.yaml
port: 8080
api-key: change-me

smtp:
  host: smtp.example.com
  port: 587
  starttls: true
  username: user@example.com
  password: secret
  from: sender@example.com

msgraph:
  tenantid: 00000000-0000-0000-0000-000000000000
  clientid: 00000000-0000-0000-0000-000000000000
  secret: your-client-secret
  mailbox: user@example.com
```

```powershell
gomailtest serve --config ./serve.yaml
```

## Notes

- An empty or omitted `--config` is a no-op; all values fall back to flag
  defaults (and env vars / CLI flags, as usual).
- If `--config` points to a file that doesn't exist or can't be parsed as
  YAML, the command fails with an error rather than silently ignoring it.
- Secrets (passwords, client secrets, access tokens) can be placed in a
  config file, but treat it like any other credentials file: keep it out of
  version control and restrict its permissions.
