# SQL Optima OS Collector

Lightweight **Linux shell agent** that runs on the **PostgreSQL database host** and pushes host RAM, CPU, load, and PostgreSQL process RSS to the SQL Optima API.

- **PostgreSQL-only** — does not connect to the database.
- **No Go** on the DB host — only `bash`, `curl`, `jq`, and `awk`.
- **Pre-configured zip** from the SQL Optima UI (instance name, server ID, app URL, metrics endpoint).

## Quick start (recommended)

### 1. Download from SQL Optima

In the UI, use **Download bundle (.zip)**:

- **Admin → Add server** (when engine is PostgreSQL), or
- **PostgreSQL → Memory** or **CPU** (when the OS Collector setup panel is shown)

Download **after** the server is saved so **server ID** is included in the bundle.

### 2. Enable ingest on the monitoring server (one-time)

**From the SQL Optima UI (no restart):** On Memory/CPU, click **Enable ingest** in the OS Collector panel, or **Download bundle** (enables ingest automatically). Requires DBA/Admin.

**Or** set `OS_METRICS_INGEST_ENABLED=1` in `docker/.env` / `backend/.env` and restart the API (locks ingest to env).

Create an admin user and note the JWT for the agent.

### 3. Install on the PostgreSQL Linux host

```bash
unzip sql-optima-os-collector-*.zip
cd sql-optima-os-collector-*   # or the folder you unzipped into
chmod +x quick-install.sh sql-optima-os-collector.sh
./quick-install.sh
```

You are prompted **once** for the admin JWT. Everything else is already in `bundled-config.env`.

`quick-install.sh` runs `sql-optima-os-collector.sh install`, which:

- Writes config to `/etc/sql-optima/os-collector.env` (as root) or `~/.config/sql-optima/os-collector.env` (non-root)
- Copies the agent to `/usr/local/bin/` or `~/.local/bin/`
- Adds a **cron job** (default: every **5 minutes**)
- Appends logs to `/var/log/sql-optima-os-collector.log` or `~/.local/share/sql-optima/os-collector.log`

### 4. Verify

- `journalctl` is not used for cron; check the log file above.
- SQL Optima UI: **PostgreSQL → Memory** shows the **OS Collector Active** badge and host RAM KPIs.
- Optional test: `source bundled-config.env` then `./sql-optima-os-collector.sh --once` (expect HTTP 202).

## What is pre-filled in the zip

| Setting | Source |
|---------|--------|
| `SQL_OPTIMA_INSTANCE_NAME` | Monitored server name in SQL Optima (spaces OK, e.g. `Postgres Local`) |
| `SQL_OPTIMA_SERVER_ID` | UUID from server registry (after save) |
| `SQL_OPTIMA_APP_URL` | SQL Optima URL you used in the browser |
| `SQL_OPTIMA_BACKEND_URL` | `{app_url}/api/os/metrics` |
| `SQL_OPTIMA_API_KEY` | You enter at install (or edit `bundled-config.env` first) |

Files in the zip:

| File | Purpose |
|------|---------|
| `quick-install.sh` | One-command install (cron + config) |
| `sql-optima-os-collector.sh` | Agent script (values baked in from UI) |
| `bundled-config.env` | Same settings as env file; add JWT here |
| `INSTALL.txt` | Host install checklist |
| `README.txt` | Short reference |
| `systemd/sql-optima-os-collector.service` | Optional unit for daemon mode |

## Scheduling: cron vs systemd

The agent does **not** run as a long-lived process unless you choose systemd or foreground mode.

| Mode | How to install | Interval |
|------|----------------|----------|
| **Cron (default)** | `./quick-install.sh` | Every 5 minutes (`SQL_OPTIMA_CRON_INTERVAL_MIN` in config) |
| **Systemd daemon** | `sudo ./sql-optima-os-collector.sh install --systemd` | ~30s loop (requires root + systemd) |
| **Foreground** | `./sql-optima-os-collector.sh -interval 30s` | Continuous; for testing only |
| **One-shot** | `./sql-optima-os-collector.sh --once` | Single collect + push (what cron runs) |

Cron is created automatically by `install`. Linux cron cannot schedule sub-minute jobs; use **systemd** if you need ~30s sampling.

## Operations: monitor, failures, and stop

### Impact on the PostgreSQL host

Each **cron** run is a short **one-shot** (`--once`), not a long-lived daemon.

