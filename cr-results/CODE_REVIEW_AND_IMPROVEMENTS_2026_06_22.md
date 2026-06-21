# Code Review & Improvement Plan: gomailtesttool
**Date:** 2026-06-22
**Version Reviewed:** Current `main` / tag series up to `v2.6.15`
**Reviewer:** GitHub Copilot CLI (rubber-duck synthesis)

## 1. Executive Summary

The codebase still has a coherent product idea: a portable Go-based toolkit for testing and operating across multiple mail protocols. The main issue is no longer feature capability, but **consistency drift**. The repository has evolved from a 2-tool project into a broader mail suite, while versioning, naming, documentation, and some shared implementation seams still reflect the older shape.

**Overall assessment:**
* **Strength:** The suite direction is good. The repository now covers Graph, SMTP, IMAP, POP3, and JMAP with a shared Go module and cross-platform build pipeline.
* **Primary concern:** The project identity and internal contracts are not yet standardized around that broader scope.
* **Recommended direction:** Treat this as a **mail tools suite** first, and make naming, release/versioning, documentation, and shared protocol utilities reflect that explicitly.

---

## 2. Consistency Review

### 2.1 Product Identity Drift

There is a visible mismatch between the current repository shape and the language used in core documentation:

* `AGENTS.md` still says the repository contains **two complementary tools** and points to `https://github.com/ziembor/msgraphtool`.
* `README.md` also opens with the same 2-tool framing and the same repository URL.
* The actual repository now ships **five tools** under `cmd/`: `msgraphtool`, `smtptool`, `imaptool`, `pop3tool`, and `jmaptool`.
* The GitHub Actions release workflow already packages the project as **`gomailtesttool`** and produces archives such as `gomailtesttool-linux-amd64.zip`.

**Why it matters:** new contributors, future automation, and AI-assisted changes start from the wrong mental model. The implementation already behaves like a suite, but the docs still describe a smaller product.

**Recommendation:** standardize the public identity as `gomailtesttool` (suite) containing several focused binaries. Keep individual tool names for executables and tool-specific READMEs.

### 2.2 Module Name vs Repository Name

The root `go.mod` still declares:

```go
module msgraphtool
```

That conflicts with the repository and release identity, which now clearly lean toward `gomailtesttool`.

**Risk:** even if this does not immediately break local builds, it creates long-term confusion in imports, docs, release assets, and future refactoring. It also biases the codebase toward one tool name even though the repo now spans multiple protocols.

**Recommendation:** decide whether `msgraphtool` is still intended as the permanent module identity. If not, do a deliberate one-time rename to match the suite. If yes, document the asymmetry clearly so future contributors do not create mixed naming conventions.

### 2.3 Versioning Contract Is Not Holding

`internal/common/version/version.go` still defines:

```go
const Version = "2.6.10"
```

but the repository already has newer tags through `v2.6.15`.

This directly breaks the stated "single source of truth" model documented in `AGENTS.md` and `RELEASE.md`.

**Why it matters:** binaries built from current `main` can report a version that no longer matches the release history, which makes troubleshooting and release validation harder.

**Recommendation:** fix the current version immediately and add a CI check that compares the constant to the tag during release workflows.

### 2.4 Release Documentation Uses Inconsistent Changelog Path Casing

The repository contains `ChangeLog/`, but `RELEASE.md` repeatedly references `Changelog/`.

**Why it matters:** this is harmless on case-insensitive filesystems, but it is a real maintenance risk on Linux runners and for scripted release steps. A future manual or automated release could create the wrong directory or stage the wrong path.

**Recommendation:** choose one casing and update all docs and scripts to match. Lowercase-only paths are the safest long-term convention, but consistency matters more than the specific choice.

---

## 3. Architecture Review

### 3.1 The High-Level Direction Is Good

The current build workflow and `cmd/` layout show a clear evolution:

* thin per-tool command folders
* shared internal packages for common concerns
* cross-platform builds
* one release artifact family for the suite

This is the right direction. The project should continue as a **small family of focused CLIs** rather than forcing everything into one multi-mode executable.

### 3.2 Shared Seams Need Stronger Enforcement

There is already reusable TLS version parsing in `internal/smtp/tls/validation.go`:

* `ParseTLSVersion`
* `TLSVersionString`

However, both:

* `cmd/imaptool/imap_client.go`
* `cmd/pop3tool/pop3_client.go`

define their own local `parseTLSVersion()` with the same mapping logic.

**Why it matters:** this is exactly the kind of small duplication that turns into drift later. Once protocol tools multiply, utility behavior must come from one place.

