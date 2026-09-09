#!/usr/bin/env bash
# Sync infrastructure/sql_scripts (01–07) into the Helm chart ConfigMap source tree.
# Run from repo root after editing SQL bootstrap scripts.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
SRC="$ROOT/infrastructure/sql_scripts"
DST="$ROOT/deploy/helm/sql-optima/files/sql"
mkdir -p "$DST"
for f in \
  01_timescale_schema.sql \
  02_rule_engine.sql \
  03_additional_pg_rules.sql \
  04_alert_engine.sql \
  05_os_metrics_collector.sql \
  06_seed_data.sql \
  07_optima_server_dr_policy.sql
do
  cp -f "$SRC/$f" "$DST/$f"
  echo "synced $f"
done
