# Interactive Release Script for gomailtesttool
# This script guides you through creating a new release that triggers GitHub Actions
#
# QUICK REFERENCE:
#   Usage:      .\run-interactive-release.ps1
#   Version:    Updates internal/common/version/version.go
#   Changelog:  Creates ChangeLog/{version}.md with interactive prompts
#   Git Tag:    Creates v{version} tag - TRIGGERS GITHUB ACTIONS
#   Output:     Cross-platform suite binaries + GitHub Release archives
#   Auto-Bump:  Bumps patch version and creates b{version} branch for next dev cycle
#
# Full documentation: See RELEASE.md

<#
.SYNOPSIS
    Interactive release script for gomailtesttool

.DESCRIPTION
    This script helps you:
    1. Update version in internal/common/version/version.go
    2. Create/update changelog entry in ChangeLog/{version}.md
    3. Commit changes to git with formatted message
    4. Create and push git tag to trigger GitHub Actions build
    5. Optionally create a Pull Request
    6. Optionally monitor GitHub Actions workflow
    7. Bump patch version and create b{version} branch for next development cycle

    IMPORTANT: Updates the shared version constant used by all tools in the suite.

.EXAMPLE
    .\run-interactive-release.ps1

    Runs the interactive release process

.EXAMPLE
    Get-Help .\release.ps1 -Full

    Shows complete help documentation

.NOTES
    - Version format: x.y.z (Semantic Versioning)
    - Version file: internal/common/version/version.go
    - Requires: Git (required), GitHub CLI (optional for PR creation)
    - Pushing tag triggers: .github/workflows/build.yml
    - Builds: Windows, Linux, and macOS archives for the full suite
    - Documentation: See RELEASE.md for complete documentation
#>

[CmdletBinding()]
param()

# Color functions
function Write-Info { param($Message) Write-Host $Message -ForegroundColor Cyan }
function Write-Success { param($Message) Write-Host "✓ $Message" -ForegroundColor Green }
function Write-Warning { param($Message) Write-Host "⚠ $Message" -ForegroundColor Yellow }
function Write-Error { param($Message) Write-Host "✗ $Message" -ForegroundColor Red }
function Write-Header { param($Message) Write-Host "`n========================================" -ForegroundColor Magenta; Write-Host $Message -ForegroundColor Magenta; Write-Host "========================================`n" -ForegroundColor Magenta }

function Get-VersionFromFile {
    param([string]$Path)

    $content = Get-Content $Path -Raw
    $match = [regex]::Match($content, 'const Version = "([^"]+)"')
    if (-not $match.Success) {
        throw "Could not extract version from $Path"
    }

    return $match.Groups[1].Value
}

