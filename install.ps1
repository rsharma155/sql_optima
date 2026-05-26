# SQL Optima - one-command install from anywhere (clone if needed, then start dev stack).
# Works on Windows PowerShell 5.1+ and PowerShell Core on Linux/macOS (pwsh).
#
# Usage:
#   PowerShell -ExecutionPolicy Bypass -File .\install.ps1
#   pwsh ./install.ps1
#   irm https://raw.githubusercontent.com/rsharma155/sql_optima/main/install.ps1 | iex
#
# On Linux/macOS without PowerShell, use install.sh instead.
param(
    [string]$Dir = '',
    [Alias('d')]
    [string]$Directory = '',
    [switch]$NoBrowser,
    [switch]$NoClone,
    [switch]$Help
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass -Force

function Show-InstallHelp {
    @'
SQL Optima - local dev install (PowerShell)

Usage: install.ps1 [options]

Options:
  -Dir PATH          Use or clone into PATH (default: ./sql_optima or SQL_OPTIMA_DIR)
  -NoBrowser         Do not open a browser when the API is ready
  -NoClone           Require current directory to be the repo root (no git clone)
  -Help              Show this help

Environment:
  SQL_OPTIMA_DIR              Target repo directory
  SQL_OPTIMA_REPO_URL         Git clone URL (default: GitHub sql_optima)
  SQL_OPTIMA_NO_BROWSER=1     Same as -NoBrowser
  SQL_OPTIMA_SETUP_MODE       easy|custom (non-interactive; default: easy)
  SQL_OPTIMA_EXPOSE_DB_LAN    0|1 - publish TimescaleDB for LAN/DBeaver (custom only)
  SQL_OPTIMA_DB_USER          Metrics DB user (default: dbmonitor)
  SQL_OPTIMA_DB_PASSWORD      Metrics DB password
  SQL_OPTIMA_DB_NAME          Metrics DB name (default: dbmonitor_metrics)
  SQL_OPTIMA_API_PORT         Web UI port (default: 8080)
  SQL_OPTIMA_DB_PUBLISH_PORT  Host port when LAN enabled (default: 5432)
  SQL_OPTIMA_DB_PUBLISH_BIND  LAN IP to bind (auto-detected if unset)

First-time setup (interactive when run in a terminal):
  1) Easy (default) - DB not exposed on LAN; safest for curl|bash style runs
  2) Custom - choose credentials and optionally enable LAN/DBeaver

See docs/QUICKSTART.md for details.
'@ | Write-Host
}

function Test-RepoRoot([string]$Path) {
    $goMod = Join-Path (Join-Path $Path 'backend') 'go.mod'
    return (Test-Path -LiteralPath $goMod)
}

function Test-NonInteractiveInstall {
    try {
        if ([Console]::IsInputRedirected) { return $true }
    } catch {
        return $true
    }
    if ($env:SQL_OPTIMA_SETUP_MODE) { return $true }
    return $false
}

if ($Help) {
    Show-InstallHelp
    exit 0
}

if ($Directory -and -not $Dir) { $Dir = $Directory }

$RepoUrl = if ($env:SQL_OPTIMA_REPO_URL) { $env:SQL_OPTIMA_REPO_URL } else { 'https://github.com/rsharma155/sql_optima.git' }
if ($env:SQL_OPTIMA_DIR -and -not $Dir) { $Dir = $env:SQL_OPTIMA_DIR }
if ($env:SQL_OPTIMA_NO_BROWSER -eq '1') { $NoBrowser = $true }

Write-Host '[sql-optima] Checking prerequisites...'
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw 'Docker is not installed. Install Docker Desktop or Docker Engine: https://docs.docker.com/get-docker/'
}
docker info 2>$null | Out-Null
if ($LASTEXITCODE -ne 0) {
    throw 'Docker daemon is not running. Start Docker and retry.'
}
docker compose version 2>$null | Out-Null
if ($LASTEXITCODE -ne 0) {
    throw "Docker Compose v2 is required ('docker compose')."
}
if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
    throw 'git is required to clone the repository.'
}

$repoRoot = $null
$cwd = (Get-Location).Path
if (Test-RepoRoot -Path $cwd) {
    $repoRoot = $cwd
} elseif ($Dir -and (Test-RepoRoot -Path $Dir)) {
    $repoRoot = (Resolve-Path -LiteralPath $Dir).Path
} else {
    $target = if ($Dir) { $Dir } else { Join-Path $cwd 'sql_optima' }
    if (Test-RepoRoot -Path $target) {
        $repoRoot = (Resolve-Path -LiteralPath $target).Path
    } elseif ($NoClone) {
        throw 'Not inside the sql_optima repo and -NoClone was set.'
    } else {
        Write-Host "[sql-optima] Cloning $RepoUrl into $target..."
        git clone --depth 1 $RepoUrl $target
        if ($LASTEXITCODE -ne 0) { throw "git clone failed with exit code $LASTEXITCODE" }
        $repoRoot = (Resolve-Path -LiteralPath $target).Path
    }
}

Write-Host "[sql-optima] Using repository: $repoRoot"

if (Test-NonInteractiveInstall) {
    if (-not $env:SQL_OPTIMA_SETUP_MODE) {
        Write-Host '[sql-optima] Non-interactive install: defaulting to Easy setup (no LAN DB exposure).'
        Write-Host '[sql-optima] Set SQL_OPTIMA_SETUP_MODE=custom and SQL_OPTIMA_EXPOSE_DB_LAN=1 for LAN/DBeaver.'
    }
}

$startArgs = @()
if ($NoBrowser) { $startArgs += '-NoBrowser' }

$startScript = Join-Path (Join-Path $repoRoot 'docker') 'start-dev.ps1'
if (-not (Test-Path -LiteralPath $startScript)) {
    throw "Missing $startScript"
}

& $startScript @startArgs
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