| Activity | Impact |
|----------|--------|
| Reads `/proc/meminfo`, `/proc/stat`, `/proc/loadavg`, `/proc/cpuinfo` | Read-only; negligible I/O |
| Scans `/proc/[pid]` for `postgres` processes | Brief directory walk; no signals, no DB connection |
| **1 second sleep** for CPU % sample | ~1s once per cron interval (default every 5 minutes) |
| One `curl` POST (max **10s** timeout) | Small outbound HTTP(S) payload; no inbound port |
| Appends to the **log file** | Slow growth; rotate if needed |

The agent does **not** start PostgreSQL, change its config, or open SQL ports. A broken install only means cron keeps running a lightweight script that logs errors and exits.

Default paths after install:

| Install as | Config | Log | Binary |
|------------|--------|-----|--------|
| Non-root | `~/.config/sql-optima/os-collector.env` | `~/.local/share/sql-optima/os-collector.log` | `~/.local/bin/sql-optima-os-collector.sh` |
| Root | `/etc/sql-optima/os-collector.env` | `/var/log/sql-optima-os-collector.log` | `/usr/local/bin/sql-optima-os-collector.sh` |

### How to monitor

**1. Log file (primary on the DB host)**

```bash
# non-root example
tail -f ~/.local/share/sql-optima/os-collector.log
```

Cron redirects **stdout and stderr** to this file. `journalctl` applies only if you installed with **systemd** (`install --systemd`).

| Log line | Meaning |
|----------|---------|
| *(no new lines each interval)* | Cron missing, wrong user crontab, or script path broken |
| `push failed: HTTP 401` | JWT expired or invalid — update `SQL_OPTIMA_API_KEY` |
| `push failed: HTTP 404` | Wrong URL path (should be `/api/os/metrics`) |
| `push failed: HTTP 403` | Ingest disabled — click **Enable ingest** in UI; remove `OS_METRICS_INGEST_ENABLED=0` from API env (Docker defaulted to off) |
| `push failed: HTTP 400` | Often **instance name mismatch** vs SQL Optima Admin |
| `collect failed` | Could not read `/proc` (permissions, non-Linux host) |
| `FATAL: ...` | Missing config or dependencies; run exits before push |
| `push recovered after N failure(s)` | API was down, then succeeded (common in **daemon** mode) |

**2. Manual test (same command install prints)**

```bash
ENV_FILE=~/.config/sql-optima/os-collector.env \
  ~/.local/bin/sql-optima-os-collector.sh --once
echo "exit code: $?"
```

Exit **0** = collect + push OK. Non-zero = check the log immediately after.

**3. SQL Optima UI**

On **PostgreSQL → Memory** or **CPU**, use **Refresh status** in the OS Collector panel:

- **Host metrics received** — samples in TimescaleDB within ~20 minutes
- **No host metrics yet** — no successful push for this instance

**4. Confirm cron is installed**

```bash
crontab -l | grep sql-optima-os-collector
```

**5. Optional: watch for recent failures**

```bash
grep -E 'push failed|FATAL|collect failed' ~/.local/share/sql-optima/os-collector.log | tail -5
```

### When the API is unreachable

**Cron mode (default):**

1. The script still **collects** metrics from `/proc`.
2. `curl` fails or returns non-2xx → logged (e.g. `push failed: HTTP 503`).
3. That run **exits with failure**; there is **no in-run retry** until the **next cron tick** (default 5 minutes).
4. **No backoff** in cron mode (backoff exists only in continuous **daemon/systemd** loop).
5. PostgreSQL is **unaffected**; SQL Optima host charts stop updating until pushes succeed again.
6. When the API returns, the **next** cron run recovers automatically.

**JWT expiry:** pushes fail with **401** every interval until you update `SQL_OPTIMA_API_KEY` in the env file (new admin JWT from SQL Optima login).

**Long outage:** repeated log lines and stale dashboards only — no crash loop on the DB host.

| Mode | On API failure | Where to look |
|------|----------------|---------------|
| **Cron** | Fail once per interval; retry on next cron | Log file + UI status |
| **Systemd daemon** | Push errors + **backoff sleep** (5s…120s) inside the process | Log file + `systemctl status sql-optima-os-collector` |

### Error behavior summary

| Failure | Cron behavior | DB / OS impact |
|---------|---------------|----------------|
| API down / timeout | Log error; exit 1; retry next interval | None |
| Wrong JWT | HTTP 401 in log | None |
| Ingest disabled | HTTP 404 in log | None |
| Wrong instance name | HTTP 400 in log | None |
| Missing `curl` / `jq` | `FATAL: missing required commands` | None |
| Cannot read `/proc` | `FATAL` or `collect failed` | None |

Cron does **not** disable itself after repeated failures — it keeps trying on schedule so recovery is automatic when the API is back.

### Gracefully stop, pause, and resume

**Stop completely (remove schedule)**

