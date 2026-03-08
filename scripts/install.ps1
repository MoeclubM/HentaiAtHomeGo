[CmdletBinding()]
param(
    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA "HentaiAtHomeGo"),
    [string]$BinaryName = "hathgo.exe",
    [switch]$Force
)

$ErrorActionPreference = "Stop"

$ScriptDir = $PSScriptRoot
$RepoRoot = Split-Path -Parent $ScriptDir

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

$BinaryPath = Join-Path $InstallDir $BinaryName
$WrapperPs1 = Join-Path $InstallDir "run-hathgo.ps1"
$WrapperCmd = Join-Path $InstallDir "run-hathgo.cmd"

if ((Test-Path $BinaryPath) -and -not $Force) {
    throw "Binary already exists at $BinaryPath . Use -Force to overwrite."
}

$SourceTree = (Test-Path (Join-Path $RepoRoot "cmd\client")) -and (Test-Path (Join-Path $RepoRoot "internal"))

if ($SourceTree) {
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
        throw "Go is required in source tree mode but was not found in PATH."
    }

    Write-Host "Building client into $BinaryPath"
    Push-Location $RepoRoot
    try {
        & go build -trimpath -o $BinaryPath .\cmd\client
        if ($LASTEXITCODE -ne 0) {
            throw "go build failed with exit code $LASTEXITCODE"
        }
    }
    finally {
        Pop-Location
    }
}
else {
    $Candidates = @(
        (Join-Path $ScriptDir $BinaryName),
        (Join-Path $ScriptDir "hathgo.exe"),
        (Join-Path $ScriptDir "hathgo")
    ) | Select-Object -Unique

    $BundledBinary = $Candidates | Where-Object { Test-Path $_ } | Select-Object -First 1
    if (-not $BundledBinary) {
        throw "No source tree or bundled binary found next to install.ps1."
    }

    Write-Host "Installing bundled binary from $BundledBinary to $BinaryPath"
    $BundledBinaryFull = [System.IO.Path]::GetFullPath($BundledBinary)
    $BinaryPathFull = [System.IO.Path]::GetFullPath($BinaryPath)
    if ($BundledBinaryFull -ine $BinaryPathFull) {
        Copy-Item -Force $BundledBinary $BinaryPath
    }
}

foreach ($dir in @("data", "log", "cache", "tmp", "download", "certs")) {
    New-Item -ItemType Directory -Force -Path (Join-Path $InstallDir $dir) | Out-Null
}

$wrapperPs1Content = @"
param(
    [Parameter(ValueFromRemainingArguments = `$true)]
    [string[]]`$ArgsFromCaller
)

& "$BinaryPath" `
  --data-dir="$InstallDir\data" `
  --log-dir="$InstallDir\log" `
  --cache-dir="$InstallDir\cache" `
  --temp-dir="$InstallDir\tmp" `
  --download-dir="$InstallDir\download" `
  @ArgsFromCaller
"@

$wrapperCmdContent = @"
@echo off
powershell.exe -ExecutionPolicy Bypass -File "$WrapperPs1" %*
"@

Set-Content -Path $WrapperPs1 -Value $wrapperPs1Content -Encoding UTF8
Set-Content -Path $WrapperCmd -Value $wrapperCmdContent -Encoding ASCII

Write-Host "Install complete."
Write-Host ""
Write-Host "Binary:"
Write-Host "  $BinaryPath"
Write-Host ""
Write-Host "Launchers:"
Write-Host "  $WrapperPs1"
Write-Host "  $WrapperCmd"
