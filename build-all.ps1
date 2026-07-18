#!/usr/bin/env pwsh
# Build script for gomailtesttool
# Builds the unified gomailtest binary (optimized)
#
# To build a protocol-subset binary, pass the -Protocols parameter with a
# space-separated list of protocol tags, e.g.:
#   .\build-all.ps1 -Protocols "smtp imap pop3"
# Valid protocol tags: smtp, imap, pop3, jmap, ews, gmail, msgraph
# The output binary will be named gomailtest-<protocol1>-<protocol2>...exe

param(
    [switch]$Verbose,
    [string]$Protocols = ""
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

# Read version from version.go
$match = Select-String -Path (Join-Path $PSScriptRoot "internal/common/version/version.go") -Pattern 'Version = "([^"]+)"'
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

# Summary
Write-ColorOutput "`n═══════════════════════════════════════════════════════════" "Cyan"
Write-ColorOutput "  Build Complete!" "Green"
Write-ColorOutput "═══════════════════════════════════════════════════════════`n" "Cyan"