**Recommendation:** move this logic behind a protocol-neutral shared package if needed, or reuse the existing exported helper directly. Shared TLS behavior should be standardized across SMTP, IMAP, and POP3.

### 3.3 Documented Structure Does Not Fully Match Actual Structure

`AGENTS.md` documents `internal/msgraph/`, but the actual implementation emphasis is much more `cmd/`-centric for Graph, while SMTP has deeper reusable internal packages.

This is not automatically wrong. The Graph tool may legitimately remain thinner because the Microsoft Graph SDK already provides a lot of transport and object structure. But the asymmetry should be intentional and documented.

**Recommendation:** either:

1. move more Graph-specific reusable logic into `internal/msgraph/`, or
2. update the docs to say the Graph tool is intentionally more self-contained in `cmd/msgraphtool/`.

The important part is to avoid a mismatch between the architecture described and the architecture contributors will actually find.

### 3.4 Legacy `src/` Is Still a Source of Ambiguity

The repository still contains a legacy `src/` tree with its own `go.mod` and its own version file (`2.6.9`).

**Why it matters:**
* it creates a second, stale version source in practice
* it weakens the "single source of truth" story
* it increases the chance that future changes are made in the wrong location
* release documentation still references `src/go.mod`

**Recommendation:** either remove `src/` entirely or quarantine it clearly as legacy-only and stop referencing it from current release documentation.

---

## 4. Pragmatic Development Direction

### 4.1 What to Standardize Next

The next cleanup phase should standardize the following in this order:

1. **Versioning and release truth**
   * make `internal/common/version/version.go` accurate
   * align changelog path casing
   * remove stale release references to `src/`

2. **Repository identity**
   * update `README.md` and `AGENTS.md` to describe the current 5-tool suite
   * decide whether the canonical name is `gomailtesttool` or `msgraphtool`

3. **Shared protocol utilities**
   * centralize TLS version parsing/formatting
   * keep protocol-agnostic helpers outside per-tool command packages

4. **Architecture documentation**
   * describe which packages are intentionally shared
   * describe where per-tool divergence is expected

### 4.2 Seams Worth Preserving

These are the good boundaries to preserve as the suite grows:

* **`cmd/<tool>` as thin CLI orchestration**
* **`internal/common` for cross-cutting concerns** such as versioning, logging, retry, validation
* **protocol-specific internal packages** where transport rules or parsers differ materially
* **separate binaries** instead of overloading one executable with all protocol behaviors

This keeps the suite understandable and avoids turning one tool into a god application.

### 4.3 Where Divergence Is Acceptable

Not every tool needs identical internal depth.

Acceptable divergence:
* Graph may remain more SDK-driven than SMTP/IMAP/POP3.
* Protocol-specific capabilities can keep their own request/response handling.
* Different tools may expose different action shapes if the protocol semantics require it.

Non-acceptable divergence:
* inconsistent version reporting
* inconsistent naming and repo identity
* duplicated shared TLS/config parsing behavior
* docs that no longer describe the actual product

---

## 5. Recommended Improvement Plan

### Phase 1: Consistency Baseline (High Priority)
* Update `internal/common/version/version.go` to the current release line.
* Fix `README.md` and `AGENTS.md` to reflect the 5-tool suite and the correct repository URL.
* Standardize `ChangeLog/` vs `Changelog/`.
* Remove `src/go.mod` references from `RELEASE.md`.

### Phase 2: Shared Utility Cleanup (Medium Priority)
* Replace duplicated `parseTLSVersion()` functions in IMAP and POP3 with a shared helper.
* Review other recent tools for copy-pasted validation or TLS logic before it spreads further.

### Phase 3: Identity and Structure Decision (Medium Priority)
* Decide whether the canonical long-term project name is `gomailtesttool` or `msgraphtool`.
* If the suite identity wins, align `go.mod`, imports, docs, and release language in one controlled change.

### Phase 4: Legacy Removal (Low/Medium Priority)
* Remove or archive the legacy `src/` module once nothing depends on it.
* Keep only one active code path for current releases.

---

## 6. Final Recommendation

The project should continue developing as a **portable mail operations and diagnostics suite**, not as a Graph-first tool with add-ons. The implementation is already heading there. The next work should not primarily be new features; it should be **consistency work** that locks the current direction in place:

* one identity
* one version truth
* one release story
* one shared implementation approach for common protocol utilities

Once that baseline is in place, adding more protocol features or deeper diagnostics will be much safer and much easier to maintain.
