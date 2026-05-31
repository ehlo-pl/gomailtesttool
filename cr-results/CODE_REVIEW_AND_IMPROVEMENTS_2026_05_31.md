# Code Review — 2026-05-31

## Scope

Branch: `b3.3.6` → `main`
Files changed: 2 (+2 insertions, -19 deletions)
Tool: ultrareview (claude.ai cloud, free tier)

## Changes Reviewed

Pre-existing staticcheck lint fixes on `internal/protocols/smtp/`:

- `testauth.go` — ST1005: lowercase capitalized error string (`errors.New` → `fmt.Errorf`)
- `sendmail.go` — ST1005: lowercase capitalized error string; U1000: removed unused `sanitizeEmailBody` function and orphaned `"errors"` import

## Findings

None. The ultra review returned zero findings.
