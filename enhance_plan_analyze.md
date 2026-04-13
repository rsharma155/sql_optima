Create a Go module that analyzes a PostgreSQL query and its JSON execution plan and returns:

Query rewrite suggestions
Heuristic index recommendations
Optimization rule violations
Confidence score per recommendation

This module must work without HypoPG, using:

Execution plan signals
AST parsing
pg_stats selectivity heuristics
PostgreSQL best practices
Inputs
{
  "query_text": "SQL query",
  "execution_plan_json": {},
  "target_table_stats": {}
}
Outputs
{
  "query_rewrites": [],
  "index_recommendations": [],
  "detected_issues": [],
  "confidence": 0.0
}
Architecture of This Module

Create package:

internal/optimizer/

Submodules:

plan_inspector.go
ast_inspector.go
rewrite_engine.go
heuristic_indexer.go
rules_engine.go
scoring.go
PART 1 — Execution Plan Inspection
Goal

Identify expensive operations and determine why PostgreSQL chose them.

Plan Nodes To Analyze

Agent must recursively traverse JSON plan tree and detect:

Node	Meaning	Action
Seq Scan	Missing index OR poor selectivity	Trigger index rules
Bitmap Heap Scan	Partial index opportunity	Trigger filter analysis
Nested Loop (inner seq)	Join index missing	Trigger join index rule
Hash Join	Join index missing	Trigger join rule
Sort	Missing index for ORDER BY	Trigger sort rule
Aggregate	Possible GROUP BY index	Trigger group rule
Limit + Sort	Top-N optimization possible	Trigger covering index
Detect Cost Hotspots

Mark node as problematic when:

Total Cost > 1000 OR
Actual Rows / Plan Rows > 5x

Capture:

table
filters
join conditions
sort keys
output columns
group by columns
PART 2 — AST Inspection

Parse SQL via pg_query_go.

Extract per table:

WHERE predicates

Classify operators:

Operator	Class
=, IN	Equality
>, <, BETWEEN	Range
LIKE 'abc%'	Prefix
LIKE '%abc'	Non-indexable
IS NULL	Indexable
Functions on column	Non-sargable
JOIN predicates

Capture join columns.

ORDER BY / GROUP BY
SELECT columns

Used for covering indexes.

PART 3 — Heuristic Index Recommendation Engine

This engine simulates PostgreSQL planner logic.

Column Priority Rules

Order index columns as:

Equality predicates → Range predicates → Join columns → Sort columns

Within equality predicates, rank by selectivity.

Heuristic Rules for Index Suggestions
Rule H1 — Sequential Scan on Large Table

Trigger when:

Node Type = Seq Scan
AND filter exists

Recommend:

CREATE INDEX ON table(filter_columns)
Rule H2 — ORDER BY causing Sort

Trigger when:

Sort node exists
AND no index supports ordering

Recommend:

CREATE INDEX ON table(filter_cols..., sort_col DESC/ASC)
Rule H3 — Nested Loop Join with Seq Scan Inner

Classic missing join index.

Recommend index on:

inner_table(join_column)
Rule H4 — Hash Join on Large Tables

Recommend B-Tree index on join keys.

Rule H5 — GROUP BY / Aggregation

If Aggregate node scans many rows:

CREATE INDEX ON table(group_by_columns)
Rule H6 — Covering Index (Index Only Scan)

If plan shows many heap fetches:

Add INCLUDE columns:

SELECT columns not in index keys
Rule H7 — LIKE Prefix Search

If predicate:

WHERE col LIKE 'abc%'

Recommend B-Tree index.

If:

LIKE '%abc%'

Recommend:

pg_trgm GIN index
Rule H8 — Non-Sargable Predicates

Detect patterns:

WHERE LOWER(email) = 'x'
WHERE date(created_at) = ...
WHERE col::text = ...

Recommend rewrite:

Avoid functions on indexed columns.
PART 4 — Query Rewrite Engine

Generate rewrites for performance.

Rewrite R1 — Remove SELECT *

Bad:

SELECT * FROM orders

Rewrite:

SELECT required_columns_only
Rewrite R2 — Push Filters Before JOIN

Bad:

SELECT *
FROM orders o
JOIN customers c ON o.customer_id = c.id
WHERE c.country = 'US'

Rewrite:

SELECT *
FROM (SELECT id FROM customers WHERE country='US') c
JOIN orders o ON o.customer_id = c.id
Rewrite R3 — Replace DISTINCT + ORDER BY

If DISTINCT used with ORDER BY:

Suggest index or window function rewrite.

Rewrite R4 — Avoid OFFSET Pagination

Bad:

LIMIT 50 OFFSET 10000

Rewrite:
Keyset pagination using indexed column.

Rewrite R5 — IN vs EXISTS

