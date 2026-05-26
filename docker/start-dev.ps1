# SQL Optima - one-command local quick start (development / evaluation).
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
$backendGoMod = Join-Path (Join-Path $repoRoot 'backend') 'go.mod'
if (-not (Test-Path -LiteralPath $backendGoMod)) {
    throw 'backend/ is missing. Copy the full sql_optima repo (not only docker/). Build context is the repo root.'
}

function Get-DevLanIp {
    $ip = (Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
        Where-Object { $_.IPAddress -notmatch '^127\.' -and $_.PrefixOrigin -ne 'WellKnown' } |
        Select-Object -First 1 -ExpandProperty IPAddress)
    if (-not $ip) {
        $ip = (Get-NetIPConfiguration -ErrorAction SilentlyContinue |
            Where-Object { $_.IPv4DefaultGateway -and $_.NetAdapter.Status -eq 'Up' } |
            Select-Object -First 1 -ExpandProperty IPv4Address).IPAddress
    }
    return $ip
}

function Test-DevInteractive {
    if ($env:SQL_OPTIMA_SETUP_MODE) { return $false }
    try { return [Console]::IsInputRedirected -eq $false } catch { return $false }
}

function Read-DevDefault([string]$Prompt, [string]$Default) {
    $input = Read-Host "$Prompt [$Default]"
    if ([string]::IsNullOrWhiteSpace($input)) { return $Default }
    return $input.Trim()
}

function Read-DevYesNo([string]$Prompt, [bool]$DefaultNo = $true) {
    $input = Read-Host "$Prompt [y/N]"
    if ([string]::IsNullOrWhiteSpace($input)) {
        return -not $DefaultNo
    }
    return ($input.Trim().ToLowerInvariant() -match '^(y|yes)$')
}

function Resolve-DevSetup {
    $script:DevSetupMode = if ($env:SQL_OPTIMA_SETUP_MODE) { $env:SQL_OPTIMA_SETUP_MODE } else { 'easy' }
    $script:DevExposeLan = if ($env:SQL_OPTIMA_EXPOSE_DB_LAN -eq '1') { $true } else { $false }
    $script:DevDbUser = if ($env:SQL_OPTIMA_DB_USER) { $env:SQL_OPTIMA_DB_USER } else { 'dbmonitor' }
    $script:DevDbPassword = if ($env:SQL_OPTIMA_DB_PASSWORD) { $env:SQL_OPTIMA_DB_PASSWORD } else { 'sql_optima_dev_local_only' }
    $script:DevDbName = if ($env:SQL_OPTIMA_DB_NAME) { $env:SQL_OPTIMA_DB_NAME } else { 'dbmonitor_metrics' }
    $script:DevApiPort = if ($env:SQL_OPTIMA_API_PORT) { $env:SQL_OPTIMA_API_PORT } else { '8080' }
    $script:DevDbPublishPort = if ($env:SQL_OPTIMA_DB_PUBLISH_PORT) { $env:SQL_OPTIMA_DB_PUBLISH_PORT } else { '5432' }
    $script:DevDbPublishBind = $env:SQL_OPTIMA_DB_PUBLISH_BIND

    if (Test-DevInteractive) {
        Write-Host ''
        Write-Host 'SQL Optima Dev Setup'
        Write-Host 'Choose configuration:'
        Write-Host '  1) Easy setup (safe): Local-only DB. No LAN/DBeaver access to TimescaleDB.'
        Write-Host '  2) Custom setup: Configure credentials and optionally enable LAN/DBeaver access.'
        Write-Host ''
        $choice = Read-DevDefault 'Enter 1 or 2' '1'
        if ($choice -eq '2') {
            $script:DevSetupMode = 'custom'
        } else {
            $script:DevSetupMode = 'easy'
            $script:DevExposeLan = $false
        }
    }

    if ($script:DevSetupMode -eq 'easy') {
        $script:DevExposeLan = $false
        if (Test-DevInteractive) {
            Write-Host ''
            Write-Host 'Easy setup selected.'
            Write-Host 'Starting the dev stack with safe defaults...'
        }
        return
    }

    if (Test-DevInteractive) {
        Write-Host ''
        Write-Host 'Custom setup selected.'
        Write-Host ''
        Write-Host 'TimescaleDB (monitoring DB) defaults:'
        Write-Host "  DB name   [$script:DevDbName]"
        Write-Host "  DB user   [$script:DevDbUser]"
        Write-Host '  (Press Enter to keep defaults.)'
        Write-Host ''
        $script:DevDbName = Read-DevDefault 'DB name' $script:DevDbName
        $script:DevDbUser = Read-DevDefault 'DB user' $script:DevDbUser
        $secure = Read-Host 'DB password (Enter for default)' -AsSecureString
        $plain = [Runtime.InteropServices.Marshal]::PtrToStringAuto(
            [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure))
        if (-not [string]::IsNullOrWhiteSpace($plain)) {
            $script:DevDbPassword = $plain
        }
        Write-Host ''
        $script:DevApiPort = Read-DevDefault 'API server port (API_PORT)' $script:DevApiPort
        Write-Host ''
        Write-Host 'Enable LAN/DBeaver access to the monitoring DB?'
        Write-Host '  This publishes the DB port so other machines on your LAN can connect.'
        Write-Host '  Bind scope: LAN-only (host LAN IP).'
        $script:DevExposeLan = Read-DevYesNo 'Enable LAN?' $true
        if ($script:DevExposeLan) {
            $script:DevDbPublishPort = Read-DevDefault 'Publish port on host' $script:DevDbPublishPort
        }
    }

    if ($script:DevExposeLan) {
        if ([string]::IsNullOrWhiteSpace($script:DevDbPublishBind)) {
            $script:DevDbPublishBind = Get-DevLanIp
        }
        if ([string]::IsNullOrWhiteSpace($script:DevDbPublishBind)) {
            throw 'Could not detect LAN IP. Set SQL_OPTIMA_DB_PUBLISH_BIND and retry.'
        }
    }
}

