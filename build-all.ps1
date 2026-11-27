#!/usr/bin/env pwsh
# Build script for gomailtesttool
# Builds the unified gomailtest binary (optimized)
#
# To build a protocol-subset binary, pass the -Protocols parameter with a
# space-separated list of protocol tags, e.g.:
#   .\build-all.ps1 -Protocols "smtp imap pop3"
# Valid protocol tags: smtp, imap, pop3, jmap, ews, gmail, msgraph
# The output binary will be named gomailtest-<protocol1>-<protocol2>...exe
#
# Pass -Dmarc to also build the DMARC analyzer (apps/DMARCanalyzer) as
# bin\dmarcanalyzer.exe.
#
# Pass -MailFlow to also build the mail flow probe service (apps/TestMailFlow)
# as bin\testmailflow.exe.
#
# Pass -Warmup to also build the domain warm-up service (apps/warmingup) as
# bin\warmingup.exe.

param(
    [switch]$Verbose,
    [string]$Protocols = "",
    [switch]$Dmarc,
    [switch]$MailFlow,
    [switch]$Warmup
)

$ErrorActionPreference = "Stop"

function Write-ColorOutput {
    param(
        [string]$Message,
        [string]$Color = "White"
    )
    Write-Host $Message -ForegroundColor $Color
}

# Header
Write-ColorOutput "`n═══════════════════════════════════════════════════════════" "Cyan"
Write-ColorOutput "  gomailtesttool Suite - Build Script" "Cyan"
Write-ColorOutput "═══════════════════════════════════════════════════════════`n" "Cyan"

# Ensure bin directory exists
$binDir = Join-Path $PSScriptRoot "bin"
New-Item -ItemType Directory -Path $binDir -Force | Out-Null

$versionFile = Join-Path $PSScriptRoot "internal/common/version/version.go"

# Validate version: on a release branch (b<major>.<minor>.<patch>) raise
# version.go to the branch version. It is never lowered.
$branch = ""
if (Get-Command git -ErrorAction SilentlyContinue) {
    $branch = "$(git -C $PSScriptRoot branch --show-current 2>$null)".Trim()
}
if ($branch -cmatch '^b(\d+\.\d+\.\d+)$') {
    $branchVersion = $Matches[1]
    $source = [System.IO.File]::ReadAllText($versionFile)
    if ($source -notmatch 'const Version = "(\d+\.\d+\.\d+)"') {
        Write-ColorOutput "ERROR: version.go has no semantic version to compare with branch $branch" "Red"
        exit 1
    }
    $fileVersion = $Matches[1]
    if ([version]$branchVersion -gt [version]$fileVersion) {
        $source = $source -replace '(const Version = ")[^"]+(")', "`${1}$branchVersion`${2}"
        [System.IO.File]::WriteAllText($versionFile, $source)
        Write-ColorOutput "  Version raised $fileVersion -> $branchVersion (branch $branch); commit version.go" "Yellow"
        if (-not (Test-Path (Join-Path $PSScriptRoot "ChangeLog/$branchVersion.md"))) {
            Write-ColorOutput "  WARNING: ChangeLog/$branchVersion.md is missing" "Yellow"
        }
    } elseif ([version]$branchVersion -lt [version]$fileVersion) {
        Write-ColorOutput "  WARNING: branch $branch is older than version.go ($fileVersion); not changed" "Yellow"
    }
}

# Read version from version.go
$match = Select-String -Path $versionFile -Pattern 'Version = "([^"]+)"'
if (-not $match) {
    Write-ColorOutput "ERROR: Could not extract version from version.go" "Red"
    exit 1
}
$version = $match.Matches[0].Groups[1].Value

# Determine output file name and build tags
if ($Protocols -ne "") {
    $suffix = ($Protocols.Trim() -split '\s+') -join '-'
    $outputFile = Join-Path $binDir "gomailtest-$suffix.exe"
    $buildTags = "custom $Protocols"
    Write-ColorOutput "  Protocol-subset build: $Protocols" "Yellow"
} else {
    $outputFile = Join-Path $binDir "gomailtest.exe"
    $buildTags = ""
}

# Build gomailtest
$buildArgs = @("-ldflags=-s -w", "-trimpath", "-o", $outputFile)
if ($Verbose) {
    $buildArgs = @("-v") + $buildArgs
}
if ($buildTags -ne "") {
    $buildArgs = @("-tags", $buildTags) + $buildArgs
}
$buildArgs += "./cmd/gomailtest"

& go build @buildArgs

if ($LASTEXITCODE -ne 0) {
    Write-ColorOutput "  ✗ Build failed" "Red"
    exit 1
}

$relPath = $outputFile.Replace($PSScriptRoot + [System.IO.Path]::DirectorySeparatorChar, "")
Write-ColorOutput "  Built $relPath — version $version" "Green"

# Build dmarcanalyzer
if ($Dmarc) {
    $dmarcOut = Join-Path $binDir "dmarcanalyzer.exe"
    $dmarcArgs = @("-ldflags=-s -w", "-trimpath", "-o", $dmarcOut, "./apps/DMARCanalyzer/cmd/dmarcanalyzer")
    if ($Verbose) {
        $dmarcArgs = @("-v") + $dmarcArgs
    }
    & go build @dmarcArgs
    if ($LASTEXITCODE -ne 0) {
        Write-ColorOutput "  ✗ dmarcanalyzer build failed" "Red"
        exit 1
    }
    Write-ColorOutput "  Built bin\dmarcanalyzer.exe — version $version" "Green"
}

# Build testmailflow
if ($MailFlow) {
    $tmfOut = Join-Path $binDir "testmailflow.exe"
    $tmfArgs = @("-ldflags=-s -w", "-trimpath", "-o", $tmfOut, "./apps/TestMailFlow/cmd/testmailflow")
    if ($Verbose) {
        $tmfArgs = @("-v") + $tmfArgs
    }
    & go build @tmfArgs
    if ($LASTEXITCODE -ne 0) {
        Write-ColorOutput "  ✗ testmailflow build failed" "Red"
        exit 1
    }
    Write-ColorOutput "  Built bin\testmailflow.exe — version $version" "Green"
}

# Build warmingup
if ($Warmup) {
    $warmOut = Join-Path $binDir "warmingup.exe"
    $warmArgs = @("-ldflags=-s -w", "-trimpath", "-o", $warmOut, "./apps/warmingup/cmd/warmingup")
    if ($Verbose) {
        $warmArgs = @("-v") + $warmArgs
    }
    & go build @warmArgs
    if ($LASTEXITCODE -ne 0) {
        Write-ColorOutput "  ✗ warmingup build failed" "Red"
        exit 1
    }
    Write-ColorOutput "  Built bin\warmingup.exe — version $version" "Green"
}

# Summary
Write-ColorOutput "`n═══════════════════════════════════════════════════════════" "Cyan"
Write-ColorOutput "  Build Complete!" "Green"
Write-ColorOutput "═══════════════════════════════════════════════════════════`n" "Cyan"
