# Code Review Results - gomailtesttool

**Scope:** Full codebase review (~26k lines Go) with short- and long-term improvement plan, 2026-07-07.
**Version reviewed:** 3.4.1 (branch `b3.4.2`).
Builds on the 2026-06-14 code review and the 2026-06-22 architecture evaluation; verifies their status and adds new findings.

## 0. Status of previous findings

Fixed since 2026-06-14:

- Masking consolidation (June 1.1) — **mostly done**: `smtp`, `jmap`, and `msgraph` now import `internal/common/security`; the dead/buggy `msgraph.maskSecret` (June 1.2) is gone. Two leftovers remain (see 1.3 below).
- POP3 `ReadResponseWithTimeout` (June 1.3) — fixed in commit `4f8f30d`.
- EWS autodiscover hardcoded `https://` scheme (June 2.1) — fixed; no hardcoded scheme remains in `autodiscover.go`.

Still open from the 2026-06-22 architecture evaluation (all confirmed still present):

- Repeated 7-step command lifecycle across every subcommand (§3.2)
- Duplicated signal-context creation: root `PersistentPreRunE` **and** every action `RunE` call `bootstrap.SetupSignalContext()` (§3.4)
- CLI/serve execution boundary not extracted (§3.3/3.5)
- `internal/common/retry` at 0% coverage; protocol action packages at 4–13% (§4.2/4.3)

> **Status update (same day):** Finding 1.1 is fixed — `retry.RetryWithBackoffFunc`
> now accepts a classifier, and msgraph supplies `isRetryableGraphError`
> (typed `ODataError` check for 429/503/504, honoring `Retry-After`, capped at
> 5 min). Finding 1.2 is half-done: the retry package now has tests (97.4%
> coverage) but still logs via `log.Printf`; the slog conversion remains open.
> 1.4's `IsSMTPRetryableError` was kept (now tested) but is still unwired.

## 1. New findings

### 1.1 Major (correctness): Graph API throttling and service errors are never actually retried

- **Location:** `internal/common/retry/retry.go:15-47` (`IsRetryableError`), used via `internal/protocols/msgraph/utils.go:199` (`retryWithBackoff`) by every Graph call in `handlers.go`.
- **Issue:** `RetryWithBackoff` bails out immediately when `IsRetryableError(err)` is false. `IsRetryableError` classifies only by substring-matching network error text (`"timeout"`, `"connection reset"`, …). A Kiota `*odataerrors.ODataError` for HTTP 429 (`TooManyRequests`/`activityLimitReached`) or 503/504 does not contain any of those substrings, so the retry loop returns on the **first** attempt.
- **Impact:** The `--maxretries`/`--retrydelay` flags are effectively dead for the most common transient Graph failures — exactly the throttling scenarios the tool is likely to hit when exporting mailboxes. `enrichGraphAPIError` then correctly extracts `Retry-After` and prints "retry after Xs" advice, but only **after** the tool has already given up; the machinery to wait and retry exists but is never engaged.
- **Recommendation:**
  1. Add typed classification to the retry path: `errors.As(err, &odataErr)` → retry on response status 429/503/504 (and honor the `Retry-After` header for the delay instead of blind exponential backoff).
  2. Keep the string patterns as a fallback for plain network errors.
  3. Cover with table-driven tests (fake `ODataError`s with each status).
- **Design note:** the cleanest shape is an optional classifier: `RetryWithBackoffFunc(ctx, max, delay, op, isRetryable func(error) bool)` so msgraph passes a Graph-aware classifier and other protocols keep the default — avoids importing Kiota types into `internal/common`.

### 1.2 Medium (test + convention debt): `internal/common/retry` has zero tests and logs via stdlib `log`

- **Location:** `internal/common/retry/retry.go:82,104` (`log.Printf`), no `retry_test.go`.
- **Issue:** The package that decides user-visible reliability behavior is the only shared package with 0% coverage (June 22 §4.3, still true). It also bypasses the repo's slog convention with global `log.Printf`, producing differently-formatted lines on stderr that ignore `--loglevel`/`--logformat`.
- **Recommendation:** Fix together with 1.1 — accept a `*slog.Logger` (or return attempt info to the caller for logging) and add table-driven tests: success after N attempts, non-retryable short-circuit, context cancellation mid-wait, delay capping. Small package, high leverage.

### 1.3 Minor: finish the masking consolidation (two 1-line call sites)

- **Location:** `internal/protocols/imap/utils.go` and `internal/protocols/pop3/utils.go` — private `maskUsername` copies, each used exactly once (`imap/exportmessages.go:62`, `pop3/exportmessages.go:72`).
- **Issue:** Everything else in those same packages already uses `security.MaskUsername`. These two copies were added by commit `eff7fa5` ("Fix lint failures: add maskUsername to imap/pop3") — i.e., the lint fix re-introduced the duplication that the June review's top recommendation was eliminating.
- **Recommendation:** Switch both call sites to `security.MaskUsername` and delete both `utils.go` files. Closes June 1.1 completely.

### 1.4 Minor (dead code): `retry.IsSMTPRetryableError` is never called

- **Location:** `internal/common/retry/retry.go:52-60`.
- **Issue:** Defined, exported, documented — zero callers. Either the SMTP send path should classify 4xx responses as retryable (plausible original intent), or the function should go.
- **Recommendation:** Decide intent while touching the package for 1.1/1.2; delete if SMTP retry classification isn't wanted.

### 1.5 Medium (CI/process): lint is non-blocking and unconfigured; no vulnerability scanning or dependency automation

