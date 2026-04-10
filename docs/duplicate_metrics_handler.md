# Duplicate Metrics Handler

This document describes how duplicate records are prevented when collecting and storing metrics data in the TimescaleDB database.

## Overview

The metrics collection system uses a **delta detection approach** to prevent inserting duplicate records. Each metrics table has a corresponding state tracking mechanism in the `TimescaleLogger` struct that stores the previous values for comparison.

## State Tracking Mechanism

The `TimescaleLogger` struct maintains in-memory state for each metric type:

```go
type TimescaleLogger struct {
    pool *pgxpool.Pool
    // State tracking for delta inserts
    prevDiskHistory     map[string]map[string]interface{}
    prevJobDetails      map[string]map[string]interface{}
    prevJobFailures     map[string]string
    prevLongRunningHash map[string]int64
    prevMemoryPLE       float64
    prevWaitHistory     map[string]map[string]float64
    prevQueryStoreStats map[string]int64
    prevSchedulerStats  map[string]int64
}
```

## Delta Detection by Table

### 1. sqlserver_disk_history
- **Key**: Database name
- **Fields Compared**: `data_mb`, `log_mb`, `free_mb`
- **Logic**: Inserts only when any size value changes

```go
if prevDataMB == dataMB && prevLogMB == logMB && prevFreeMB == freeMB {
    shouldInsert = false
}
```

### 2. sqlserver_job_details
- **Key**: Job name
- **Fields Compared**: `enabled`, `owner`, `current_status`, `last_run_date`, `last_run_time`, `last_run_status`
- **Logic**: Inserts only when job properties change

### 3. sqlserver_job_failures
- **Key**: `job_name + "||" + step_name + "||" + run_date + "||" + run_time`
- **Fields Compared**: Error message
- **Logic**: Inserts only new failures not seen before

### 4. sqlserver_long_running_queries
- **Key**: `session_id + "_" + request_id`
- **Fields Compared**: `total_elapsed_time_ms`
- **Logic**: Inserts only new sessions or sessions with changed elapsed time

### 5. sqlserver_memory_history
- **Key**: Single value (no key needed)
- **Fields Compared**: `page_life_expectancy` (PLE)
- **Logic**: Inserts only when PLE value changes

### 6. sqlserver_wait_history
- **Key**: Wait type
- **Fields Compared**: `disk_read_ms_per_sec`, `blocking_ms_per_sec`, `parallelism_ms_per_sec`, `other_ms_per_sec`
- **Logic**: Inserts only when wait stats change for any wait type

### 7. sqlserver_query_store_stats
- **Key**: `database_name + "||" + query_hash`
- **Fields Compared**: `executions`
- **Logic**: Inserts only when execution count changes

### 8. sqlserver_cpu_scheduler_stats
- **Fingerprint**: FNV-1a hash of all collected scheduler/memory/warning fields (same snapshot as the INSERT row, excluding timestamp)
- **Logic**: Inserts only when the fingerprint differs from the previous poll for that instance (in-memory; resets on process restart)

### 9. sqlserver_top_queries
- **Logic**: Handled in the Go agent's `FetchTopCPUQueries` function with the "Noise Filter"
- **Criteria**: 
  - Delta CPU > 10ms OR
  - Delta Executions > 5

```go
// The DBA Noise Filter:
// Keep if it's heavy (>10ms CPU) OR high-frequency (>5 executions in 15s)
if deltaExecs > 0 {
    if deltaCPU > 10.0 || deltaExecs > 5 {
        // Insert delta record
    }
}
```

## Implementation Details

### Insert Flow
1. Collect new data from SQL Server
2. Compare with previous state stored in memory
3. If changed, insert new record and update state
4. If not changed, skip insertion and log

### State Persistence
- State is stored in memory only (not persisted to database)
- State is lost on server restart
- On restart, first collection may insert duplicates until state is rebuilt

## Benefits

1. **Reduced Database Storage**: Prevents duplicate records
2. **Improved Query Performance**: Less data to scan
3. **Accurate Delta Tracking**: Only meaningful changes are recorded
4. **Noise Filtering**: Background queries filtered out in Top Queries

## Limitations

1. **Memory Usage**: State maps consume memory proportional to metric diversity
2. **No Persistence**: State lost on restart (first cycle may dupe)
3. **Single Instance**: Each collector instance maintains its own state

## Enterprise DMV snapshots (Enterprise Metrics / dashboard Phase 2)

**Collector**: `StartEnterpriseMetricsCollector` in `query_store_worker.go` (default interval **120s**; set `ENTERPRISE_METRICS_INTERVAL_SEC` for 30–600s).

**Tables** (batch fingerprint per instance; skip the whole scrape if unchanged vs last successful insert):

- `sqlserver_latch_waits`
- `sqlserver_spinlock_stats`
- `sqlserver_memory_clerks`
- `sqlserver_procedure_stats`
- `sqlserver_file_io_latency`
- `sqlserver_waits_delta` (category-total rows only; same pattern as other batches)

State: `TimescaleLogger.prevEnterpriseBatchHash` (in-memory; cleared on restart).

`sqlserver_waits_delta` is still computed from **cumulative** wait stats via `prevWaitHistory` + `ComputeWaitDeltas`; the new check avoids writing another identical **category totals** row when nothing moved in that window.

## Tables Without Delta Detection

The following tables insert on every collection cycle (no duplicate prevention):
- sqlserver_cpu_history
- sqlserver_connection_history
- sqlserver_lock_history
- sqlserver_job_metrics
- sqlserver_job_schedules
- sqlserver_cpu_last_collected (UPSERT based)

## Future Improvements

1. Persist state to database for surviving restarts
2. Add delta detection for remaining tables
3. Implement distributed state for multi-instance collectors