```bash
crontab -l 2>/dev/null | grep -v 'sql-optima-os-collector' | crontab -
crontab -l | grep sql-optima || echo "cron job removed"
```

Optional cleanup (does not affect PostgreSQL):

```bash
rm -f ~/.local/bin/sql-optima-os-collector.sh
rm -f ~/.config/sql-optima/os-collector.env
# keep or delete: ~/.local/share/sql-optima/os-collector.log
```

**Pause without uninstalling**

Comment the line in `crontab -e`, or:

```bash
crontab -l > /tmp/cron.bak
crontab -l | sed 's|^\(.*sql-optima-os-collector.*\)|# \1|' | crontab -
```

**Resume:** restore from `/tmp/cron.bak` or re-run `./quick-install.sh` from the zip folder.

**Systemd install**

```bash
sudo systemctl stop sql-optima-os-collector
sudo systemctl disable sql-optima-os-collector
```

**Update JWT without reinstall**

```bash
nano ~/.config/sql-optima/os-collector.env   # SQL_OPTIMA_API_KEY=...
chmod 600 ~/.config/sql-optima/os-collector.env
ENV_FILE=~/.config/sql-optima/os-collector.env ~/.local/bin/sql-optima-os-collector.sh --once
```

### Dismissing the setup banner in the UI

On Memory, CPU, Admin, and onboarding pages, the **Host telemetry (OS Collector)** panel can be **collapsed** (this browser tab only) or **Dismissed** (hidden on all pages). Dismiss is stored in browser `localStorage` (`sql_optima_os_collector_prompt_dismissed`).

To show the banner again (e.g. after fixing ingest):

```javascript
// Browser devtools console on the SQL Optima app origin
clearOsCollectorPromptDismiss()
// then reload the page
```

### Optional log hygiene

Truncate if the log grows large (example: when over 5 MB):

```bash
LOG=~/.local/share/sql-optima/os-collector.log
[ -f "$LOG" ] && [ "$(wc -c <"$LOG")" -gt 5242880 ] && : >"$LOG"
```

## Manual / advanced usage

```bash
# Edit JWT before install
nano bundled-config.env

# Install with systemd instead of cron
sudo ./sql-optima-os-collector.sh install --systemd

# Run once (dry-run JSON to stdout)
SQL_OPTIMA_COLLECT_ONLY=1 ./sql-optima-os-collector.sh

# Flags (override bundled-config.env)
./sql-optima-os-collector.sh -url https://monitor.example.com/api/os/metrics \
  -instance my-pg-prod -key "$JWT" -interval 30s
```

## Requirements

| Requirement | Notes |
|-------------|--------|
| Linux + `/proc` | RHEL, Ubuntu, Debian, etc. |
| `bash`, `curl`, `jq`, `awk` | Checked at startup |
| Outbound HTTPS | To your SQL Optima API |
| Admin JWT | Bearer token for `POST /api/os/metrics` |
| `OS_METRICS_INGEST_ENABLED=1` | On the **monitoring** server, not the DB host |

Run as the `postgres` user (or a dedicated user) so `/proc` for PostgreSQL PIDs is readable.

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `missing required commands` | Install `curl` and `jq` on the host |
| `set SQL_OPTIMA_API_KEY` | Run `./quick-install.sh` interactively or edit `bundled-config.env` |
| HTTP 403 on push | Enable ingest in the UI (**Enable ingest** or download bundle), or set `OS_METRICS_INGEST_ENABLED=1` and restart API |
| `push failed (1), backoff 5s` in cron log | Fixed in current script (`--once` did not set one-shot mode); copy updated `sql-optima-os-collector.sh` to `~/.local/bin/` or re-run `quick-install.sh` |
| HTTP 401 / 403 | Use a valid **admin** JWT |
| HTTP 400 unknown instance | Instance name in bundle must match Admin → Servers exactly (including spaces/case) |
| `Local: command not found` when sourcing env | Re-download bundle (old unquoted env); names with spaces must be quoted in `bundled-config.env` |
| Server ID `unknown` in zip | Save the server in SQL Optima first, then re-download |
| Badge still hidden | Agent must run on the **same host** as PostgreSQL; wait one cron cycle |
| Want 30s samples | `sudo ./sql-optima-os-collector.sh install --systemd` |

## Development

```bash
./test/run_tests.sh
```

Metric field mapping: [`PARITY.md`](PARITY.md).

Bundled API assets (sync when the script changes): [`../backend/internal/oscollectorbundle/assets/`](../backend/internal/oscollectorbundle/assets/).

## See also

- [`docs/os_collector.md`](../docs/os_collector.md) — platform-side enablement and API
- Root [`README.md`](../README.md) — project overview
