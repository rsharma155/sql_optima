# Sync infrastructure/sql_scripts (01–07) into the Helm chart ConfigMap source tree.
# Run from repo root after editing SQL bootstrap scripts.
$ErrorActionPreference = "Stop"
$Root = Resolve-Path (Join-Path $PSScriptRoot "..\..\..\..")
$Src = Join-Path $Root "infrastructure\sql_scripts"
$Dst = Join-Path $Root "deploy\helm\sql-optima\files\sql"
New-Item -ItemType Directory -Force -Path $Dst | Out-Null
$files = @(
  "01_timescale_schema.sql",
  "02_rule_engine.sql",
  "03_additional_pg_rules.sql",
  "04_alert_engine.sql",
  "05_os_metrics_collector.sql",
  "06_seed_data.sql",
  "07_optima_server_dr_policy.sql"
)
foreach ($f in $files) {
  Copy-Item -Force (Join-Path $Src $f) (Join-Path $Dst $f)
  Write-Host "synced $f"
}
