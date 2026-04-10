HIGH-LEVEL ARCHITECTURE FOR GO 

Agent responsibilities:

Register server with monitoring DB
Pull enabled rules from Postgres
Execute detection SQL on SQL Server
Convert result → JSON payload
Send payload to Postgres functions
Trigger evaluation
Repeat on schedule

This runs every 5–15 minutes.

AGENT RUNTIME LOOP
Start Agent
   ↓
Register Server
   ↓
Start Rule Run
   ↓
Fetch Enabled Rules
   ↓
FOR EACH RULE
    Execute DetectionSQL on SQL Server
    Convert result → JSON
    Send to Postgres store_raw_result()
END
   ↓
Call evaluate_run()
   ↓
Sleep (interval)
GO MODULE STRUCTURE
sqlmonitor-agent/
│
├── cmd/agent/main.go
├── config/
│     config.go
├── collectors/
│     sqlserver.go
├── postgres/
│     pgclient.go
├── engine/
│     runner.go
├── models/
│     models.go
└── scheduler/
      scheduler.go
CONFIG STRUCTURE

Example YAML:

agent_name: ktm-sql-collector
interval_seconds: 300

sqlserver:
  host: localhost
  port: 1433
  user: monitor_login
  password: ***
  encrypt: false

postgres:
  host: monitoring-db
  port: 5432
  user: monitor
  password: ***
  database: sqlmonitor
DATA MODELS
Rule Model (from Postgres)
type Rule struct {
    RuleID        string
    RuleName      string
    DetectionSQL  string
}
Detection Result Payload
type DetectionPayload struct {
    RuleID string        `json:"rule_id"`
    Rows   []interface{} `json:"rows"`
}
POSTGRES CLIENT LAYER
Connect to Monitoring DB

Use pgx (recommended for performance)

pgxpool.Connect(ctx, connString)
Fetch Enabled Rules

Query:

SELECT rule_id, detection_sql
FROM rule_engine.rules
WHERE is_enabled = true;

Go function:

func FetchRules() ([]Rule, error)
Start Rule Run

Call Postgres function:

SELECT rule_engine.start_rule_run($1);

Go:

func StartRuleRun(serverID int) (int64, error)
Send Raw Result

Call function:

SELECT rule_engine.store_raw_result($1,$2,$3,$4);

Go:

func StoreRawResult(runID int64, serverID int, ruleID string, payload []byte)
SQL SERVER COLLECTOR

Use Microsoft driver:

github.com/microsoft/go-mssqldb
Execute Detection SQL

Generic executor:

func ExecuteDetectionSQL(db *sql.DB, query string) ([]map[string]interface{}, error)

Workflow:

Run query
Get column names
Scan rows dynamically
Convert to map[string]interface{}
Return []map → JSON marshal
CORE ENGINE (runner.go)

This is the brain of the agent.

Pseudo-implementation:

func RunCollectionCycle() {

    serverID := RegisterServer()

    runID := StartRuleRun(serverID)

    rules := FetchRules()

    for _, rule := range rules {

        rows := ExecuteDetectionSQL(sqlServerConn, rule.DetectionSQL)

        payload := json.Marshal(rows)

        StoreRawResult(runID, serverID, rule.RuleID, payload)
    }

    EvaluateRun(runID)
}
CALL RULE EVALUATION
SELECT rule_engine.evaluate_run($1);
SCHEDULER

Use simple ticker:

ticker := time.NewTicker(interval)

for {
   RunCollectionCycle()
   <-ticker.C
}
ERROR HANDLING STRATEGY

Agent must NEVER stop.

If a rule fails:

• log error
• continue next rule
• send partial data

This is critical for reliability.

LOGGING STRATEGY

Log per cycle:

Cycle started
Rules fetched: 29
Rule INST_MEM_MAX_001 executed (120ms)
Rule BACKUP_LOG_010 failed (timeout)
Cycle finished (4.2s)
SECURITY BEST PRACTICE

SQL Server login requires only:

VIEW SERVER STATE
VIEW ANY DEFINITION
CONNECT ANY DATABASE
SELECT on msdb

Read-only monitoring login.

PARALLEL EXECUTION (PHASE 2)

Future optimization:

Run rules concurrently:

workerPool := 5 goroutines

This reduces collection time from minutes → seconds.