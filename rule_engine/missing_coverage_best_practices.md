the current rules have strong coverage in these domains:

Memory
Parallelism
TempDB
Backups (basic)
Query Store
Security basics (sa/xp_cmdshell)
Disk latency
Alerts presence
Autogrowth %
Auto shrink/close
Stats auto update
Resource governor (basic)
PLE baseline
Basic HA recommendation

But enterprise monitoring tools typically have 70–120 rules. We currently have ~30. So we need the missing half.

CRITICAL GAPS — SQL SERVER (must add)

These are the most important missing rules.

1. MAX WORKER THREADS (commonly forgotten)
Why: Non-default value can cripple concurrency.

Rule:
SELECT value_in_use
FROM sys.configurations
WHERE name = 'max worker threads';

Severity:Critical if NOT 0 (default)

2.  LIGHTWEIGHT POOLING (Fiber mode)
This setting is dangerous if enabled.

SELECT value_in_use
FROM sys.configurations
WHERE name = 'lightweight pooling';

Severity: Critical if ON

3. PRIORITY BOOST
SELECT value_in_use
FROM sys.configurations
WHERE name = 'priority boost';

Severity: Critical if ON

4️. LOCK PAGES IN MEMORY

You check IFI but LPIM is missing.

Detection:

SELECT sql_memory_model_desc 
FROM sys.dm_os_sys_info;

Severity: Warning → Critical (prod)

5️. SQL SERVER POWER PLAN (CPU throttling)

Detection (via PowerShell collector): Must be High Performance
Severity: Warning
This is a HUGE real-world issue.

6️. TRACE FLAGS CHECK

Enterprise baseline commonly checks:

Trace Flag	Why
1117	File growth uniform
1118	TempDB allocation
3226	Backup log noise
4199	Query optimizer fixes

Detection: DBCC TRACESTATUS(-1);

Severity: Warning

7️. PAGE VERIFY CHECKSUM

Often missed.

SELECT name, page_verify_option_desc
FROM sys.databases
WHERE database_id > 4;

Critical if NOT CHECKSUM.

8️. DATABASE TRUSTWORTHY ON

Security risk.

SELECT name
FROM sys.databases
WHERE is_trustworthy_on = 1
AND database_id > 4;

Severity: Critical

9️. CROSS DB OWNERSHIP CHAINING
SELECT name,is_db_chaining_on
FROM sys.databases
WHERE is_db_chaining_on=1;

Severity: Critical

10. ORPHANED USERS

Very common operational issue.

Detection query needed.

Severity: Warning

#BACKUP & RECOVERY GAPS

we have basic backup checks, but enterprise baseline includes:

11️. Backup retention policy

Check if backups older than X days still exist.

12️. Backup encryption enabled
SELECT encryptor_type 
FROM msdb.dbo.backupset;

Severity: Warning/Critical (regulated env)

13️. COPY_ONLY backup misuse detection

Important for AG environments.

14️. Last restore test date

DR testing rule (very important).

# SQL AGENT & JOB RELIABILITY

You only check alerts/operators.

Missing:

15️. Failed jobs in last 24h
SELECT name,last_run_outcome
FROM msdb.dbo.sysjobs j
JOIN msdb.dbo.sysjobservers s ON j.job_id=s.job_id
WHERE last_run_outcome=0;

Critical.

16️. Disabled jobs detection

Very important.

17️. Job ownership check (must be sa)

#Security best practice.

#AVAILABILITY / HA (big gap)

we only have HA recommendation logic. Missing real checks:

18️. AG replica health
Sync state
Failover mode
Suspend state

19️. Log shipping health
Last restore time
Copy delay
Restore delay

These are core enterprise rules.

#PERFORMANCE MONITORING GAPS

Huge missing area.

20️. Memory pressure signals

Check: Memory grants pending Target vs total server memory

21️. Plan cache bloat check

Single-use plans %.

22️. Long running queries baseline

Active requests > threshold.

23️. Blocking detection baseline

Wait chains.

24️. Top wait types baseline

Store trend.

#POSTGRES RULE GAPS

We mentioned rules for Postgres exist — here’s what is usually missing.

1. Postgres memory config

Check:shared_buffers, work_mem, maintenance_work_mem, effective_cache_size

2. Autovacuum health (CRITICAL)


Rules: autovacuum enabled, last_autovacuum per table, dead tuples %, Replication health

Check: replication lag, slot bloat, wal retention, Checkpoint tuning

Check: checkpoint_timeout, checkpoint_completion_target, Long running transactions, 

Huge bloat risk.

Idle in transaction sessions

Critical rule.

Index bloat detection

Enterprise must-have.