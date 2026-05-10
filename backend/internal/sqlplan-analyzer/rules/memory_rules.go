// File: internal/sqlplan_rules/memory_rules.go
// Purpose: Memory and memory grant rules based on SQL Server Execution Plans book
// Package: github.com/rsharma155/sqlplan-analyzer/rules
package rules

import (
	"fmt"
	"strings"

	"github.com/rsharma155/sqlplan-analyzer/models"
)

func (e *RuleEngine) registerMemoryRules() {
	e.rules = append(e.rules, RuleDefinition{
		Name:        "MemorySpillToTempDB",
		Description: "Memory exceeded, spilled to tempdb",
		Category:    "Memory",
		Severity:    models.SeverityHigh,
		RuleType:    RuleTypeMemory,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			return op.ActualSpills > 0
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Operation Spilled to tempdb",
				fmt.Sprintf("Query exceeded memory grant and wrote %d pages to tempdb. This is 100x slower than memory operations.", op.ActualSpills),
				"Increase memory grant, optimize query to reduce memory needs, or reduce result set size",
				"Massive disk I/O degradation, up to 100x slower than memory operations",
				models.SeverityHigh,
			)
		},
		Priority:    1,
		BookChapter: "Chapter 5: Spills to Tempdb",
		Tags:        []string{"spill", "tempdb", "memory"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "ExcessiveMemoryGrant",
		Description: "Memory grant much larger than needed",
		Category:    "Memory",
		Severity:    models.SeverityMedium,
		RuleType:    RuleTypeMemory,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if op.ID != 1 || plan.QueryPlan == nil {
				return false
			}
			granted := int64(plan.QueryPlan.MemoryGrantInfo.GrantedMemory)
			maxUsed := int64(plan.QueryPlan.MemoryGrantInfo.MaxUsedMemory)

			if granted == 0 || maxUsed == 0 {
				return false
			}

			ratio := float64(maxUsed) / float64(granted)
			return ratio < 0.3 && granted > 1024*1024
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			grantedMB := float64(plan.QueryPlan.MemoryGrantInfo.GrantedMemory) / 1024
			maxMB := float64(plan.QueryPlan.MemoryGrantInfo.MaxUsedMemory) / 1024
			return makeBaseFinding(op,
				"Excessive Memory Grant",
				fmt.Sprintf("Granted %.1f MB but only used %.1f MB (%.1f%% utilization). Large unused memory blocks other queries.", grantedMB, maxMB, (maxMB/grantedMB)*100),
				"Review query pattern and consider memory grant hints. This may indicate stale statistics.",
				"Resource contention, other queries wait for memory",
				models.SeverityMedium,
			)
		},
		Priority:    3,
		BookChapter: "Chapter 5: Memory Grants",
		Tags:        []string{"memory", "grant", "excessive"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "InsufficientMemoryGrant",
		Description: "Memory grant smaller than ideal",
		Category:    "Memory",
		Severity:    models.SeverityMedium,
		RuleType:    RuleTypeMemory,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if op.ID != 1 || plan.QueryPlan == nil {
				return false
			}
			ideal := plan.QueryPlan.MemoryGrantInfo.SerialDesiredMemory
			granted := int64(plan.QueryPlan.MemoryGrantInfo.GrantedMemory)

			if ideal == 0 || granted == 0 {
				return false
			}

			ratio := float64(granted) / float64(ideal)
			return ratio < 0.7
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			idealMB := float64(plan.QueryPlan.MemoryGrantInfo.SerialDesiredMemory) / 1024
			grantedMB := float64(plan.QueryPlan.MemoryGrantInfo.GrantedMemory) / 1024
			return makeBaseFinding(op,
				"Insufficient Memory Grant",
				fmt.Sprintf("Query ideal memory is %.1f MB but only granted %.1f MB. Will require multiple passes or spills.", idealMB, grantedMB),
				"Consider increasing memory or simplifying query to fit in available grant",
				"Multiple passes over data, potential spills",
				models.SeverityMedium,
			)
		},
		Priority:    2,
		BookChapter: "Chapter 5: Memory Grants",
		Tags:        []string{"memory", "grant", "insufficient"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "HashSpillWarning",
		Description: "Hash operation spilled to tempdb",
		Category:    "Memory",
		Severity:    models.SeverityHigh,
		RuleType:    RuleTypeTempDB,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			return strings.Contains(op.PhysicalOp, "Hash") && op.ActualSpills > 0
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Hash Operation Spill",
				fmt.Sprintf("Hash operation spilled %d pages to tempdb. Hash tables require contiguous memory.", op.ActualSpills),
				"Increase memory grant, reduce input size, or partition data",
				"Disk I/O bottleneck for hash operations",
				models.SeverityHigh,
			)
		},
		Priority:    1,
		BookChapter: "Chapter 5: Hash Spills",
		Tags:        []string{"hash", "spill", "tempdb"},
	})
}