function Set-VersionInFile {
    param(
        [string]$Path,
        [string]$NewVersion
    )

    $content = Get-Content $Path -Raw
    if ($content -notmatch 'const Version = "([^"]+)"') {
        throw "Could not find Version constant in $Path"
    }

    $updated = [regex]::Replace($content, 'const Version = "[^"]+"', "const Version = `"$NewVersion`"", 1)
    Set-Content -Path $Path -Value $updated -NoNewline
}

# Check if we're in the right directory
$versionFilePath = Join-Path $PSScriptRoot "internal\common\version\version.go"
if (-not (Test-Path $versionFilePath)) {
    Write-Error "This script must be run from the project root directory"
    Write-Error "Current directory: $(Get-Location)"
    exit 1
}

# Check git status
Write-Header "Step 1: Checking Git Status"
$gitStatus = git status --porcelain
if ($gitStatus) {
    Write-Warning "You have uncommitted changes:"
    git status --short
    $continue = Read-Host "`nDo you want to continue anyway? (y/N)"
    if ($continue -ne 'y' -and $continue -ne 'Y') {
        Write-Info "Release cancelled. Please commit or stash your changes first."
        exit 0
    }
}
Write-Success "Git status checked"

# Security scan for secrets
Write-Header "Step 2: Security Scan for Secrets"
Write-Info "Scanning repository for non-sanitized secrets..."

# Define patterns to search for
$secretPatterns = @{
    "Azure AD Client Secret" = "[a-zA-Z0-9~_-]{34,}"  # Pattern like z3P8Q~...
    "GUID/UUID" = "[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}"
    "Email addresses" = "[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}"
    "API Keys" = "(api[_-]?key|apikey|access[_-]?token|secret[_-]?key)[\s:=]+['`"]?[a-zA-Z0-9_-]{20,}['`"]?"
}

# False positive filtering patterns - extracted for easier maintenance
$placeholderPatterns = @(
    "^x+$"              # All x's (e.g., xxx, xxxx)
    "^y+$"              # All y's (e.g., yyy, yyyy)
    "xxx"               # Contains xxx
    "yyy"               # Contains yyy
    "example\.com"      # Example domain
    "user@example"      # Example user email
    "tenant-guid"       # Placeholder tenant ID
    "client-guid"       # Placeholder client ID
    "your-.*-here"      # Template patterns (e.g., your-secret-here)
    "test-tenant-id"    # Test placeholder
    "test-client-id"    # Test placeholder
)

$knownSafeEmails = @(
    "noreply@anthropic\.com"    # Claude Code attribution
    "example@example\.com"       # Generic example
    "test@example\.com"          # Test example
    "user@example\.com"          # User example
    "admin@example\.com"         # Admin example
)

$knownSafeGUIDs = @(
    "00000000-0000-0000-0000-000000000000"    # Null GUID
    "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"    # Placeholder GUID
    "12345678-1234-1234-1234-123456789012"    # Example GUID (from tests)
    "abcdefgh-5678-9012-abcd-ef1234567890"    # Example GUID (from tests)
)

# Files/paths to scan
$filesToScan = @(
    "test-results/*.md",
    "ChangeLog/*.md",
    "*.md",
    "cmd",
    "internal"
)

# Collect all files to scan (for progress tracking)
Write-Info "Collecting files to scan..."
$allFiles = @()
foreach ($pattern in $filesToScan) {
    $files = Get-ChildItem -Path $pattern -Recurse -File -ErrorAction SilentlyContinue
    foreach ($file in $files) {
        # Skip files that should have example secrets
        if ($file.Name -match "EXAMPLES|README|CLAUDE|IMPROVEMENTS|UNIT_TESTS") {
            continue
        }
        $allFiles += $file
    }
}

$totalFiles = $allFiles.Count
Write-Info "Scanning $totalFiles files for secrets..."

$secretsFound = @()
$fileCount = 0

foreach ($file in $allFiles) {
    $fileCount++

    # Update progress every 5 files or at milestones
    if ($fileCount % 5 -eq 0 -or $fileCount -eq 1 -or $fileCount -eq $totalFiles) {
        $percentComplete = [math]::Round(($fileCount / $totalFiles) * 100, 1)
        Write-Progress -Activity "Security Scan: Checking for secrets" `
                       -Status "Processing file $fileCount of $totalFiles ($percentComplete%)" `
                       -PercentComplete $percentComplete `
                       -CurrentOperation $file.Name
    }

    $content = Get-Content $file.FullName -Raw -ErrorAction SilentlyContinue
    if (-not $content) { continue }

    foreach ($secretType in $secretPatterns.Keys) {
        $regex = $secretPatterns[$secretType]

        if ($content -match $regex) {
            $matches = [regex]::Matches($content, $regex)

            foreach ($match in $matches) {
                $value = $match.Value

                # Skip if it's a placeholder pattern
                $isPlaceholder = $false
                foreach ($pattern in $placeholderPatterns) {
                    if ($value -match $pattern) {
                        $isPlaceholder = $true
                        break
                    }
                }
                if ($isPlaceholder) {
                    continue
                }

                # Skip known safe emails
                if ($secretType -eq "Email addresses") {
                    $isSafeEmail = $false
                    foreach ($safeEmail in $knownSafeEmails) {
                        if ($value -match $safeEmail) {
                            $isSafeEmail = $true
                            break
                        }
                    }
                    if ($isSafeEmail) {
                        continue
                    }
                }

                # Skip known safe GUIDs
                if ($secretType -eq "GUID/UUID") {
                    $isSafeGUID = $false
                    foreach ($safeGUID in $knownSafeGUIDs) {
                        if ($value -eq $safeGUID) {
                            $isSafeGUID = $true
                            break
                        }
                    }
                    if ($isSafeGUID) {
                        continue
                    }
                }

                # Get line number
                $lineNumber = ($content.Substring(0, $match.Index) -split "`n").Count

                $secretsFound += [PSCustomObject]@{
                    File = $file.FullName.Replace((Get-Location).Path + "\", "")
                    Line = $lineNumber
                    Type = $secretType
                    Value = if ($value.Length -gt 40) { $value.Substring(0, 40) + "..." } else { $value }
                }
            }
        }
    }
}