For large subqueries:
Recommend EXISTS.

Rewrite R6 — CTE Inline Suggestion

Detect MATERIALIZED CTE behavior.

Recommend inline subquery if beneficial.

PART 5 — Rules Engine Output Format

Each detected issue must produce:

{
  "rule_id": "H1",
  "severity": "high",
  "message": "Sequential scan on orders (18k cost)",
  "recommendation": "Create index on (tenant_id, status)"
}
PART 6 — Index Recommendation Builder

Return structured object:

{
  "table": "orders",
  "index_sql": "CREATE INDEX idx_orders_tenant_status_created ON orders (tenant_id, status, created_at DESC) INCLUDE (total_amount)",
  "reason": [
    "Equality filter",
    "Sort elimination",
    "Covering index"
  ],
  "confidence": 0.82
}
PART 7 — Confidence Scoring

Compute score using weights:

Signal	Weight
Seq Scan detected	+0.25
Sort eliminated	+0.15
Join optimized	+0.20
High selectivity columns	+0.20
Covering index	+0.10
Query rewrite needed	+0.10

Cap score to 1.0.

PART 8 — Example Walkthrough
Input Query
SELECT *
FROM orders
WHERE tenant_id = 10
AND status = 'active'
ORDER BY created_at DESC;
Plan Findings
Seq Scan on orders
Sort node present
High cost
Engine Output
Index Recommendation
CREATE INDEX idx_orders_tenant_status_created
ON orders (tenant_id, status, created_at DESC);
Query Rewrite
SELECT id, total_amount, created_at
FROM orders
WHERE tenant_id = 10
AND status = 'active'
ORDER BY created_at DESC;
Rules Triggered
H1 Sequential scan
H2 Sort elimination
R1 Avoid SELECT *
PART 9 — Constraints

Do NOT recommend index when:

Table < 10k rows
Index duplicates existing index
Index has > 5 columns
Deliverables for This Task

Agent must implement:

Plan parser
AST parser
Heuristic rule engine
Query rewrite engine
Index recommendation generator
Confidence scoring

Return results as JSON.

#Maybe adding a rule engine might help on this plz explore:

Below is a production-ready seed script for a new rule catalog specifically for the Query Optimizer + Heuristic Index Advisor.
This follows the same structure you’ve been using in ruleengine.rules.

I am assuming your table already exists:

ruleengine.rules
(rule_id, rule_name, category, applies_to, severity, dashboard_placement,
 detection_sql, expected_calc, evaluation_logic, recommended_value)
