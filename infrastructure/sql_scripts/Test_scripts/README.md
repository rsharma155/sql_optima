# TimescaleDB Hypertable Test Scripts

## Purpose
These scripts are designed to simulate a running application environment by populating TimescaleDB hypertables with dummy historical data. This allows testing of:
1.  **Compression**: How TimescaleDB compresses data older than 7 days.
2.  **Retention**: How TimescaleDB automatically drops data older than 90 days (or as configured).
3.  **Performance**: How the application handles large volumes of data (e.g., 3 months of history).

## Growth Analysis: Top 10 Tables
Based on the collector frequencies and data density, the following tables are expected to grow the fastest and are included in the test script:

| Rank | Table Name | Frequency | Growth Driver |
| :--- | :--- | :--- | :--- |
| 1 | `staging.sqlserver_session_request_raw` | 15s | High-frequency pulse data (staging) |
| 2 | `sqlserver_session_snapshots` | 30s | Multi-row per scrape session details |
| 3 | `sqlserver_perf_counters` | 30s | Multiple counters per server per scrape |
| 4 | `sqlserver_blocking_snapshots` | 30s | Detailed blocking chain snapshots |
| 5 | `sqlserver_metrics` | 60s | Core system metrics |
| 6 | `sqlserver_wait_stats_cumulative` | 60s | Cumulative waits for all types |
| 7 | `sqlserver_wait_stats_delta` | 60s | Per-wait-type delta statistics |
| 8 | `sqlserver_query_metrics_v2` | 60s | Heavy text/plan data for queries |
| 9 | `sqlserver_file_io` | 60s | Per-file I/O statistics |
| 10 | `sqlserver_procedure_stats` | 120s | Execution stats for stored procedures |

## Contents
- `generate_historical_data.sql`: A script to insert 1 week or 3 months of dummy data for the top 10 high-growth tables.

## How to Run
... (rest of the file)
### 1. Connect to your PostgreSQL instance
Use `psql` or any SQL client (like DBeaver or pgAdmin).

```bash
psql -h localhost -U postgres -d sql_optima
```

### 2. Execute the script
You can run the script directly:

```bash
\i infrastructure/sql_scripts/Test_scripts/generate_historical_data.sql
```

### 3. Verify the results

#### Check if data was inserted:
```sql
SELECT server_id, MIN(capture_timestamp), MAX(capture_timestamp), COUNT(*) 
FROM sqlserver_metrics 
GROUP BY server_id;
```

#### Check compression status:
```sql
SELECT * FROM chunk_compression_stats('sqlserver_metrics');
```

#### Check hypertable details:
```sql
SELECT * FROM timescaledb_information.hypertables WHERE hypertable_name = 'sqlserver_metrics';
```

## Detailed Instructions for Performance Testing
1.  **1-Week Test**: Run the script as-is. It will generate data for the last 7 days. Observe the UI dashboards to see how it renders.
2.  **3-Month Test**: Modify the `num_days` variable in the SQL script to `90`. This will generate a larger volume of data. Check the query response times in the application to identify any bottlenecks.
3.  **Retention Test**: Insert data with a timestamp older than 90 days (e.g., `now() - INTERVAL '95 days'`). If the retention policy is active, TimescaleDB will eventually drop this data. You can force a retention run with:
    ```sql
    SELECT drop_chunks('sqlserver_metrics', older_than => INTERVAL '90 days');
    ```
