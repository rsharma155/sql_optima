# TimescaleDB Growth Analysis & Simulation Plan

This document explains the strategy used in the `generate_historical_data.sql` script to simulate a high-load production environment over 1 week and 3 months.

## 1. Top 10 High-Growth Hypertables
The following tables were identified as the primary drivers of storage growth in the SQL Optima platform. The simulation script prioritizes these to ensure that performance bottlenecks, compression efficiency, and retention policies are thoroughly tested.

| Rank | Table Name | Frequency | Rows/Day (Simulated) | Why it grows fast |
| :--- | :--- | :--- | :--- | :--- |
| **1** | `staging.sqlserver_session_request_raw` | 15s | ~5,760 | Highest frequency "pulse" data used for real-time dashboards. |
| **2** | `sqlserver_session_snapshots` | 30s | ~28,800 | Captures 10 concurrent sessions every 30s. Multi-row per scrape. |
| **3** | `sqlserver_perf_counters` | 30s | ~11,520 | Tracks 4 different performance counters every 30s. |
| **4** | `sqlserver_blocking_snapshots` | 30s | ~150-300 | High-frequency detail during blocking events (simulated at 5% occurrence). |
| **5** | `sqlserver_metrics` | 60s | ~1,440 | Core system metrics (CPU, RAM, Disk) captured every minute. |
| **6** | `sqlserver_wait_stats_cumulative` | 60s | ~7,200 | Tracks 5 major wait categories every minute. |
| **7** | `sqlserver_wait_stats_delta` | 60s | ~7,200 | Stores deltas for the same 5 categories every minute. |
| **8** | `sqlserver_query_metrics_v2` | 60s | ~1,440 | Includes large text fields (SQL statements) and plan handles. |
| **9** | `sqlserver_file_io` | 60s | ~1,440 | Per-file I/O metrics for active databases. |
| **10** | `sqlserver_procedure_stats` | 120s | ~720 | Execution stats for stored procedures. |

## 2. Simulation Logic
The script uses `generate_series` and `random()` to create a realistic distribution of metrics:

- **Data Density**: We mimic the actual collector frequencies defined in `optima_collector_configs`.
- **Randomized Metrics**: CPU values fluctuate between 10-60%, Memory between 20-100%, and I/O latency between 5-30ms to ensure dashboards show realistic trends.
- **Event Simulation**:
    - **Deadlocks**: Simulated to occur in ~1% of metric captures.
    - **Blocking**: Simulated as intermittent spikes where a session is blocked by another.
    - **Query Diversity**: Uses randomized Query Hashes to simulate a realistic plan cache.

## 3. Testing Scenarios

### Scenario A: The 1-Week Test (Compression)
- **Goal**: Verify that the **Compression Policy** (set to 7 days) correctly compresses old chunks.
- **Expectation**: After running the script for 7 days, you should see the earliest data chunks being compressed. This reduces disk usage by ~90% while keeping data queryable.
- **Validation**:
  ```sql
  SELECT * FROM chunk_compression_stats('sqlserver_metrics');
  ```

### Scenario B: The 3-Month Test (Performance & Retention)
- **Goal**: Test UI responsiveness with millions of rows and verify the **Retention Policy** (set to 90 days).
- **Expectation**: With 90 days of data, some tables (like `sqlserver_session_snapshots`) will exceed 2 million rows. This is the ultimate test for index efficiency and TimescaleDB's ability to prune data via chunk dropping.
- **Validation**:
  ```sql
  -- Check row counts
  SELECT count(*) FROM sqlserver_session_snapshots;
  -- Check if chunks older than 90 days were dropped
  SELECT show_chunks('sqlserver_metrics', older_than => INTERVAL '90 days');
  ```

## 4. How to Use
1.  Read the `README.md` for connection instructions.
2.  Open `generate_historical_data.sql`.
3.  Set `v_num_days := 7` for a quick check or `v_num_days := 90` for a stress test.
4.  Run the script via `psql`.