function Write-DevEnvFile([string]$Path = '.env') {
    $lines = @(
        '# Generated by start-dev.ps1 - dev evaluation only (not for production).'
        "DB_USER=$script:DevDbUser"
        "DB_PASSWORD=$script:DevDbPassword"
        "DB_NAME=$script:DevDbName"
        ''
        "API_PORT=$script:DevApiPort"
        ''
        'AUTH_REQUIRED=1'
        'DISABLE_PUBLIC_SETUP=0'
        'JWT_SECRET=sql-optima-local-dev-jwt-secret-32chars-min'
        ''
        'VAULT_TRANSIT_KEY=sql-optima'
        ''
        'COLD_STORAGE_ENABLED=false'
        "COLD_STORAGE_SECRET_ACCESS_KEY=$script:DevDbPassword"
        ''
        "SQL_OPTIMA_SETUP_MODE=$script:DevSetupMode"
        "SQL_OPTIMA_EXPOSE_DB_LAN=$(if ($script:DevExposeLan) { '1' } else { '0' })"
    )
    if ($script:DevExposeLan) {
        $lines += @(
            ''
            'COMPOSE_FILE=docker-compose.yml:docker-compose.dev.yml'
            "DB_PUBLISH_PORT=$script:DevDbPublishPort"
            "DB_PUBLISH_BIND=$script:DevDbPublishBind"
        )
    }
    Set-Content -LiteralPath $Path -Value ($lines -join "`n") -Encoding utf8
}

function Test-DevEnvExposeLan {
    if (-not (Test-Path -LiteralPath '.env')) { return $false }
    $expose = Get-EnvVal 'SQL_OPTIMA_EXPOSE_DB_LAN' '0'
    if ($expose -eq '1') { return $true }
    $compose = Get-EnvVal 'COMPOSE_FILE' ''
    return ($compose -match 'docker-compose\.dev\.yml')
}

if (-not (Test-Path -LiteralPath '.env')) {
    if (-not (Test-DevInteractive)) {
        Write-Host '[sql-optima] Non-interactive install: using Easy setup (no LAN DB exposure).'
        Write-Host '[sql-optima] Override with SQL_OPTIMA_SETUP_MODE=custom and SQL_OPTIMA_EXPOSE_DB_LAN=1 if needed.'
    }
    Resolve-DevSetup
    Write-DevEnvFile
    Write-Host "[sql-optima] Created docker/.env ($script:DevSetupMode setup)."
}

