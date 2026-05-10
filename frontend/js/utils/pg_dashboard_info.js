/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Comprehensive information data for PostgreSQL dashboards.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

(function () {
    'use strict';

    window.pgDashboardInfo = {
        "Control Center": {
            description: "The primary command center providing a high-level, real-time overview of database health, throughput, concurrency, and operational diagnostics. This is the 'first look' dashboard — a DBA should be able to determine within 30 seconds whether the instance is healthy or in crisis. It consolidates signals from pg_stat_activity, pg_stat_bgwriter, and pg_stat_database to provide a 360-degree view of instance performance.",
            metrics: {
                "Health Score": {
                    title: "Health Score (0–100%)",
                    text: "**What it is:** A composite index calculated from weighted sub-scores across session health, cache efficiency, wait event distribution, replication lag, and bloat risk.\n\n**Why it matters:** Provides a single actionable number for quick triage. It is especially valuable during on-call rotations where the responder needs to assess severity before committing to a deep investigation."
                },
                "Active Sessions": {
                    title: "Active Sessions",
                    text: "**What it is:** Count of connections with `state = 'active'` in `pg_stat_activity`. These are sessions currently executing a query.\n\n**Why it matters:** Active session count is the real-time concurrency gauge. A sudden spike without a corresponding TPS increase means queries are running slower (possible contention or regression)."
                },
                "Waiting Sessions": {
                    title: "Waiting Sessions",
                    text: "**What it is:** Count of connections with a non-null `wait_event` in `pg_stat_activity`. These sessions are executing but stalled on a resource — a lock, I/O, a buffer pin, etc.\n\n**Why it matters:** Waiting sessions represent blocked concurrency. A high waiting-to-active ratio is the clearest signal of a systemic bottleneck."
                },
                "Blocked Sessions": {
                    title: "Blocked Sessions",
                    text: "**What it is:** Count of sessions waiting specifically on a PostgreSQL lock held by another session.\n\n**Why it matters:** Readers never block writers in PostgreSQL's MVCC design. When blocking occurs, it is for explicit DDL locks, LOCK TABLE commands, or row-level conflicts. It often indicates an application design problem."
                },
                "TPS": {
                    title: "TPS (Transactions Per Second)",
                    text: "**What it is:** The rate of committed transactions per second across the instance.\n\n**Why it matters:** TPS is the heartbeat of the database. A sudden drop while active users remain constant signals a bottleneck."
                },
                "Total Connections": {
                    title: "Total Connections",
                    text: "**What it is:** Total count of all connections regardless of state. Compared against `max_connections`.\n\n**Why it matters:** PostgreSQL's `max_connections` is a hard ceiling. When reached, new connection attempts fail immediately (FATAL). Each connection is a separate OS process consuming memory."
                },
                "Replica Lag": {
                    title: "Replica Lag",
                    text: "**What it is:** The maximum replication lag across all connected standby servers, in bytes or seconds.\n\n**Why it matters:** Determines the data currency of reads routed to standbys and the potential data loss window if the primary fails."
                },
                "Cache Hit Ratio": {
                    title: "Cache Hit Ratio (%)",
                    text: "**What it is:** Percentage of data blocks found in the PostgreSQL buffer cache vs. those that had to be read from disk.\n\n**Why it matters:** Reading from RAM is orders of magnitude faster than disk. A low ratio indicates memory pressure or inefficient scan patterns."
                }
            }
        },
        "Enterprise Monitor": {
            description: "Deep-dive metrics for PostgreSQL internals including background writer, checkpointing, and WAL archiving health. This dashboard surfaces bottlenecks in the core engine processes that manage durability and memory-to-disk synchronization.",
            metrics: {
                "Checkpoint Ratio": {
                    title: "Timed vs Requested Checkpoints",
                    text: "**What it is:** Ratio of checkpoints triggered by time (`checkpoint_timeout`) vs. those triggered by data volume (`max_wal_size`).\n\n**Why it matters:** In a well-tuned system, most checkpoints should be timed. Frequent requested checkpoints indicate `max_wal_size` is too small, causing I/O spikes."
                },
                "BGWriter Halts": {
                    title: "BGWriter Halts",
                    text: "**What it is:** Frequency of the background writer process being forced to halt due to high workload pressure.\n\n**Why it matters:** High halts indicate that the background writer cannot keep up with cleaning dirty buffers, forcing backend processes to do it themselves, which slows down queries."
                },
                "Archive Success": {
                    title: "Archive Success %",
                    text: "**What it is:** Percentage of WAL files successfully archived.\n\n**Why it matters:** Foundation of PITR (Point-In-Time Recovery). A single missing WAL file makes all subsequent archives useless for recovery."
                }
            }
        },
        "CPU Health": {
            description: "Monitors CPU consumption by the database engine and identifies high-CPU queries. Tracks host vs. Postgres load, saturation, and thread activity.",
            metrics: {
                "Postgres CPU %": {
                    title: "Postgres CPU %",
                    text: "**What it is:** Percentage of total CPU capacity consumed specifically by PostgreSQL processes.\n\n**Why it matters:** High usage across many connections suggests a need for more cores or optimization of top queries."
                },
                "CPU per Connection": {
                    title: "CPU per Connection",
                    text: "**What it is:** Average CPU consumption allocated to each individual connection.\n\n**Why it matters:** Helps identify if a few heavy sessions are starving the rest of the instance."
                }
            }
        },
        "Memory Intelligence": {
            description: "Deep-dive analysis of memory allocation, resident set size (RSS), cache efficiency, and predictive analytics. PostgreSQL's process-per-connection model means memory management is a balance between `shared_buffers` and the sum of `work_mem` used by active connections.",
            metrics: {
                "OS Used %": {
                    title: "OS Used % (Host Memory)",
                    text: "**What it is:** Percentage of host RAM currently in use by all processes.\n\n**Why it matters:** If the OS runs out of memory, it may trigger the OOM killer, which often targets the largest process (PostgreSQL)."
                },
                "Postgres RSS": {
                    title: "Postgres RSS (Resident Set Size)",
                    text: "**What it is:** The actual amount of physical RAM consumed by the PostgreSQL processes.\n\n**Why it matters:** Tracks the real memory footprint of the database engine."
                },
                "Swap Activity": {
                    title: "Swap Activity",
                    text: "**What it is:** Rate of memory pages being moved to or from disk swap space.\n\n**Why it matters:** Swapping is orders of magnitude slower than RAM. Sustained swap activity will tank database performance."
                },
                "Temp Spill Rate": {
                    title: "Temp Spill Rate",
                    text: "**What it is:** Rate at which sort and hash operations exceed `work_mem` and spill to disk.\n\n**Why it matters:** Spilling to disk significantly increases query latency. High rates suggest `work_mem` should be tuned."
                }
            }
        },
        "Waits & Sessions": {
            description: "Active backend session monitoring, including state, duration, and client details. Visualizes what sessions are executing vs. what they are waiting for using `pg_stat_activity` data.",
            metrics: {
                "Longest Active": {
                    title: "Longest Active Session",
                    text: "**What it is:** The duration and PID of the longest-running query currently active.\n\n**Why it matters:** Identifies runaway queries or long-running batch jobs that may be consuming resources or holding locks."
                },
                "Longest Idle Txn": {
                    title: "Longest Idle-in-Transaction",
                    text: "**What it is:** The duration and PID of the session that has been in 'idle in transaction' state for the longest time.\n\n**Why it matters:** These are highly dangerous as they prevent vacuum from reclaiming dead tuples and can block DDL."
                }
            }
        },
        "Locks & Blocking": {
            description: "Monitors lock contention, identifies root blockers, and visualizes blocking trees. Critical for diagnosing application-level concurrency issues. PostgreSQL's MVCC design means readers never block writers and vice versa for most operations; when blocking *does* occur, it often indicates explicit DDL locks, LOCK TABLE commands, or row-level write-write conflicts. This dashboard helps identify the 'Idle in Transaction' pattern where a session holds locks indefinitely, preventing autovacuum from cleaning dead tuples.",
            metrics: {
                "Active Blocking Sessions": {
                    title: "Active Blocking Sessions",
                    text: "**What it is:** Count of sessions currently blocking other sessions by holding a lock that another backend is waiting for.\n\n**Why it matters:** Blocking in PostgreSQL often signals a design issue. Persistent blocking leads to application timeouts and connection pool exhaustion."
                },
                "Idle-in-Transaction Risk": {
                    title: "Idle-in-Transaction Risk",
                    text: "**What it is:** Count of sessions in 'idle in transaction' state that are also holding locks.\n\n**Why it matters:** This is the most dangerous blocking pattern in PostgreSQL. These sessions hold locks indefinitely, preventing autovacuum from reclaiming space (leading to bloat) and potentially blocking DDL operations."
                }
            }
        },
        "Query Performance": {
            description: "Deep analysis of query performance trends derived from the `pg_stat_statements` extension. This dashboard provides a ranked view of the most resource-intensive queries by total time, I/O, or frequency. It enables DBAs to transition from 'reactive firefighting' to 'proactive optimization' by identifying the 10% of queries responsible for 90% of the database load.",
            metrics: {
                "QPS": {
                    title: "QPS (Queries Per Second)",
                    text: "**What it is:** Total number of queries executed per second across the instance.\n\n**Why it matters:** Measures the overall query throughput. Significant deviations from the baseline can indicate application bugs, bot traffic, or systemic slowdowns."
                },
                "Latency Percentiles": {
                    title: "Latency Percentiles (p95, p99)",
                    text: "**What it is:** The time under which 95% or 99% of queries complete.\n\n**Why it matters:** Represents the tail-end latency experienced by users. Essential for identifying performance regressions that might be masked by 'average' execution time."
                }
            }
        },
        "EXPLAIN Plan Analyzer": {
            description: "Visualizes and analyzes PostgreSQL execution plans to identify inefficient operators, missing indexes, and cardinality estimation errors. By pasting a JSON-formatted plan (generated via `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)`), users can see a Sankey-style tree showing the flow of data and a heuristic-driven optimization report that flags 'plan smells' like sequential scans on large tables or expensive nested loops.",
            metrics: {
                "Plan Map": {
                    title: "Plan Map (Sankey-Style Tree)",
                    text: "**What it is:** A visual representation of the execution plan hierarchy, showing the flow of data between operators. The width of the connections often represents the volume of rows or time consumed.\n\n**Why it matters:** Makes complex, nested plans easy to navigate and highlights which specific branches of the query are the primary bottlenecks."
                },
                "Optimization Report": {
                    title: "Optimization Report",
                    text: "**What it is:** A rule-based analysis that flags common plan inefficiencies such as sequential scans on large tables, excessive disk spilling, or large row-count discrepancies (estimate vs. actual).\n\n**Why it matters:** Provides immediate, actionable guidance on how to improve query performance without requiring deep expertise in PostgreSQL plan internals."
                }
            }
        },
        "Storage & Maintenance": {
            description: "Tracks database size, growth trends, and monitors MVCC health (bloat and autovacuum performance). PostgreSQL uses MVCC to manage concurrency, which creates 'dead tuples' whenever rows are updated or deleted. These must be reclaimed by the autovacuum process. If autovacuum falls behind, tables and indexes 'bloat', wasting disk space and slowing down sequential scans. This dashboard surfaces the tables with the highest risk of bloat or Transaction ID (XID) wraparound.",
            metrics: {
                "7-Day Growth (%)": {
                    title: "7-Day Growth (%)",
                    text: "**What it is:** Percentage increase in total database size over the last 7 days.\n\n**Why it matters:** Essential for capacity planning and identifying runaway logging or unexpected bulk data loads."
                },
                "Worst Dead Tuple %": {
                    title: "Worst Dead Tuple %",
                    text: "**What it is:** The highest percentage of dead tuples (bloat) across all monitored tables.\n\n**Why it matters:** High bloat indicates that autovacuum is not keeping up. This wastes disk space and increases the I/O required for scans, as PostgreSQL must still read the dead versions during a table scan."
                },
                "XID Wraparound Risk": {
                    title: "XID Wraparound Risk",
                    text: "**What it is:** Distance to the hard limit of 2.1 billion transaction IDs. PostgreSQL will force a shutdown if this limit is approached without a 'freeze' vacuum to prevent data corruption.\n\n**Why it matters:** This is a critical availability metric. DBAs must ensure that `VACUUM FREEZE` operations occur regularly enough to keep the 'Age' of the oldest database well below the threshold."
                }
            }
        },
        "Backup & DR": {
            description: "Monitors the safety, durability, and high-availability status of the instance—specifically WAL archiving health, streaming replication status, and replication slot risk. The archive is the foundation of Point-In-Time Recovery (PITR); a single missing WAL file makes the entire subsequent archive useless for recovery. This dashboard ensures that your data is being replicated safely and that no replication slots are holding back WAL, which could fill the disk and crash the primary.",
            metrics: {
                "Replication Slots Risk": {
                    title: "Replication Slots Risk",
                    text: "**What it is:** A risk assessment of replication slots, which record the oldest WAL position needed by a subscriber. If a consumer disconnects, the slot holds WAL indefinitely.\n\n**Why it matters:** An inactive replication slot is one of the most dangerous conditions in PostgreSQL. It prevents WAL deletion, causing the `pg_wal` directory to grow until the disk is full, resulting in a database crash."
                },
                "Max Repl Lag": {
                    title: "Max Replica Lag",
                    text: "**What it is:** The highest lag (in seconds or bytes) among all connected standby servers.\n\n**Why it matters:** Determines the data currency of reads routed to standbys and the potential data loss window if the primary fails and a standby must be promoted."
                }
            }
        },
        "Security Monitor": {
            description: "Audits user access, failed authentication attempts, privilege assignments, and data activity patterns. Essential for compliance (SOC2, PCI-DSS) and detecting security threats. Unlike some other systems, PostgreSQL brute-force protection must often be implemented at the network or log-analysis layer. This dashboard surfaces failed login patterns and audits 'Superuser' counts, which should always be kept to an absolute minimum according to the Principle of Least Privilege.",
            metrics: {
                "Failed Logins": {
                    title: "Failed Logins (Count)",
                    text: "**What it is:** Count of authentication failures from log analysis.\n\n**Why it matters:** Indicates brute-force attacks, misconfigured applications, or potential credential theft attempts."
                },
                "Superuser Count": {
                    title: "Superuser Count",
                    text: "**What it is:** Count of roles with the `SUPERUSER` attribute.\n\n**Why it matters:** Superusers bypass all access controls and can even access the underlying OS via PostgreSQL. This list should be extremely short (typically just the 'postgres' role and one emergency DBA account)."
                }
            }
        },
        "Best Practices": {
            description: "Automated auditor that compares the current configuration against PostgreSQL-specific best practices, producing a clear RED/YELLOW/GREEN report with remediation SQL. Surfaces accumulated technical risk from sub-optimal configurations, such as inadequate `shared_buffers` or aggressive `autovacuum` settings.",
            metrics: {
                "shared_buffers": {
                    title: "shared_buffers",
                    text: "**What it is:** PostgreSQL's own memory cache for data pages.\n\n**Why it matters:** Production databases require significant memory for caching to avoid disk I/O. Inadequate settings lead to frequent disk reads and poor performance."
                },
                "Autovacuum Config": {
                    title: "Autovacuum Configuration",
                    text: "**What it is:** Settings controlling the background cleanup process.\n\n**Why it matters:** Essential for preventing bloat and transaction ID (XID) wraparound."
                }
            }
        }
    };

    /**
     * Shows a modal with information about a specific metric or section.
     */
    window.pgShowInfo = function(section, metric) {
        const data = window.pgDashboardInfo[section];
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
    window.pgShowDashboardDetail = function(dashboardName) {
        const data = window.pgDashboardInfo[dashboardName];
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
