import sys

path = '../frontend/js/utils/pg_dashboard_info.js'
with open(path, 'r') as f:
    content = f.read()

# 1. Add metrics to Control Center
control_center_new_metrics = \"\"\"                \"Wait Categories\": {
                    title: \"Wait Categories\",
                    text: \"**What it is:** Distribution of session wait states aggregated by category (CPU, IO, Lock, Client, etc.).\\n\\n**Why it matters:** Identifies the primary bottleneck type. CPU indicates processing pressure; IO indicates storage latency; Lock indicates application-level contention.\"
                },
                \"Session Distribution\": {
                    title: \"Session State Distribution\",
                    text: \"**What it is:** Break-down of all connections by their current state: Active, Idle, Idle in Transaction, and Waiting.\\n\\n**Warning signal:** A high number of 'Idle in Transaction' sessions is dangerous as they hold locks and prevent autovacuum from reclaiming space.\"
                },\"\"\"

if '\"Live Sessions\": {' in content and '\"Wait Categories\"' not in content:
    content = content.replace('\"Live Sessions\": {', control_center_new_metrics + '\n                \"Live Sessions\": {')

# 2. Add Enterprise Monitor section if missing
enterprise_monitor_new_metrics = \"\"\"        \"Enterprise Monitor\": {
            description: \"Deep-dive diagnostics for enterprise-scale PostgreSQL workloads, focusing on buffer management, background writer efficiency, and internal resource contention.\",
            metrics: {
                \"Contention Waits\": {
                    title: \"Contention Wait Analysis\",
                    text: \"**What it is:** Detailed analysis of non-IO waits such as LWLocks (Lightweight Locks) and Buffer Pins.\\n\\n**Why it matters:** High LWLock contention (e.g., WALWriteLock, CLogControlLock) often indicates internal PostgreSQL bottlenecks that require configuration tuning rather than just adding hardware.\"
                },
                \"I/O & Temp Spill\": {
                    title: \"Database I/O & Temp Spill\",
                    text: \"**What it is:** Measures physical disk reads vs. temporary file creation (temp spills).\\n\\n**Why it matters:** Temp spills occur when `work_mem` is insufficient for a sort or hash operation, forcing PostgreSQL to use disk. This is 10-100x slower than RAM-based operations.\"
                }
            }
        },\"\"\"

if '\"Enterprise Monitor\": {' not in content:
    content = content.replace('\"Best Practices\": {', enterprise_monitor_new_metrics + '\n        \"Best Practices\": {')

with open(path, 'w') as f:
    f.write(content)
