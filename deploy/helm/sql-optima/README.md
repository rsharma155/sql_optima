# SQL Optima Helm chart

Starter chart for the **control plane** (API + SPA image). TimescaleDB, Vault, Trino, and the OS collector agent are external.

## Install

```bash
helm upgrade --install sql-optima ./deploy/helm/sql-optima \
  --set timescale.host=timescale.example \
  --set timescale.password='…' \
  --set auth.jwtSecret='…at-least-32-chars…'
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

## Notes

- Prefer `existingSecret` references in production instead of chart-managed secrets.
- OS collector remains a host-side agent (`docs/os_collector.md`); mTLS remote collectors are not in this chart yet.
- Schema bootstrap still uses `infrastructure/sql_scripts` (Job or external migrate).
