# Release & Versioning Guide

This document is the **definitive guide** for versioning and releasing the `gomailtesttool` suite.

## 1. Versioning Policy

### Single Source of Truth
The version is stored in **one place only**:
- File: `internal/common/version/version.go`
- Format: Go const string (e.g., `const Version = "2.0.2"`)

To update the version, edit the `Version` constant in `internal/common/version/version.go`. No external VERSION files are needed.

### Version Numbering
- **Format:** `x.y.z` (Semantic Versioning)
  - `x` (Major): Breaking changes, major architectural shifts, new tools/executables.
  - `y` (Minor): New features, significant enhancements.
  - `z` (Patch): Bug fixes, documentation updates.

### Changelog Format
Changelogs are stored as individual files in the `ChangeLog/` directory:
- **Location:** `ChangeLog/{version}.md` (e.g., `ChangeLog/1.16.2.md`)
- **Format:** [Keep a Changelog](https://keepachangelog.com) style.

## 2. Release Process

Use the manual release flow below. GitHub Actions handles the cross-platform build and release packaging after the version commit and tag are pushed.

### Step 1: Update Version
Update the version constant with the new number (e.g., `2.1.0`).

Edit `internal/common/version/version.go`:
```go
const Version = "2.1.0"  // Change this line
```

### Step 2: Create Changelog
Create a new file `ChangeLog/2.1.0.md`:
```markdown
## [2.1.0] - 2026-01-05

### Added
- New feature X

### Fixed
- Bug fix Y
```

### Step 3: Verify Build (Optional)
```powershell
.\build-all.ps1
.\msgraphtool.exe -version
# Should output: ... Version 2.1.0
.\smtptool.exe -version
# Should output: ... Version 2.1.0
```

### Step 4: Commit Changes
```powershell
git add internal/common/version/version.go ChangeLog/2.1.0.md
git commit -m "Release v2.1.0"
git push origin main
```

### Step 5: Create and Push Tag
**This is the trigger for the automated build pipeline.**
```powershell
git tag v2.1.0
git push origin v2.1.0
```

## 3. GitHub Actions Workflow

When a tag matching `v*` is pushed (e.g., `v2.1.0`), the `.github/workflows/build.yml` workflow triggers automatically.

**The Workflow:**
1. **Builds** the application for Windows, Linux, and macOS.
2. **Creates** a GitHub Release with the tag name.
3. **Packages** the binary and documentation (README, LICENSE) into ZIP files.
4. **Uploads** the ZIP files to the Release.

**Artifacts Produced:**
- `gomailtesttool-windows-amd64.zip`
- `gomailtesttool-linux-amd64.zip`
- `gomailtesttool-macos-arm64.zip`

## 4. Troubleshooting & Rollback

### Build Fails in GitHub Actions
- Check the workflow logs: `gh run view --log-failed`
- Ensure `go.mod` and `build.yml` both target the same Go version.

### Emergency Rollback
If a bad release is pushed:

1. **Delete Tag:**
   ```powershell
   git tag -d v1.16.2
   git push origin :refs/tags/v1.16.2
   ```
2. **Delete Release:**
   ```powershell
   gh release delete v1.16.2 --yes
   ```
3. **Revert Commit:**
   ```powershell
   git revert HEAD
   git push origin main
   ```

## For Future AI Assistants

To perform a release:
1. Read `internal/common/version/version.go` to get the current version.
2. Increment according to Semantic Versioning (major.minor.patch).
3. Update the `Version` const in `internal/common/version/version.go`.
4. Write `ChangeLog/{version}.md`.
5. Commit and tag the release.