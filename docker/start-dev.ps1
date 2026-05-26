# SQL Optima — one-command local quick start (development / evaluation).
# Requires: Docker Desktop, PowerShell 5.1+
param(
    [switch]$NoBrowser
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass -Force

if ($env:SQL_OPTIMA_NO_BROWSER -eq '1') { $NoBrowser = $true }

Set-Location $PSScriptRoot

$repoRoot = Split-Path -Parent $PSScriptRoot
if (-not (Test-Path -LiteralPath (Join-Path $repoRoot 'backend\go.mod'))) {
    throw "backend/ is missing. Copy the full sql_optima repo (not only docker/). Build context is the repo root."
}

if (-not (Test-Path -LiteralPath '.env')) {
    Copy-Item -LiteralPath '.env.dev' -Destination '.env'
    Write-Host '[sql-optima] Created docker/.env from .env.dev (ready-to-run dev defaults).'
}

if (Test-Path -LiteralPath '.env') {
    $envLines = Get-Content -LiteralPath '.env'
    $filtered = $envLines | Where-Object { $_ -notmatch '^\s*VAULT_TOKEN\s*=\s*root\s*$' }
    if ($filtered.Count -lt $envLines.Count) {
        $filtered | Set-Content -LiteralPath '.env'
        Write-Host '[sql-optima] Removed VAULT_TOKEN=root from .env (API reads token from Vault volume).'
    }
}

Write-Host '[sql-optima] Starting stack (first run may take a few minutes to build)...'
docker compose up --build -d
$composeExit = $LASTEXITCODE
if ($composeExit -ne 0) {
    Write-Host ''
    Write-Host "[sql-optima] docker compose failed (exit code $composeExit)."
    Write-Host '[sql-optima] If the log above mentions parent snapshot / does not exist: not found — clear BuildKit cache:'
    Write-Host '  docker builder prune -af'
    Write-Host '  docker compose build --no-cache api'
    Write-Host '  docker compose up -d'
    Write-Host '  (Restart Docker Desktop if the error persists.)'
    Write-Host '[sql-optima] If Vault is unhealthy instead:'
    Write-Host '  docker compose logs vault --tail 80'
    Write-Host '  docker compose down -v'
    Write-Host '  docker compose up --build -d'
    Write-Host ''
    throw "docker compose failed with exit code $composeExit"
}

$apiPort = '8080'
if (Test-Path -LiteralPath '.env') {
    $line = Get-Content -LiteralPath '.env' -ErrorAction SilentlyContinue |
        Where-Object { $_ -match '^\s*API_PORT\s*=' } |
        Select-Object -Last 1
    if ($line -match '^\s*API_PORT\s*=\s*(.+)\s*$') {
        $apiPort = $Matches[1].Trim().Trim('"').Trim("'")
    }
}

$baseUrl = "http://127.0.0.1:$apiPort"
$maxWait = 900
if ($env:SQL_OPTIMA_WAIT_SEC) {
    $parsed = 0
    if ([int]::TryParse($env:SQL_OPTIMA_WAIT_SEC, [ref]$parsed) -and $parsed -gt 0) {
        $maxWait = $parsed
    }
}
$intervalSec = 3
$elapsed = 0
$lastProgress = 0

Write-Host ''
Write-Host "[sql-optima] Waiting for API at $baseUrl (up to ${maxWait}s on first build)..."

function Test-ApiReady([string]$Url) {
    try {
        $r = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 5 -ErrorAction Stop
        return ($r.StatusCode -ge 200 -and $r.StatusCode -lt 300)
    } catch {
        return $false
    }
}

function Test-TimescaleReady([string]$StatusUrl) {
    try {
        $r = Invoke-WebRequest -Uri $StatusUrl -UseBasicParsing -TimeoutSec 5 -ErrorAction Stop
        if ($r.StatusCode -lt 200 -or $r.StatusCode -ge 300) { return $false }
        $j = $r.Content | ConvertFrom-Json
        return [bool]$j.timescale_connected
    } catch {
        return $false
    }
}

$ready = $false
while ($elapsed -lt $maxWait) {
    if ((Test-ApiReady "$baseUrl/api/health") -and (Test-TimescaleReady "$baseUrl/api/setup/status")) {
        $ready = $true
        break
    }
    if (($elapsed - $lastProgress) -ge 15) {
        Write-Host "[sql-optima] Still starting... (${elapsed}s)"
        $lastProgress = $elapsed
    }
    Start-Sleep -Seconds $intervalSec
    $elapsed += $intervalSec
}

if (-not $ready) {
    Write-Host "[sql-optima] ERROR: API did not become ready within ${maxWait}s." -ForegroundColor Red
    Write-Host '[sql-optima] Recent API logs:'
    docker compose logs api --tail 50 2>$null
    throw 'API readiness timeout'
}

Write-Host "[sql-optima] API is ready (${elapsed}s)."

Write-Host ''
Write-Host '[sql-optima] Checking Vault token injection in API...'
docker compose logs api --tail 15 2>$null
$vaultTok = (docker compose exec -T api printenv VAULT_TOKEN 2>$null | Out-String).Trim()
$tokenFileOk = $false
docker compose exec -T api test -r /vault/token/.root_token 2>$null | Out-Null
if ($LASTEXITCODE -eq 0) { $tokenFileOk = $true }
if ([string]::IsNullOrWhiteSpace($vaultTok) -or $vaultTok -eq 'root' -or -not $tokenFileOk) {
    Write-Host '[sql-optima] WARNING: API may not have a valid Vault token. Try:'
    Write-Host '  docker compose up -d --build --force-recreate api'
}

$displayUrl = "http://localhost:$apiPort"
if (-not $NoBrowser) {
    try {
        Start-Process $displayUrl | Out-Null
        Write-Host "[sql-optima] Opened browser at $displayUrl"
    } catch {
        Write-Host "[sql-optima] Could not launch a browser automatically. Open: $displayUrl"
    }
} else {
    Write-Host '[sql-optima] Browser launch skipped (-NoBrowser).'
}

Write-Host ''
Write-Host "[sql-optima] $displayUrl"
Write-Host '[sql-optima] First visit: setup wizard — create your admin username and password.'
Write-Host '[sql-optima] Then: add PostgreSQL or SQL Server (or use the in-app guide for local HA test clusters).'
Write-Host ''
Write-Host 'Stop (keep data):  docker compose down'
Write-Host 'Stop + wipe data: docker compose down -v'
