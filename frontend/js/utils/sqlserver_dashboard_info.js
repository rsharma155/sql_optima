/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Comprehensive information data for SQL Server dashboards.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

(function () {
    'use strict';

    window.sqlserverDashboardInfo = {
        "Instance Dashboard": {
            description: "The primary oversight view for a SQL Server instance. Provides a high-level health score and immediate visibility into critical resource pressure across CPU, memory, I/O, and concurrency. This is the 'first look' dashboard — a DBA should be able to determine within 30 seconds whether the instance is healthy or in crisis. It consolidates signals from multiple Dynamic Management Views (DMVs) and Performance Counters to provide a 360-degree view of instance performance.",
            metrics: {
                "CPU Load": {
                    title: "CPU Load (%)",
                    text: "**What it is:** The percentage of total CPU capacity currently consumed by the SQL Server process, sampled from `sys.dm_os_ring_buffers` or `sys.dm_os_schedulers`.\n\n**Why it matters:** CPU is a shared, finite resource. Sustained high CPU is often the first visible symptom of a query regression, a missing index, or a sudden increase in workload volume.\n\n**Thresholds:**\n- ✅ Healthy: < 60% sustained\n- ⚠️ Warning: 60–80% sustained for > 5 minutes\n- 🔴 Critical: > 80% sustained or > 95% at any point"
                },
                "Memory Usage": {
                    title: "Memory Usage (%)",
                    text: "**What it is:** The percentage of the SQL Server's configured `max server memory` that is currently allocated.\n\n**Why it matters:** SQL Server is designed to consume most of the memory it's given. The concern is whether memory pressure is causing Page Life Expectancy (PLE) to drop or memory grants to pend. If the OS is reclaiming memory from SQL Server, query performance degrades sharply."
                },
                "TPS": {
                    title: "TPS (Transactions Per Second)",
                    text: "**What it is:** The rate of committed transactions per second. Represents overall database throughput.\n\n**Why it matters:** TPS is the heartbeat of the database. A sudden drop while load is constant indicates a bottleneck (blocking, I/O saturation, or CPU exhaustion)."
                },
                "Active Users": {
                    title: "Active Users (Sessions)",
                    text: "**What it is:** Count of non-sleeping, non-system sessions currently connected to the instance.\n\n**Why it matters:** Tracks real concurrency. Distinguishes a sudden CPU spike caused by a single runaway query versus a connection surge from an application retry storm."
                },
                "Blocked Sessions": {
                    title: "Blocked Sessions",
                    text: "**What it is:** The count of sessions currently waiting to acquire a lock held by another session.\n\n**Why it matters:** Even a single blocking chain can cascade. This causes timeouts, connection pool exhaustion, and user-facing errors. It is the primary cause of 'application hangs'."
                },
                "Health Score": {
                    title: "Health Score (0–100%)",
                    text: "**What it is:** A composite index calculated from weighted sub-scores across CPU, memory, I/O, blocking, and wait states.\n\n**Why it matters:** Provides a single number for at-a-glance assessment. 100% = all systems healthy."
                },
                "PLE": {
                    title: "Page Life Expectancy (PLE)",
                    text: "**What it is:** Number of seconds a data page stays in the buffer pool without being referenced.\n\n**Why it matters:** A key indicator of memory pressure. If PLE drops below 300-1000s, SQL Server is frequently evicting pages to make room for new ones, causing excessive disk I/O."
                }
            }
        },
        "Workload Analytics": {
            description: "Detailed view of query execution efficiency, application demand, and SQLOS scheduler performance. Helps identify who and what is consuming resources. This dashboard breaks down workload intensity into CPU compute time, I/O volume, and throughput, allowing DBAs to distinguish between 'more work' (higher execution counts) and 'less efficient work' (higher resource consumption per execution).",
            metrics: {
                "CPU Sec": {
                    title: "CPU Time (ms)",
                    text: "**What it is:** Total CPU time consumed by queries during the interval.\n\n**Why it matters:** Tracks the actual compute cost of the workload. Useful for identifying trends in workload growth and the impact of code deployments."
                },
                "Throughput": {
                    title: "Total Executions",
                    text: "**What it is:** Total count of query executions.\n\n**Why it matters:** Measures application demand. Helps correlate resource spikes with user activity increases."
                },
                "IO Volume": {
                    title: "Logical Reads",
                    text: "**What it is:** The number of 8KB pages read from the buffer cache.\n\n**Why it matters:** High logical reads are a proxy for I/O pressure. Even if the data is in memory, high read volume consumes CPU cycles and can indicate missing indexes or poorly written queries."
                }
            }
        },
        "Blocking Monitor": {
            description: "Monitors lock contention, identifies root blockers, and visualizes blocking trees. Critical for resolving performance 'stalls' and identifying uncommitted transactions. Even a single blocking chain can cascade across an entire application if the blocker holds a table-level lock. This dashboard surfaces live chains and provides deadlocks forensics for the last 24 hours.",
            metrics: {
                "Active Blocked Sessions": {
                    title: "Active Blocked Sessions",
                    text: "**What it is:** Count of sessions currently waiting for a lock held by another session.\n\n**Why it matters:** These users are stalled and their applications may be timing out. Persistent blocking indicates systemic resource contention or long-running transactions."
                },
                "Deadlock Count": {
                    title: "Deadlock Count (24h)",
                    text: "**What it is:** Number of deadlocks detected by SQL Server in the last 24 hours. A deadlock is a circular dependency where two or more sessions are waiting for each other.\n\n**Why it matters:** SQL Server automatically kills one session to resolve a deadlock, resulting in application errors. High counts indicate problematic application access patterns or missing indexes."
                }
            }
        },
        "Real-Time Diagnostics": {
            description: "Live, high-frequency view of executing sessions, file I/O latency, and current wait states. The 'Task Manager' for SQL Server. This dashboard is intended for active troubleshooting of immediate performance issues, showing exactly what SQL Server is waiting for at this moment—whether it's CPU, disk I/O, or network delivery.",
            metrics: {
                "Batch Requests/sec": {
                    title: "Batch Requests/sec",
                    text: "**What it is:** The number of T-SQL command batches received per second.\n\n**Why it matters:** A primary measure of SQL Server throughput. Compare against baseline to detect surges in application activity or drops caused by bottlenecks."
                }
            }
        },
        "Memory Analyzer": {
            description: "Deep-dive analysis of memory clerk allocation, buffer pool health, and workspace memory grants. Essential for diagnosing 'Internal' SQL Server memory pressure. While PLE measures the health of the buffer cache, this dashboard investigates how memory is distributed among internal components like the plan cache, connection memory, and sort/hash grants.",
            metrics: {
                "Grants Pending": {
                    title: "Memory Grants Pending",
                    text: "**What it is:** Number of processes waiting for workspace memory grants before they can execute sort or hash operations.\n\n**Why it matters:** A non-zero value indicates severe memory pressure or inefficient queries with large memory requirements. Queries in this state are effectively stalled until memory is released."
                },
                "Available Headroom": {
                    title: "Available Headroom (MB)",
                    text: "**What it is:** The amount of physical memory available to the OS before paging starts.\n\n**Why it matters:** SQL Server needs the OS to be healthy to avoid 'External' memory pressure. If headroom drops to zero, the OS may force SQL Server to release memory, tanking performance."
                }
            }
        },
        "Enterprise Metrics": {
            description: "Advanced telemetry focusing on long-term wait stats trends, throughput, and subsystem health snapshots. This dashboard provides a more granular view of internal engine health than the main dashboard, including scheduler queue lengths and latch contention rates.",
            metrics: {
                "Runnable Tasks": {
                    title: "Runnable Tasks",
                    text: "**What it is:** Number of tasks waiting for a CPU core to become available.\n\n**Why it matters:** High values indicate CPU saturation. Queries are ready to run but are 'queued' for a processor. This is often caused by too many parallel queries or insufficient CPU cores."
                },
                "Latch Waits/sec": {
                    title: "Latch Waits/sec",
                    text: "**What it is:** Rate of internal latch waits. Latches are lightweight synchronization objects that protect internal memory structures.\n\n**Why it matters:** High latch waits indicate internal contention, such as Buffer Pool contention or TempDB allocation bottlenecks."
                }
            }
        },
        "Storage & Maintenance": {
            description: "Tracks database size, growth trends, and evaluates index efficiency and fragmentation. Maintaining healthy indexes is the single most effective way to ensure consistent query performance. Fragmented indexes cause SQL Server to perform extra I/O to read what should be sequential data. This dashboard identifies candidate indexes for reorganization or rebuilding, and identifies 'wasted' indexes that are updated often but never read.",
            metrics: {
                "Read:Write Ratio": {
                    title: "Read:Write Ratio",
                    text: "**What it is:** Ratio of index reads (seeks/scans) to writes (inserts/updates/deletes) that must maintain the index.\n\n**Why it matters:** An index with a ratio below 1.0 (more writes than reads) costs more to maintain than it benefits queries and should be evaluated for removal."
                },
                "Fragmentation": {
                    title: "Fragmentation (%)",
                    text: "**What it is:** Percentage of out-of-order pages in an index. Fragmentation occurs when page splits force data out of logical order.\n\n**Why it matters:** High fragmentation causes extra I/O during scans. REORGANIZE at 10-30%, REBUILD at >30% for large indexes."
                }
            }
        },
        "Query Analysis": {
            description: "Deep analysis of Query Store data to identify regressions, plan instability, and high-impact queries. Query Store provides a persistent history of query execution plans and performance statistics. This dashboard makes that data actionable, surfacing queries that were fast yesterday but are slow today, or queries that are oscillating between multiple execution plans.",
            metrics: {
                "Plan Changes": {
                    title: "Plan Changes (24h)",
                    text: "**What it is:** Count of query execution plan changes recorded by Query Store in the last 24 hours.\n\n**Why it matters:** Sudden spikes indicate mass plan invalidation, which can cause compilation surges and transient CPU spikes."
                },
                "Regressions": {
                    title: "Regressions (24h)",
                    text: "**What it is:** Count of queries whose performance has significantly degraded compared to their prior baseline.\n\n**Why it matters:** These are causing active user pain and should be optimized first. Common causes include parameter sniffing or stale statistics."
                }
            }
        }
    };

    /**
     * Shows a modal with information about a specific metric or section.
     */
    window.sqlserverShowInfo = function(section, metric) {
        const data = window.sqlserverDashboardInfo[section];
        if (!data) return;

        let title, text;
        if (metric && data.metrics[metric]) {
            title = data.metrics[metric].title;
            text = data.metrics[metric].text;
        } else {
            title = section + " Overview";
            text = data.description;
        }

        if (window.showAppInfoModal) {
            window.showAppInfoModal(title, text);
        }
    };

    /**
     * Shows a complete detailed info modal for the entire dashboard with scroll.
     */
    window.sqlserverShowDashboardDetail = function(dashboardName) {
        const data = window.sqlserverDashboardInfo[dashboardName];
        if (!data) return;

        let fullText = `# ${dashboardName}\n\n${data.description}\n\n---\n\n`;
        fullText += `## Dashboard Metrics & Components\n\n`;
        
        for (const m in data.metrics) {
            const metric = data.metrics[m];
            fullText += `### ${metric.title}\n${metric.text}\n\n`;
        }

        if (window.showAppInfoModal) {
            window.showAppInfoModal(dashboardName + " - Complete Detailed Info", fullText, { width: '850px', maxHeight: '90vh' });
        }
    };
})();