# Complete the progress indicator
Write-Progress -Activity "Security Scan: Checking for secrets" -Completed

if ($secretsFound.Count -gt 0) {
    Write-Error "Potential secrets detected in repository!"
    Write-Host "`nFound $($secretsFound.Count) potential secret(s):" -ForegroundColor Yellow

    $secretsFound | Format-Table -AutoSize

    Write-Warning "`nThese may be sensitive credentials that should be sanitized before release."
    Write-Info "Common fixes:"
    Write-Host "  • Replace real GUIDs with: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
    Write-Host "  • Replace real emails with: user@example.com"
    Write-Host "  • Replace secrets with: xxx~xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
    Write-Host "  • Review test-results/*.md files for real credentials"

    $continue = Read-Host "`nDo you want to continue anyway? (y/N)"
    if ($continue -ne 'y' -and $continue -ne 'Y') {
        Write-Error "Release cancelled due to security concerns"
        Write-Info "Please sanitize the detected secrets and try again"
        exit 1
    }

    Write-Warning "Proceeding with release despite security warnings..."
} else {
    Write-Success "No unsanitized secrets detected"
}

# Read current version
Write-Header "Step 3: Version Information"
$currentVersion = Get-VersionFromFile $versionFilePath
Write-Info "Current version: $currentVersion"

# Validate current version format (must be x.y.z)
if ($currentVersion -notmatch '^\d+\.\d+\.\d+$') {
    Write-Error "Current version '$currentVersion' is invalid. Must be in format x.y.z"
    exit 1
}

# Parse current version
$versionParts = $currentVersion -split '\.'
$currentMajor = [int]$versionParts[0]
$currentMinor = [int]$versionParts[1]
$currentPatch = [int]$versionParts[2]

# Suggest next versions
Write-Info "`nSuggested versions:"
Write-Host "  1. Patch release (bug fixes):    $currentMajor.$currentMinor.$($currentPatch + 1)"
Write-Host "  2. Minor release (new features): $currentMajor.$($currentMinor + 1).0"
Write-Host "  3. Major release (breaking):     $($currentMajor + 1).0.0"

# Prompt for new version
$newVersion = $null
while ($true) {
    $input = Read-Host "`nEnter new version (must be x.y.z format, or press Enter for patch: $currentMajor.$currentMinor.$($currentPatch + 1))"

    if ([string]::IsNullOrWhiteSpace($input)) {
        # Default to patch version
        $newVersion = "$currentMajor.$currentMinor.$($currentPatch + 1)"
        break
    }

    # Validate format
    if ($input -notmatch '^\d+\.\d+\.\d+$') {
        Write-Warning "Invalid format. Must be x.y.z"
        continue
    }

    # Check if version is greater than current
    $newParts = $input -split '\.'
    $newMajor = [int]$newParts[0]
    $newMinor = [int]$newParts[1]
    $newPatch = [int]$newParts[2]

    if ([version]$input -le [version]$currentVersion) {
        Write-Warning "New version must be greater than current version ($currentVersion)"
        continue
    }

    $newVersion = $input
    break
}

$newParts = $newVersion -split '\.'
$newMajor = [int]$newParts[0]
$newMinor = [int]$newParts[1]
$newPatch = [int]$newParts[2]

Write-Success "New version: $newVersion"

# Determine release type
$releaseType = if ($newMajor -gt $currentMajor) { "Major" } elseif ($newMinor -gt $currentMinor) { "Minor" } else { "Patch" }
Write-Info "Release type: $releaseType"

# Update version file
Write-Header "Step 4: Updating Version File"
Write-Info "Updating $versionFilePath..."
$versionFileOriginalContent = Get-Content $versionFilePath -Raw
Set-VersionInFile -Path $versionFilePath -NewVersion $newVersion
Write-Success "Updated version to $newVersion"

Write-Info "`nNote: Version is read from the shared const in internal/common/version/version.go"

# Create/update changelog entry
Write-Header "Step 5: Changelog Entry"
$changelogPath = "CHANGELOG.md"
$changelogEntryPath = "ChangeLog\$newVersion.md"

