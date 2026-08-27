# Codebase Size and Cost Analysis

## Size

| Metric | Value |
|---|---|
| Total files (excl. `.git`) | 387 |
| Total lines of code | ~58,500 |
| Go source lines (production) | ~26,750 |
| Go test lines | ~12,900 |
| Non-Go files (docs, YAML, etc.) | ~18,800 |
| Disk size (excl. `.git`) | 3.1 MB |

### File Type Breakdown

| Extension | Count |
|---|---|
| `.go` | 176 |
| `.md` | 148 |
| `.yaml` / `.yml` | 29 |
| `.env` | 24 |
| Other (Makefile, Dockerfile, etc.) | 10 |

### Package Structure

The codebase contains ~30 packages. The largest areas:

| Directory | Files |
|---|---|
| `internal/protocols` | 69 |
| `internal/common` | 20 |
| `internal/devtools` | 11 |
| `cmd/gomailtest` | 9 |
| `internal/serve` | 8 |
| `internal/smtp` | 4 |
| `internal/pop3` | 3 |
| `internal/jmap` | 3 |
| `internal/imap` | 1 |

Protocol packages inside `internal/protocols`: EWS (19 files), Microsoft Graph (10), IMAP (9), POP3/JMAP/SMTP (8 each), Gmail (6).

---

## Cost

### Dependencies

- **15 direct dependencies**, ~70 indirect dependencies (243 entries in `go.sum`).
- Key direct dependencies: Azure SDK, Microsoft Graph SDK, `go-imap`, SASL, JWT, Kerberos (`gokrb5`), MCP SDK, Cobra, Viper, Google API, OAuth2, PKCS12.

### Cost Drivers

1. **Build time** — heavy dependency tree (Azure, Google, and Microsoft Graph SDKs) makes cold builds slow.
2. **Binary size** — expect a large statically-linked binary (~50–100 MB) due to the included SDK giants.
3. **Maintenance cost** — wide dependency graph; keeping Azure, Graph, Google, and Kerberos libraries current requires ongoing attention.
4. **Test coverage** — ~33% of Go lines are tests (12,900 / 39,650), which is reasonable but leaves room for improvement.
5. **No infrastructure costs** — the project is a CLI/tool with no managed cloud services, databases, or hosted infrastructure.
