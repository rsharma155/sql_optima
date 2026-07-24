# Release Engineering (P2-6)

<!-- Author: Ravi Sharma -->
<!-- Copyright (c) 2026 Ravi Sharma -->
<!-- SPDX-License-Identifier: MIT -->

## Versioning

- **Source of truth**: [`VERSION`](../VERSION) at repo root (also read in CI).
- **Changelog**: [`CHANGELOG.md`](../CHANGELOG.md) (Keep a Changelog format).
- **Detailed notes**: [`RELEASES.md`](../RELEASES.md) for narrative release history.

Until **1.0.0**, minor releases may include breaking API or schema changes; document them in CHANGELOG.

## Container images (GHCR)

Images are published to GitHub Container Registry on **version tags**:

```text
ghcr.io/<org>/sql-optima:<version>
ghcr.io/<org>/sql-optima:latest   # only on stable x.y.z tags
```

### Maintainer: cut a release

1. Update `VERSION`, `CHANGELOG.md`, and `RELEASES.md`.
2. Commit and tag:

   ```bash
   git tag -a v0.5.0 -m "v0.5.0"
   git push origin v0.5.0
   ```

3. GitHub Actions workflow `.github/workflows/release.yml` builds and pushes the image, attaches an **SPDX SBOM**, and opens a GitHub Release with notes extracted from `CHANGELOG.md`.

### Consumers: pin by digest

Production should pin an immutable digest, not `:latest`:

```yaml
image: ghcr.io/rsharma155/sql-optima:0.5.0
```

SBOM artifact: attached to the GitHub Release as `sbom-sql-optima.spdx.json` (BuildKit also records image SBOM/provenance).

## Pre-release checklist

- [ ] `cd backend && go test -race -timeout 120s ./...`
- [ ] `cd backend && golangci-lint run` (or CI green)
- [ ] `AUTH_REQUIRED=1` smoke test on Docker compose
- [ ] Schema migrations idempotent (`infrastructure/sql_scripts/`)
- [ ] Update `CHANGELOG.md` / `RELEASES.md` / `VERSION` for the cut

## OS collector artifact

The host agent is a **separate shell script** (`os_collector/sql-optima-os-collector.sh`). It is not included in the main `sql-optima` image; copy to each PostgreSQL Linux host via Ansible/chef or package it in your DB AMI.

CI runs `shellcheck` and fixture tests under `os_collector/test/`.