# Create ChangeLog directory if it doesn't exist
if (-not (Test-Path "ChangeLog")) {
    New-Item -ItemType Directory -Path "ChangeLog" | Out-Null
    Write-Info "Created ChangeLog directory"
}

# Check if changelog entry already exists
if (Test-Path $changelogEntryPath) {
    Write-Info "Changelog entry already exists at $changelogEntryPath"
    $existingChangelog = Get-Content $changelogEntryPath -Raw
    Write-Host "`nCurrent changelog entry:"
    Write-Host "------------------------"
    Write-Host $existingChangelog
    Write-Host "------------------------"

    $editChangelog = Read-Host "`nDo you want to edit this changelog entry? (y/N)"
    if ($editChangelog -eq 'y' -or $editChangelog -eq 'Y') {
        notepad.exe $changelogEntryPath
        Write-Info "Waiting for notepad to close..."
        Start-Sleep -Seconds 1
    }
} else {
    Write-Info "Creating new changelog entry at $changelogEntryPath"

    # Prompt for changelog sections
    Write-Info "`nEnter changelog entries (press Enter on empty line to finish each section)"

    # Added
    Write-Host "`n[Added] New features:"
    $added = @()
    while ($true) {
        $line = Read-Host "  -"
        if ([string]::IsNullOrWhiteSpace($line)) { break }
        $added += "- $line"
    }

    # Changed
    Write-Host "`n[Changed] Changes to existing functionality:"
    $changed = @()
    while ($true) {
        $line = Read-Host "  -"
        if ([string]::IsNullOrWhiteSpace($line)) { break }
        $changed += "- $line"
    }

    # Fixed
    Write-Host "`n[Fixed] Bug fixes:"
    $fixed = @()
    while ($true) {
        $line = Read-Host "  -"
        if ([string]::IsNullOrWhiteSpace($line)) { break }
        $fixed += "- $line"
    }

    # Security
    Write-Host "`n[Security] Security improvements:"
    $security = @()
    while ($true) {
        $line = Read-Host "  -"
        if ([string]::IsNullOrWhiteSpace($line)) { break }
        $security += "- $line"
    }

    # Build changelog content
    $changelogContent = @"
## [$newVersion] $(Get-Date -Format 'yyyy-MM-dd')

"@

    if ($added.Count -gt 0) {
        $changelogContent += "`n### Added`n`n"
        $changelogContent += ($added -join "`n") + "`n"
    }

    if ($changed.Count -gt 0) {
        $changelogContent += "`n### Changed`n`n"
        $changelogContent += ($changed -join "`n") + "`n"
    }

    if ($fixed.Count -gt 0) {
        $changelogContent += "`n### Fixed`n`n"
        $changelogContent += ($fixed -join "`n") + "`n"
    }

    if ($security.Count -gt 0) {
        $changelogContent += "`n### Security`n`n"
        $changelogContent += ($security -join "`n") + "`n"
    }

    # If no entries, add a default message
    if ($added.Count -eq 0 -and $changed.Count -eq 0 -and $fixed.Count -eq 0 -and $security.Count -eq 0) {
        $changelogContent += "`n### Changed`n`n- Updated default action from getevents to getinbox`n"
    }

    # Save changelog entry
    Set-Content -Path $changelogEntryPath -Value $changelogContent
    Write-Success "Created $changelogEntryPath"

    Write-Host "`nChangelog entry:"
    Write-Host "------------------------"
    Write-Host $changelogContent
    Write-Host "------------------------"
}

# Read final changelog for commit message
$changelogForCommit = Get-Content $changelogEntryPath -Raw

# Build commit message
$commitMessage = @"
Release v$newVersion - $releaseType Release

$changelogForCommit

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>
"@

# Show summary
Write-Header "Step 6: Review Changes"
Write-Info "The following changes will be made:"
Write-Host "  • internal/common/version/version.go: $currentVersion → $newVersion"
Write-Host "  • ${changelogEntryPath}: $(if (Test-Path $changelogEntryPath) { "Updated" } else { "Created" })"
Write-Host "`nCommit message:"
Write-Host "------------------------"
Write-Host $commitMessage
Write-Host "------------------------"

# Confirm
$confirm = Read-Host "`nDo you want to proceed with these changes? (y/N)"
if ($confirm -ne 'y' -and $confirm -ne 'Y') {
    Write-Warning "Release cancelled"
    # Restore version file
    Set-Content -Path $versionFilePath -Value $versionFileOriginalContent -NoNewline
    Write-Info "Restored $versionFilePath to $currentVersion"
    exit 0
}

