package scoring

import (
	"context"

	"github.com/rsharma155/sql_optima/internal/missing_index/catalog"
	"github.com/rsharma155/sql_optima/internal/missing_index/logging"
	"github.com/rsharma155/sql_optima/internal/missing_index/types"
)

const (
	StrongRecommendationThreshold    = 0.85
	TentativeRecommendationThreshold = 0.65
)

func ScoreCandidates(
	ctx context.Context,
	verificationResults []types.VerificationResult,
	tableInfos []catalog.TableInfo,
	options *types.RequestOptions,
) []types.ScoredCandidate {

	scored := []types.ScoredCandidate{}

	if options == nil {
		options = defaultOptions()
	}

	minImprovement := options.MinImprovementPct
	if minImprovement <= 0 {
		minImprovement = 15.0
	}

	for _, result := range verificationResults {
		if !result.Success {
			continue
		}

		minImp := minImprovement
		confThresh := TentativeRecommendationThreshold
		if result.HeuristicOnly {
			minImp = 5.0
			confThresh = 0.52
		}

		if result.ImprovementPct < minImp {
			logging.Debug(ctx, "Candidate below improvement threshold", map[string]any{
				"candidate":   result.Candidate.Table.Name,
				"improvement": result.ImprovementPct,
				"threshold":   minImp,
			})
			continue
		}

		if !result.HeuristicOnly && !result.IndexUsedInPlan {
			logging.Debug(ctx, "Candidate index not used in plan", map[string]any{
				"candidate": result.Candidate.Table.Name,
			})
			continue
		}

		confidence := calculateConfidence(result, tableInfos)

		if confidence < confThresh {
			logging.Debug(ctx, "Candidate below confidence threshold", map[string]any{
				"candidate":  result.Candidate.Table.Name,
				"confidence": confidence,
			})
			continue
		}

		indexStmt := buildIndexStatement(result.Candidate)

		reasoning := buildReasoning(result, confidence)

		scored = append(scored, types.ScoredCandidate{
			Table:            result.Candidate.Table,
			IndexMethod:      result.Candidate.IndexMethod,
			IndexStatement:   indexStmt,
			Confidence:       confidence,
			Reasoning:        reasoning,
			OriginalCost:     result.OriginalCost,
			HypotheticalCost: result.HypotheticalCost,
			ImprovementPct:   result.ImprovementPct,
			PlanChanged:      result.PlanChanged,
			IndexUsed:        result.IndexUsedInPlan,
		})
	}

	// Sort by confidence
	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].Confidence > scored[i].Confidence {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	logging.Info(ctx, "Scored candidates", map[string]any{
		"recommended": len(scored),
	})

	return scored
}

func defaultOptions() *types.RequestOptions {
	return &types.RequestOptions{
		MaxCandidates:     5,
		MinImprovementPct: 15.0,
	}
}

func calculateConfidence(result types.VerificationResult, tableInfos []catalog.TableInfo) float64 {
	confidence := 0.5
	if result.HeuristicOnly {
		confidence = 0.52
	}

	// High cost reduction is a strong positive signal
	if result.ImprovementPct > 50 {
		confidence += 0.25
	} else if result.ImprovementPct > 30 {
		confidence += 0.15
	} else if result.ImprovementPct > 15 {
		confidence += 0.1
	}

	// Plan change is a positive signal
	if result.PlanChanged {
		confidence += 0.1
	}

	// Index used in plan is a strong positive signal
	if result.IndexUsedInPlan {
		confidence += 0.15
	}

	// Fewer key columns is better (more targeted index)
	if len(result.Candidate.KeyColumns) <= 2 {
		confidence += 0.1
	} else if len(result.Candidate.KeyColumns) > 3 {
		confidence -= 0.1
	}

	// Cap at 1.0
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

func buildIndexStatement(candidate types.IndexCandidate) string {
	cols := ""
	for i, col := range candidate.KeyColumns {
		if i > 0 {
			cols += ", "
		}
		cols += col.Name
		if col.Descending {
			cols += " DESC"
		}
	}

	include := ""
	if len(candidate.IncludeCols) > 0 {
		include = " INCLUDE ("
		for i, col := range candidate.IncludeCols {
			if i > 0 {
				include += ", "
			}
			include += col.Name
		}
		include += ")"
	}

	return "CREATE INDEX CONCURRENTLY ON " + candidate.Table.Schema + "." + candidate.Table.Name + " (" + cols + ")" + include
}

func getFirstFewCols(cols []types.IndexColumn) string {
	result := ""
	for i := 0; i < len(cols) && i < 3; i++ {
		if i > 0 {
			result += "_"
		}
		result += cols[i].Name
	}
	return result
}

func getFirstFewChars(cols []types.IndexColumn) string {
	return getFirstFewCols(cols)
}

func buildReasoning(result types.VerificationResult, confidence float64) []string {
	reasoning := []string{}

	if result.HeuristicOnly {
		reasoning = append(reasoning, "Heuristic estimate (HypoPG verification not used)")
	} else {
		reasoning = append(reasoning, "Verified with hypothetical index (HypoPG)")
	}

	if result.ImprovementPct > 30 {
		reasoning = append(reasoning, "Estimated or measured cost improvement vs baseline plan")
	} else {
		reasoning = append(reasoning, "Moderate estimated improvement")
	}

	if !result.HeuristicOnly && result.IndexUsedInPlan {
		reasoning = append(reasoning, "Hypothetical index chosen in replanned query")
	}

	if result.PlanChanged {
		reasoning = append(reasoning, "Execution plan shape changed under verification")
	}

	if confidence >= StrongRecommendationThreshold {
		reasoning = append(reasoning, "High confidence recommendation")
	}

	return reasoning
}