- **Location:** `.github/workflows/build.yml:40` (`continue-on-error: true`), no `.golangci.yml`, no `.github/dependabot.yml`, no `govulncheck` step.
- **Issue:**
  - Lint failures already slip into main and get fixed after the fact (commit `eff7fa5` is exactly that) — and that fix itself introduced finding 1.3. Non-blocking lint is demonstrably costing quality here.
  - For a tool whose whole job is handling credentials (client secrets, PFX passwords, Kerberos, JWTs), there is no automated CVE signal: dependency updates are manual and `govulncheck` never runs.
- **Recommendation:**
  1. Add a minimal `.golangci.yml` (start with the default set + `errcheck`, `gosec`), fix current findings, then flip `continue-on-error` to `false`.
  2. Add a `govulncheck ./...` step to CI (one job, ~2 min).
  3. Add `.github/dependabot.yml` for `gomod` + `github-actions` (weekly).

### 1.6 Low (dependency debt): `go-imap/v2` is a beta pin

- **Location:** `go.mod:9` — `github.com/emersion/go-imap/v2 v2.0.0-beta.7`.
- **Issue:** Core IMAP functionality rides on a pre-release API that can still break between betas. Not urgent (pinned, tests pass) but worth tracking for GA; dependabot (1.5) will surface new betas/GA automatically.

### 1.7 Housekeeping

- `internal/common/version/version.go` still says `3.4.1` while the branch is `b3.4.2` — bump pending release (per convention).
- `TODO.md` is 100% checked items — archive to `ChangeLog/` or `cr-results/` and start a fresh one so it regains signal.
- 60+ per-version files in `ChangeLog/` and 8 reports in `cr-results/` — consider a yearly roll-up; low priority.

## 2. Prioritized backlog

Score = (Impact + Risk) × (6 − Effort), each 1–5.

| # | Item | Impact | Risk | Effort | Score |
|---|------|--------|------|--------|-------|
| 1 | Graph retry classification + Retry-After (1.1) | 4 | 4 | 2 | 32 |
| 2 | retry pkg tests + slog (1.2) | 3 | 3 | 1 | 30 |
| 3 | govulncheck + dependabot (1.5b/c) | 2 | 4 | 1 | 30 |
| 4 | Finish masking consolidation (1.3) | 2 | 3 | 1 | 25 |
| 5 | Blocking lint + .golangci.yml (1.5a) | 3 | 3 | 2 | 24 |
| 6 | go-imap GA watch (1.6) | 2 | 3 | 2 | 20 |
| 7 | Context ownership standardization (June §3.4) | 2 | 2 | 2 | 16 |
| 8 | Shared command lifecycle runner (June §3.2) | 4 | 3 | 4 | 14 |
| 9 | Protocol action test coverage (June §4.2) | 3 | 4 | 4 | 14 |
| 10 | Execution boundary extraction (June §3.3) | 3 | 3 | 4 | 12 |
| 11 | Dead code: IsSMTPRetryableError (1.4) | 1 | 1 | 1 | 10 |
| 12 | Docs/changelog hygiene (1.7) | 1 | 1 | 1 | 10 |
| 13 | Package-tree naming (`internal/smtp/protocol` vs `internal/protocols/smtp`) | 2 | 1 | 3 | 9 |

## 3. Phased plan

### Phase 1 — quick wins (one short session, do before next release)
Items 1–5 + 11. One PR for the retry package (fix classification, honor Retry-After, slog, tests, delete or wire `IsSMTPRetryableError`), one PR for the two masking call sites, one PR for CI (lint config + govulncheck + dependabot). Item 1 is the only *behavioral bug* in this review — it directly affects the tool's headline reliability story.

### Phase 2 — consolidation (next few releases, alongside feature work)
- **Context model** (item 7): root command owns the signal context; actions use `cmd.Context()`; remove the ~30 per-action `SetupSignalContext()` calls. Small, mechanical, do first.
- **Shared command runner** (item 8): `bootstrap.RunAction(cmd, v, configure func(), run func(ctx, ...) error)` encapsulating the 7-step lifecycle. Migrate one protocol per release — smtp first (canonical), then the rest. This is the single biggest drift-risk reducer and makes item 7 trivial to enforce.
- Adopt go-imap v2 GA when released (item 6).

### Phase 3 — long-term architecture (quarter horizon)
- **Execution boundary** (item 10): extract action logic into functions callable from both CLI and `serve` (the serve handlers are the forcing function — today `serve/cmd.go` re-assembles SMTP/Graph configs by hand). Do this *after* the command runner exists, since the runner defines the seam.
- **Test hardening** (item 9): targeted, not coverage-chasing — fake-server tests for one happy path + one failure path per protocol action (net.Pipe/in-process listeners for SMTP/IMAP/POP3; httptest for JMAP/EWS; Kiota mock adapter for msgraph). Prioritize msgraph and the export actions, which have the most logic.
- **Tree naming** (item 13): during boundary extraction, fold wire-level packages (`internal/smtp/protocol` etc.) into `internal/protocols/<p>/wire` — the current `internal/smtp` vs `internal/protocols/smtp` split confuses navigation and is the kind of thing best fixed when the packages are being touched anyway.
- Refresh `ARCHITECTURE_DIAGRAM.md` once Phases 2–3 land (June §Phase 5).

## 4. Summary

The repo remains in good shape: every finding from the June 14 review is fixed or nearly fixed, `go vet` passes, docs are strong, and the June 22 architecture verdict ("consolidation, not reinvention") still stands. The one genuinely new problem this pass found is that **Graph throttling errors bypass the retry machinery entirely** (1.1) — the fix is small, well-localized in the untested `retry` package (1.2), and the two together are the highest-value hour of work available in this codebase today. The long-term direction is unchanged: shared command runner → context cleanup → execution boundary → targeted tests.