# Stage changes
Write-Header "Step 7: Committing Changes"
Write-Info "Staging changes..."
git add $versionFilePath $changelogEntryPath

# Show what will be committed
Write-Info "`nFiles to be committed:"
git status --short

# Commit
Write-Info "`nCreating commit..."
git commit -m $commitMessage

if ($LASTEXITCODE -ne 0) {
    Write-Error "Git commit failed"
    exit 1
}
Write-Success "Committed changes"

# Get current branch
$currentBranch = git rev-parse --abbrev-ref HEAD
Write-Info "Current branch: $currentBranch"

# Ask about pushing
Write-Header "Step 8: Push to Remote"
$pushBranch = Read-Host "Push commit to remote branch '$currentBranch'? (Y/n)"
if ($pushBranch -ne 'n' -and $pushBranch -ne 'N') {
    Write-Info "Pushing to origin/$currentBranch..."
    git push origin $currentBranch

    if ($LASTEXITCODE -ne 0) {
        Write-Error "Git push failed"
        Write-Warning "You may need to push manually: git push origin $currentBranch"
    } else {
        Write-Success "Pushed to origin/$currentBranch"
    }
}

# Create and push tag (THIS TRIGGERS GITHUB ACTIONS)
Write-Header "Step 9: Create Git Tag (Triggers GitHub Actions)"
$tagName = "v$newVersion"
Write-Warning "Creating and pushing tag '$tagName' will trigger GitHub Actions workflow!"
Write-Info "This will:"
Write-Host "  • Build Windows, Linux, and macOS archives"
Write-Host "  • Create GitHub Release"
Write-Host "  • Attach suite ZIP files with binaries and docs to the release"

$createTag = Read-Host "`nCreate and push tag '$tagName'? (y/N)"
if ($createTag -eq 'y' -or $createTag -eq 'Y') {
    Write-Info "Creating tag $tagName..."
    git tag $tagName

    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to create tag"
        exit 1
    }

    Write-Info "Pushing tag $tagName..."
    git push origin $tagName

    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to push tag"
        Write-Warning "You may need to push manually: git push origin $tagName"
    } else {
        Write-Success "Tag pushed successfully"
        Write-Success "GitHub Actions workflow should start building now!"
    }
} else {
    Write-Warning "Tag not created. You can create it manually later with:"
    Write-Host "  git tag $tagName"
    Write-Host "  git push origin $tagName"
}

