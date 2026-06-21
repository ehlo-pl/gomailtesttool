#!/usr/bin/env pwsh
# Build script for gomailtesttool
# Builds the unified gomailtest binary (optimized)

param(
    [switch]$Verbose
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

# Build gomailtest
$outputFile = Join-Path $binDir "gomailtest.exe"

if ($Verbose) {
    go build -v -ldflags="-s -w" -o $outputFile ./cmd/gomailtest
} else {
    go build -ldflags="-s -w" -o $outputFile ./cmd/gomailtest
}

if ($LASTEXITCODE -ne 0) {
    Write-ColorOutput "  ✗ Build failed" "Red"
    exit 1
}

Write-ColorOutput "  Built bin/gomailtest.exe — version $version" "Green"

# Summary
Write-ColorOutput "`n═══════════════════════════════════════════════════════════" "Cyan"
Write-ColorOutput "  Build Complete!" "Green"
Write-ColorOutput "═══════════════════════════════════════════════════════════`n" "Cyan"
