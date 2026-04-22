package rules

var Registry = map[string]RuleHandler{
	"max_server_memory":  EvaluateMaxMemory,
	"maxdop":           EvaluateMaxDOP,
	"cost_threshold":   EvaluateCostThreshold,
	"pg_shared_buffers": EvaluateSharedBuffers,
	"pg_autovacuum":     EvaluateAutovacuum,
	"pg_vacuum_health": EvaluateVacuumHealth,
	"pg_table_bloat":   EvaluateTableBloat,
	"pg_unused_index":  EvaluateIndexUsage,
	"pg_cache_hit":     EvaluateCacheHit,
	"pg_missing_index": EvaluateMissingIndex,
}