# Optionally create PR
Write-Header "Step 10: Pull Request (Optional)"
if ($currentBranch -ne 'main') {
    $createPR = Read-Host "Create Pull Request to merge '$currentBranch' into 'main'? (y/N)"

    if ($createPR -eq 'y' -or $createPR -eq 'Y') {
        # Check if gh CLI is installed
        $ghInstalled = Get-Command gh -ErrorAction SilentlyContinue

        if (-not $ghInstalled) {
            Write-Warning "GitHub CLI (gh) is not installed"
            Write-Info "Install it from: https://cli.github.com/"
            Write-Info "Or create PR manually at: https://github.com/your-repo/pulls"
        } else {
            Write-Info "Creating Pull Request..."

            $prBody = @"
## Summary
$releaseType release v$newVersion

## Changes
$changelogForCommit

## Test plan
- [x] Version updated in all required files
- [x] Shared version source updated
- [x] Changelog entry created
- [x] Changes committed to branch

🤖 Generated with [Claude Code](https://claude.com/claude-code)
"@

            gh pr create --title "Release v$newVersion" --body $prBody

            if ($LASTEXITCODE -eq 0) {
                Write-Success "Pull Request created"

                $mergePR = Read-Host "`nMerge PR immediately? (y/N)"
                if ($mergePR -eq 'y' -or $mergePR -eq 'Y') {
                    gh pr merge --delete-branch --squash
                    if ($LASTEXITCODE -eq 0) {
                        Write-Success "PR merged and branch deleted"
                    }
                }
            } else {
                Write-Warning "Failed to create PR. You can create it manually."
            }
        }
    }
} else {
    Write-Info "Already on main branch, skipping PR creation"
}

# Monitor GitHub Actions (optional)
Write-Header "Step 11: Monitor GitHub Actions (Optional)"
$monitor = Read-Host "Monitor GitHub Actions workflow? (y/N)"
if ($monitor -eq 'y' -or $monitor -eq 'Y') {
    $ghInstalled = Get-Command gh -ErrorAction SilentlyContinue

    if (-not $ghInstalled) {
        Write-Warning "GitHub CLI (gh) is not installed"
        Write-Info "View workflow at: https://github.com/your-repo/actions"
    } else {
        Write-Info "Recent workflow runs:"
        gh run list --limit 5

        Write-Info "`nTo watch the current run:"
        Write-Host "  gh run watch"
    }
}

# Final summary
Write-Header "Release Summary"
Write-Success "Release v$newVersion completed!"
Write-Info "`nWhat happened:"
Write-Host "  ✓ Version updated: $currentVersion → $newVersion"
Write-Host "  ✓ Changelog created/updated: $changelogEntryPath"
Write-Host "  ✓ Changes committed to git"
if ($pushBranch -ne 'n' -and $pushBranch -ne 'N') {
    Write-Host "  ✓ Pushed to origin/$currentBranch"
}
if ($createTag -eq 'y' -or $createTag -eq 'Y') {
    Write-Host "  ✓ Tag $tagName created and pushed (GitHub Actions triggered)"
}

Write-Info "`nNext steps:"
if ($createTag -eq 'y' -or $createTag -eq 'Y') {
    Write-Host "  1. Monitor GitHub Actions: gh run watch"
    Write-Host "  2. Verify release created: https://github.com/your-repo/releases"
    Write-Host "  3. Test downloaded binaries from release"
} else {
    Write-Host "  1. Push tag to trigger build: git push origin $tagName"
    Write-Host "  2. Monitor GitHub Actions: gh run watch"
}

Write-Success "`nRelease process complete!"

# Bump version for next development cycle
Write-Header "Step 12: Prepare Next Development Cycle"
$bumpVersion = Read-Host "Bump patch version and create new development branch? (Y/n)"
if ($bumpVersion -ne 'n' -and $bumpVersion -ne 'N') {
    # Parse current (released) version
    $releasedParts = $newVersion -split '\.'
    $nextMajor = [int]$releasedParts[0]
    $nextMinor = [int]$releasedParts[1]
    $nextPatch = [int]$releasedParts[2] + 1

    $nextVersion = "$nextMajor.$nextMinor.$nextPatch"
    $branchName = "b$nextVersion"

    Write-Info "Next development version: $nextVersion"
    Write-Info "Branch name: $branchName"

    # Update version file
    Write-Info "`nUpdating $versionFilePath to $nextVersion..."
    Set-VersionInFile -Path $versionFilePath -NewVersion $nextVersion
    Write-Success "Updated version to $nextVersion"

    # Create new branch
    Write-Info "`nCreating branch $branchName..."
    git checkout -b $branchName

    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to create branch $branchName"
        # Restore version file
        Set-VersionInFile -Path $versionFilePath -NewVersion $newVersion
        Write-Warning "Restored version to $newVersion"
    } else {
        Write-Success "Created and checked out branch: $branchName"

        # Commit the version bump
        Write-Info "`nCommitting version bump..."
        git add $versionFilePath
        git commit -m "Bump version to $nextVersion for next development cycle

Preparing for next patch release after v$newVersion

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"

        if ($LASTEXITCODE -eq 0) {
            Write-Success "Version bump committed"

            # Ask about pushing the new branch
            $pushNewBranch = Read-Host "`nPush new branch '$branchName' to remote? (Y/n)"
            if ($pushNewBranch -ne 'n' -and $pushNewBranch -ne 'N') {
                Write-Info "Pushing $branchName to origin..."
                git push -u origin $branchName

                if ($LASTEXITCODE -eq 0) {
                    Write-Success "Branch pushed to origin/$branchName"
                } else {
                    Write-Warning "Failed to push branch. You can push manually with: git push -u origin $branchName"
                }
            }
        } else {
            Write-Error "Failed to commit version bump"
        }

        # Summary of next dev cycle
        Write-Info "`nNext development cycle ready:"
        Write-Host "  ✓ Current branch: $branchName"
        Write-Host "  ✓ Version in internal/common/version/version.go: $nextVersion"
        Write-Host "  ✓ Ready for development work"
    }
} else {
    Write-Info "Skipped version bump. Current version remains: $newVersion"
}
