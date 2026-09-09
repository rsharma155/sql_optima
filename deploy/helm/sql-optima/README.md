# SQL Optima Helm chart

Control-plane chart (API + SPA). Optionally bundles a **TimescaleDB** subchart and a **schema Job** that applies `infrastructure/sql_scripts` (01–07), matching Docker `schema-setup`.

## Install (external TimescaleDB)

```bash
helm upgrade --install sql-optima ./deploy/helm/sql-optima \
  --set timescale.host=timescale.example \
  --set timescale.password='…' \
  --set auth.jwtSecret='…at-least-32-chars…'
```

## Install (bundled TimescaleDB + schema)

```bash
RELEASE=sql-optima
PW='…strong-db-password…'
JWT='…at-least-32-chars…'

helm upgrade --install "$RELEASE" ./deploy/helm/sql-optima \
  -f deploy/helm/sql-optima/values-bundled-example.yaml \
  --set timescale.password="$PW" \
  --set auth.jwtSecret="$JWT" \
  --set timescaledb.auth.existingSecret="${RELEASE}-sql-optima-secrets"
```

The API Deployment waits (init container) until `pg_isready` succeeds and, when `schemaJob.enabled`, until table `optima_servers` exists.

### Schema-only (external DB)

```bash
helm upgrade --install sql-optima ./deploy/helm/sql-optima \
  --set timescale.host=timescale.example \
  --set timescale.password="$PW" \
  --set auth.jwtSecret="$JWT" \
  --set schemaJob.enabled=true
```

## OIDC groups

```yaml
auth:
  mode: oidc
  oidc:
    issuerURL: https://idp.example/realms/optima
    audience: sql-optima
    groupClaim: groups
    groupRoleMap: "sql-optima-admins:admin,sql-optima-dbas:dba,sql-optima-viewers:viewer"
```

## Syncing SQL into the chart

Bootstrap scripts are copied under `files/sql/` for the schema ConfigMap. After changing `infrastructure/sql_scripts/0*.sql`, re-sync:

```bash
# Linux/macOS
./deploy/helm/sql-optima/scripts/sync-sql-scripts.sh

# Windows PowerShell
./deploy/helm/sql-optima/scripts/sync-sql-scripts.ps1
```

## Notes

- Prefer `existingSecret` references in production instead of chart-managed secrets.
- OS collector remains a host-side agent (`docs/os_collector.md`); mTLS remote collectors are not in this chart yet.
- Vault / Trino / cold storage remain external; set `vault.*` / `coldStorage.*` when using them.
- Chart version **0.2.0** introduces optional `timescaledb` subchart + `schemaJob`.
