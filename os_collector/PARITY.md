# OS Collector — Metric Parity Specification

Acceptance criteria for the Linux shell agent (`sql-optima-os-collector.sh`) matching the former Go agent (gopsutil v3.24.5).

## JSON payload

Field names and types must match `service.OSCollectorPayload` in `backend/internal/service/os_metrics_bridge.go`.

| Field | Type | Source |
|-------|------|--------|
| `ts` | RFC3339 UTC | `date -u +%Y-%m-%dT%H:%M:%SZ` |
| `hostname` | string | `hostname -f` or `/proc/sys/kernel/hostname` |
| `instance_name` | string | config |
| `total_bytes` … `writeback_bytes` | uint64 | `/proc/meminfo` (see below) |
| `swap_*` | uint64 | `/proc/meminfo` `SwapTotal` / `SwapFree` |
| `cpu_*_pct` | float64 | `/proc/stat` aggregate `cpu` line, 1s delta |
| `load_1m` / `load_5m` / `load_15m` | float64 | `/proc/loadavg` fields 1–3 |
| `cpu_cores` | int | `nproc` (logical CPUs) |
| `postgres_rss_bytes` / `postgres_vsz_bytes` | uint64 | sum over `/proc/*/comm` == `postgres` |
| `backend_count` | int | count of matching postgres processes |

## Memory (`/proc/meminfo`)

All values in meminfo are kB; multiply by **1024** for bytes.

| JSON field | gopsutil / meminfo rule |
|------------|-------------------------|
| `total_bytes` | `MemTotal` × 1024 |
| `available_bytes` | `MemAvailable` × 1024 if present; else `MemFree` + `Cached` (after SReclaimable merge) |
| `free_bytes` | `MemFree` × 1024 |
| `cached_bytes` | (`Cached` + `SReclaimable`) × 1024 |
| `buffers_bytes` | `Buffers` × 1024 |
| `shared_bytes` | `Shmem` × 1024 |
| `slab_bytes` | `Slab` × 1024 |
| `dirty_bytes` | `Dirty` × 1024 |
| `writeback_bytes` | `Writeback` × 1024 |
| `used_bytes` | `total_bytes - free_bytes - buffers_bytes - cached_bytes` (uses merged cached) |

## Swap

| JSON field | Rule |
|------------|------|
| `swap_total_bytes` | `SwapTotal` × 1024 |
| `swap_free_bytes` | `SwapFree` × 1024 |
| `swap_used_bytes` | `swap_total_bytes - swap_free_bytes` |

Note: gopsutil `SwapMemory()` uses `sysinfo(2)`; meminfo swap totals are equivalent on typical Linux hosts. Fixtures use meminfo.

## CPU (`/proc/stat`)

Read the first line starting with `cpu ` (aggregate). Fields 2–10: user, nice, system, idle, iowait, irq, softirq, steal (jiffies).

Sample twice with **sleep 1** between reads.

```
delta(x) = after - before if after >= before else 0
total = du + ds + di + dio + dn + dirq + dsoft + dsteal
if total <= 0 → user=0, system=0, idle=100, iowait=0
else:
  cpu_user_pct   = (du + dn) / total * 100
  cpu_system_pct = (ds + dirq + dsoft) / total * 100
  cpu_idle_pct   = di / total * 100
  cpu_iowait_pct = dio / total * 100
```

## Load

`/proc/loadavg`: fields 1–3 → `load_1m`, `load_5m`, `load_15m`.

## PostgreSQL processes

For each `/proc/[pid]/comm` (trimmed):

- Match `postgres` or prefix `postgres:`

Sum from `/proc/[pid]/status`:

- `VmRSS:` and `VmSize:` values are kB → bytes × 1024

Increment `backend_count` per matching PID.

## Test overrides

| Variable | Purpose |
|----------|---------|
| `PROC_ROOT` | Root for procfs (default `/proc`) |
| `SQL_OPTIMA_COLLECT_ONLY=1` | Emit one JSON payload to stdout and exit |
| `SQL_OPTIMA_SKIP_CPU_SAMPLE=1` | Skip 1s sleep (fixtures use pre-baked stat samples) |

Fixture tests assert JSON fields against expected values in `test/expected.json`.