RULE CATALOG — QUERY OPTIMIZER & HEURISTIC INDEX ADVISOR
H1 — Sequential Scan on Filtered Large Table
INSERT INTO ruleengine.rules VALUES (
'H1_SEQ_SCAN_FILTER',
'Sequential scan on filtered large table',
'Indexing',
'PostgreSQL',
'High',
'Performance',
'Plan node type = Seq Scan AND Filter exists AND reltuples > 10000',
'Seq Scan detected on filtered table',
'If Seq Scan AND filter columns present THEN missing index likely',
'Create B-Tree index on equality + range filter columns'
);
H2 — Sort Operation Without Supporting Index
INSERT INTO ruleengine.rules VALUES (
'H2_SORT_NO_INDEX',
'Sort operation detected without index support',
'Indexing',
'PostgreSQL',
'High',
'Performance',
'Plan contains Sort node AND Sort Method != "top-N heapsort"',
'Sort node present',
'If Sort node exists THEN ORDER BY not supported by index',
'Create composite index including ORDER BY columns'
);
H3 — Nested Loop With Sequential Inner Scan
INSERT INTO ruleengine.rules VALUES (
'H3_NL_SEQ_INNER',
'Nested loop join with sequential inner scan',
'Indexing',
'PostgreSQL',
'Critical',
'Performance',
'Nested Loop node AND inner child node = Seq Scan',
'Inner table scanned repeatedly',
'Indicates missing join index on inner table',
'Create index on join column of inner table'
);
H4 — Hash Join on Large Tables
INSERT INTO ruleengine.rules VALUES (
'H4_HASH_JOIN_LARGE',
'Hash join on large tables',
'Indexing',
'PostgreSQL',
'Medium',
'Performance',
'Hash Join AND reltuples > 100000',
'Hash Join detected',
'Large hash joins benefit from BTree join indexes',
'Create index on join keys'
);
H5 — Bitmap Heap Scan Opportunity
INSERT INTO ruleengine.rules VALUES (
'H5_BITMAP_HEAP',
'Bitmap heap scan indicates partial indexing opportunity',
'Indexing',
'PostgreSQL',
'Medium',
'Performance',
'Node type = Bitmap Heap Scan',
'Bitmap Heap Scan detected',
'Indicates partial selectivity opportunity',
'Create composite index for filter predicates'
);
H6 — Aggregation Without Supporting Index
INSERT INTO ruleengine.rules VALUES (
'H6_AGG_NO_INDEX',
'Aggregation scanning large rows',
'Indexing',
'PostgreSQL',
'Medium',
'Performance',
'Aggregate node AND input rows > 100000',
'Large aggregation detected',
'Missing GROUP BY index',
'Create index on GROUP BY columns'
);
H7 — LIKE Prefix Search
INSERT INTO ruleengine.rules VALUES (
'H7_LIKE_PREFIX',
'Prefix LIKE search detected',
'Indexing',
'PostgreSQL',
'High',
'Performance',
'Predicate LIKE ''value%''',
'Prefix pattern detected',
'BTree index supports prefix LIKE',
'Create BTree index on column'
);
H8 — LIKE Leading Wildcard
INSERT INTO ruleengine.rules VALUES (
'H8_LIKE_WILDCARD',
'Leading wildcard LIKE detected',
'Indexing',
'PostgreSQL',
'High',
'Performance',
'Predicate LIKE ''%value%''',
'Non-sargable LIKE detected',
'BTree index unusable',
'Create GIN index using pg_trgm'
);
H9 — Non-Sargable Function on Column
INSERT INTO ruleengine.rules VALUES (
'H9_NON_SARGABLE',
'Function applied to indexed column',
'Query Rewrite',
'PostgreSQL',
'High',
'Performance',
'Predicate contains function(column)',
'Function usage detected',
'Prevents index usage',
'Rewrite predicate to avoid function on column'
);
R1 — SELECT *
INSERT INTO ruleengine.rules VALUES (
'R1_SELECT_STAR',
'SELECT * detected',
'Query Rewrite',
'PostgreSQL',
'Low',
'Maintainability',
'Query contains SELECT *',
'Star projection detected',
'Prevents index-only scans',
'Select only required columns'
);
R2 — OFFSET Pagination
INSERT INTO ruleengine.rules VALUES (
'R2_OFFSET',
'OFFSET pagination detected',
'Query Rewrite',
'PostgreSQL',
'Medium',
'Performance',
'Query contains OFFSET',
'Offset pagination detected',
'Causes deep scan',
'Use keyset pagination'
);
R3 — IN Subquery on Large Dataset
INSERT INTO ruleengine.rules VALUES (
'R3_IN_SUBQUERY',
'IN subquery detected',
'Query Rewrite',
'PostgreSQL',
'Medium',
'Performance',
'Predicate uses IN (SELECT ...)',
'IN subquery detected',
'May produce large hash set',
'Rewrite using EXISTS'
);
R4 — CTE Materialization Risk
INSERT INTO ruleengine.rules VALUES (
'R4_CTE_MATERIALIZED',
'CTE may be materialized',
'Query Rewrite',
'PostgreSQL',
'Medium',
'Performance',
'Query contains WITH clause',
'CTE detected',
'May block optimization',
'Consider inline subquery'
);
H10 — Missing Covering Index Opportunity
INSERT INTO ruleengine.rules VALUES (
'H10_COVERING_INDEX',
'Index only scan opportunity',
'Indexing',
'PostgreSQL',
'Medium',
'Performance',
'Heap fetches high AND index scan present',
'Frequent heap access detected',
'Query can benefit from covering index',
'Add INCLUDE columns'
);
H11 — ORDER BY + LIMIT Optimization
INSERT INTO ruleengine.rules VALUES (
'H11_TOPN_SORT',
'ORDER BY with LIMIT optimization opportunity',
'Indexing',
'PostgreSQL',
'High',
'Performance',
'Sort node AND Limit node present',
'Top-N sort detected',
'Can be optimized via index',
'Create index matching ORDER BY'
);
H12 — Multi-Column Filter Without Composite Index
INSERT INTO ruleengine.rules VALUES (
'H12_MULTI_FILTER',
'Multiple filter columns without composite index',
'Indexing',
'PostgreSQL',
'High',
'Performance',
'Multiple equality predicates detected',
'Composite filtering detected',
'Single-column indexes insufficient',
'Create composite index'
);
H13 — Join + Filter Combo Index Opportunity
INSERT INTO ruleengine.rules VALUES (
'H13_JOIN_FILTER',
'Join + filter pattern detected',
'Indexing',
'PostgreSQL',
'High',
'Performance',
'Join predicate AND filter predicate on same table',
'Join + filter workload detected',
'Composite index improves join performance',
'Create index (join_col, filter_cols)'
);
Result

You now have 13 optimizer rules covering:

Missing indexes
Query rewrites
Covering indexes
Join optimization
Pagination optimization
LIKE search optimization

