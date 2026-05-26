# SQL Optima — one-command install from anywhere (clone if needed, then start dev stack).
# Usage:
#   PowerShell -ExecutionPolicy Bypass -File .\install.ps1
#   irm https://raw.githubusercontent.com/rsharma155/sql_optima/main/install.ps1 | iex
param(
    [string]$Dir = '',
    [switch]$NoBrowser,
    [switch]$NoClone
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass -Force

$RepoUrl = if ($env:SQL_OPTIMA_REPO_URL) { $env:SQL_OPTIMA_REPO_URL } else { 'https://github.com/rsharma155/sql_optima.git' }
if ($env:SQL_OPTIMA_DIR -and -not $Dir) { $Dir = $env:SQL_OPTIMA_DIR }
if ($env:SQL_OPTIMA_NO_BROWSER -eq '1') { $NoBrowser = $true }

function Test-RepoRoot([string]$Path) {
    return (Test-Path -LiteralPath (Join-Path $Path 'backend\go.mod'))
}

Write-Host '[sql-optima] Checking prerequisites...'
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "Docker is not installed. Install Docker Desktop: https://www.docker.com/products/docker-desktop/"
}
docker info 2>$null | Out-Null
if ($LASTEXITCODE -ne 0) {
    throw 'Docker daemon is not running. Start Docker Desktop and retry.'
}
$composeOk = $false
docker compose version 2>$null | Out-Null
if ($LASTEXITCODE -eq 0) { $composeOk = $true }
if (-not $composeOk) {
    throw "Docker Compose v2 is required ('docker compose')."
}
if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
    throw 'git is required to clone the repository.'
}

$repoRoot = $null
if (Test-RepoRoot -Path (Get-Location).Path) {
    $repoRoot = (Get-Location).Path
} elseif ($Dir -and (Test-RepoRoot -Path $Dir)) {
    $repoRoot = (Resolve-Path -LiteralPath $Dir).Path
} else {
    $target = if ($Dir) { $Dir } else { Join-Path (Get-Location).Path 'sql_optima' }
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

$startArgs = @()
if ($NoBrowser) { $startArgs += '-NoBrowser' }

$startScript = Join-Path $repoRoot 'docker\start-dev.ps1'
if (-not (Test-Path -LiteralPath $startScript)) {
    throw "Missing $startScript"
}

& $startScript @startArgs
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
