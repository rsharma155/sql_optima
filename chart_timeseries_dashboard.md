# Candidate Dashboards for Chart Zoom Feature

This document lists all SQL Server and PostgreSQL dashboard pages in the SQL Optima UI that contain time-series charts. These pages are candidates for implementing the new **Zoom-In / Zoom-Out** feature with global time synchronization.

## SQL Server Dashboards

### 1. Health Dashboard V2 (Instant Triage)
*   **File:** `frontend/js/pages/sqlserver_HealthDashboardV2.js`
*   **Charts:**
    *   **Wait Trends:** Stacked area chart showing CPU, IO, Memory, Locking, and Parallelism waits.
    *   **Disk I/O Latency:** Line chart showing Read, Write, and Log latency with IOPS on secondary axis.
    *   **Throughput:** Line chart showing Batch Requests, Connections, and Logins/sec.
*   **Status:** Partially Implemented (Zoom enabled, synchronization logic added).

### 2. Main Dashboard (Instance Overview)
*   **File:** `frontend/js/pages/sqlserver_Dashboard.js`
*   **Charts:**
    *   **System Resources (CPU):** SQL CPU vs System Idle.
    *   **Batch Requests/sec:** Throughput trend.
    *   **Page Life Expectancy (PLE):** Memory health indicator.
    *   **Buffer Cache Hit %:** Memory efficiency trend.
*   **Status:** Partially Implemented (CPU chart updated to Time Scale).

### 3. CPU & Scheduler Dashboard
*   **File:** `frontend/js/pages/sqlserver_CpuDashboard.js`
*   **Charts:**
    *   **CPU Scheduler History:** Workers, Runnable Tasks, and Work Queue trends.
    *   **Memory Utilization:** Physical memory usage percentage over time.

### 4. Wait Stats V2 (Deep Dive)
*   **File:** `frontend/js/pages/sqlserver_WaitStatsV2.js`
*   **Charts:**
    *   **Wait Trends (Hourly/Daily):** Historical wait category distributions.
    *   **CPU Pressure:** Scheduler yield and preemptive wait trends.

### 5. Enterprise Metrics
*   **File:** `frontend/js/pages/sqlserver_EnterpriseMetrics.js`
*   **Charts:**
    *   **Wait Stats Trend:** Aggregate wait times.
    *   **Throughput Metrics:** Transactions, Batches, and Logins.
    *   **File I/O Performance:** Latency and throughput per file/drive.
    *   **Plan Cache Health:** Hit ratios and object counts.
    *   **Memory Internals:** Buffer pool and clerk distributions.

### 6. High Availability (HA) Dashboard
*   **File:** `frontend/js/pages/sqlserver_HADashboard.js`
*   **Charts:**
    *   **Replication Lag:** Send and Redo queue sizes.
    *   **Throughput:** Data transfer rates between replicas.

### 7. Locks & Blocking Dashboard
*   **File:** `frontend/js/pages/sqlserver_LocksDashboard.js`
*   **Charts:**
    *   **Blocking Trend:** Historical count of blocked sessions.
    *   **Deadlocks Trend:** Frequency of deadlock events.

---

## PostgreSQL Dashboards

### 1. Global Estate & Overview
*   **File:** `frontend/js/pages/overview.js`
*   **Charts:**
    *   **Database Throughput:** TPS (Transactions) and QPS (Queries) per second.
    *   **Host Resource Trends:** CPU and Memory usage across the estate.
    *   **Replication Health:** WAL lag and replay latency trends.

### 2. CPU & Query Performance
*   **File:** `frontend/js/pages/pg_cpu.js`
*   **Charts:**
    *   **Execution Load:** Total execution time (s) vs Avg Latency (ms).
    *   **Cache Hit Trend:** Shared buffer efficiency over time.

### 3. Wait Events & AAS (Active Average Sessions)
*   **File:** `frontend/js/pages/pg_waits.js`
*   **Charts:**
    *   **AAS Load:** Active vs Waiting sessions (DB Load).
    *   **Wait Trends:** Specific wait event category trends.
    *   **Session State Trends:** Active, Idle, and Idle-in-Transaction counts.

### 4. Storage & Index Health
*   **File:** `frontend/js/pages/PgStorage.js`
*   **Charts:**
    *   **Table/Index Growth:** Historical size metrics for tables and indexes.
    *   **Index Efficiency Trend:** Scans vs Size over time.

### 5. Backup, DR & WAL
*   **File:** `frontend/js/pages/pg_backup_dr.js`
*   **Charts:**
    *   **WAL Generation Rate:** Bytes per second/minute.
    *   **Replica Lag:** Physical and logical replication delay.

---

## Implementation Requirements for Each Page

To enable the zoom feature on these pages, the following steps are required:
1.  **Time Scale Conversion**: Ensure the x-axis is configured as `type: 'time'`.
2.  **Data Format Update**: Map dataset values to `{x: date, y: value}` objects.
3.  **Zoom Options**: Include the `getChartZoomOptions()` configuration in the chart initialization.
4.  **Synchronization Callback**: Implement or wire the `onZoomComplete` event to call `window.applyTimeRangeFromChart()`.
5.  **Surgical Refresh**: Ensure `window.refreshDashboardData()` is implemented to reload metrics without a full page navigation.