if (Test-Path -LiteralPath '.env') {
    $envLines = Get-Content -LiteralPath '.env'
    $filtered = $envLines | Where-Object { $_ -notmatch '^\s*VAULT_TOKEN\s*=\s*root\s*$' }
    if ($filtered.Count -lt $envLines.Count) {
        $filtered | Set-Content -LiteralPath '.env'
        Write-Host '[sql-optima] Removed VAULT_TOKEN=root from .env (API reads token from Vault volume).'
    }
}

function Get-EnvVal([string]$Key, [string]$Default) {
    if (-not (Test-Path -LiteralPath '.env')) { return $Default }
    $line = Get-Content -LiteralPath '.env' -ErrorAction SilentlyContinue |
        Where-Object { $_ -match "^\s*$([regex]::Escape($Key))\s*=" } |
        Select-Object -Last 1
    if ($line -match '^\s*\S+\s*=\s*(.+)\s*$') { return $Matches[1].Trim().Trim('"').Trim("'") }
    return $Default
}

function Write-TimescaleAccess {
    if (-not (Test-DevEnvExposeLan)) {
        Write-Host '[sql-optima] TimescaleDB is not published to the LAN (Easy setup or LAN disabled).'
        Write-Host '[sql-optima] To inspect the DB: docker compose exec timescaledb psql -U dbmonitor -d dbmonitor_metrics'
        return
    }
    $pubPort = Get-EnvVal 'DB_PUBLISH_PORT' '5432'
    $bindIp = Get-EnvVal 'DB_PUBLISH_BIND' ''
    $dbUser = Get-EnvVal 'DB_USER' 'dbmonitor'
    $dbName = Get-EnvVal 'DB_NAME' 'dbmonitor_metrics'
    Write-Host "[sql-optima] TimescaleDB for DBeaver / psql (LAN-enabled, bind $bindIp):"
    Write-Host "  Host: $bindIp  Port: $pubPort  Database: $dbName  User: $dbUser"
    Write-Host '  Password: (the DB password you chose during setup, or see docker/.env DB_PASSWORD)'
    Write-Host '  Same machine: use Host above if localhost fails (LAN-only bind).'
    Write-Host "  Allow TCP $pubPort in the host firewall for remote machines."
}

Write-Host '[sql-optima] Starting stack (first run may take a few minutes to build)...'
docker compose up --build -d
$composeExit = $LASTEXITCODE
if ($composeExit -ne 0) {
    Write-Host ''
    Write-Host "[sql-optima] docker compose failed (exit code $composeExit)."
    Write-Host '[sql-optima] If the log above mentions parent snapshot / does not exist: not found - clear BuildKit cache:'
    Write-Host '  docker builder prune -af'
    Write-Host '  docker compose build --no-cache api'
    Write-Host '  docker compose up -d'
    Write-Host '  (Restart Docker Desktop if the error persists.)'
    Write-Host '[sql-optima] Service status and logs:'
    docker compose ps -a 2>$null
    docker compose logs schema-setup --tail 40 2>$null
    docker compose logs api --tail 40 2>$null
    docker compose logs vault --tail 40 2>$null
    Write-Host '[sql-optima] Vault transit mount success usually means Vault is OK - check schema-setup / API build above.'
    Write-Host '[sql-optima] Password mismatch or partial install: docker compose down -v then restart.'
    Write-Host ''
    throw "docker compose failed with exit code $composeExit"
}

$apiPort = Get-EnvVal 'API_PORT' '8080'

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
Write-Host '[sql-optima] First visit: setup wizard - create your admin username and password.'
Write-Host '[sql-optima] Then: add PostgreSQL or SQL Server (or use the in-app guide for local HA test clusters).'
Write-Host ''
Write-TimescaleAccess
Write-Host ''
Write-Host 'Stop (keep data):  docker compose down'
Write-Host 'Stop + wipe data: docker compose down -v'
