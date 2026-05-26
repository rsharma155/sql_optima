package reporter

import (
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strconv"
	"strings"

	"github.com/rsharma155/sqlplan-analyzer/models"
)

type HTMLReporter struct{}

func NewHTMLReporter() *HTMLReporter {
	return &HTMLReporter{}
}

func (r *HTMLReporter) Generate(plan *models.PlanAnalysis) string {
	var sb strings.Builder
	hs := plan.HealthScore.OverallScore
	hc := r.healthClass(hs)

	sb.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>SQL Server Execution Plan Analysis</title>
`)
	sb.WriteString(r.renderStyles())
	sb.WriteString(`</head>
<body>
<input type="checkbox" id="theme-toggle">
<div class="container">
  <div class="header">
    <div class="header-top">
      <h1>SQL Server Execution Plan Analysis</h1>
      <div class="header-badge `)
	sb.WriteString(hc)
	sb.WriteString(`">`)
	sb.WriteString(strconv.Itoa(hs))
	sb.WriteString(`</div>
      <label for="theme-toggle" class="theme-btn" title="Toggle dark/light">&#x1F319;</label>
    </div>
    <div class="meta">`)
	sb.WriteString(plan.Timestamp.Format("2006-01-02 15:04:05"))
	sb.WriteString(` &middot; `)
	sb.WriteString(strconv.Itoa(len(plan.Operators)))
	sb.WriteString(` operators &middot; `)
	sb.WriteString(strconv.Itoa(len(plan.Findings)))
	sb.WriteString(` findings &middot; SQL Optima Integrated`)
	if plan.CostSummary.TotalEstimatedCost > 0 {
		sb.WriteString(` &middot; cost `)
		sb.WriteString(fmt.Sprintf("%.4f", plan.CostSummary.TotalEstimatedCost))
	}
	sb.WriteString(`</div>
  </div>
  <div class="tabs">
`)
	sb.WriteString(r.renderTabRadio("summary", "Summary", true))
	sb.WriteString(r.renderTabRadio("runtime", "Runtime Evidence", false))
	sb.WriteString(r.renderTabRadio("findings", "Findings", false, len(plan.Findings)))
	sb.WriteString(r.renderTabRadio("planviewer", "Plan Viewer", false, len(plan.Operators)))
	sb.WriteString(r.renderTabRadio("recs", "Recommendations", false, len(plan.Recommendations)))
	sb.WriteString(r.renderTabRadio("indexes", "Missing Indexes", false, len(plan.MissingIndexes)))
	sb.WriteString(r.renderTabRadio("predicates", "Predicate Analysis", false))
	sb.WriteString(r.renderTabRadio("warnings", "Warnings", false, len(plan.Warnings)))
	sb.WriteString(`
    <div class="tab-content" id="content-summary">`)
	sb.WriteString(r.renderQueryIdentity(plan))
	sb.WriteString(r.renderSummaryTab(plan))
	sb.WriteString(`</div>
    <div class="tab-content" id="content-runtime">`)
	sb.WriteString(r.renderRuntimeEvidenceMatrix(plan))
	sb.WriteString(`</div>
    <div class="tab-content" id="content-findings">`)
	sb.WriteString(r.renderFindingsTab(plan))
	sb.WriteString(`</div>
    <div class="tab-content" id="content-planviewer">`)
	sb.WriteString(r.renderPlanViewerTab(plan))
	sb.WriteString(`</div>
    <div class="tab-content" id="content-recs">`)
	sb.WriteString(r.renderOptimizationForecast(plan))
	sb.WriteString(r.renderRecommendationsTab(plan))
	sb.WriteString(`</div>
    <div class="tab-content" id="content-indexes">`)
	sb.WriteString(r.renderMissingIndexesTab(plan))
	sb.WriteString(`</div>
    <div class="tab-content" id="content-predicates">`)
	sb.WriteString(r.renderPredicateAnalysis(plan))
	sb.WriteString(`</div>
    <div class="tab-content" id="content-warnings">`)
	sb.WriteString(r.renderWarningsTab(plan))
	sb.WriteString(`</div>
  </div>
</div>
</body>
</html>`)
	return sb.String()
}

func (r *HTMLReporter) renderQueryIdentity(plan *models.PlanAnalysis) string {
	var sb strings.Builder
	sb.WriteString(`<div class="tab-panel">`)
	sb.WriteString(`<h3 class="sec-title">Query Identity</h3>`)
	sb.WriteString(`<table class="dt"><tr><th>Property</th><th>Value</th></tr>`)

	if plan.Metadata.QueryText != "" {
		sb.WriteString(fmt.Sprintf(`<tr><td>Query Text</td><td><code>%s</code></td></tr>`, html.EscapeString(truncateText(plan.Metadata.QueryText, 200))))
	}
	if plan.Metadata.DatabaseName != "" {
		sb.WriteString(fmt.Sprintf(`<tr><td>Database</td><td>%s</td></tr>`, html.EscapeString(plan.Metadata.DatabaseName)))
	}
	if plan.Metadata.QueryHash != "" {
		sb.WriteString(fmt.Sprintf(`<tr><td>Query Hash</td><td><code>%s</code></td></tr>`, html.EscapeString(plan.Metadata.QueryHash)))
	}
	if plan.Metadata.QueryPlanHash != "" {
		sb.WriteString(fmt.Sprintf(`<tr><td>Plan Hash</td><td><code>%s</code></td></tr>`, html.EscapeString(plan.Metadata.QueryPlanHash)))
	}
	if plan.Metadata.StatementType != "" {
		sb.WriteString(fmt.Sprintf(`<tr><td>Statement Type</td><td>%s</td></tr>`, html.EscapeString(plan.Metadata.StatementType)))
	}
	if plan.Metadata.CEVersion != "" {
		sb.WriteString(fmt.Sprintf(`<tr><td>CE Version</td><td>%s</td></tr>`, html.EscapeString(plan.Metadata.CEVersion)))
	}
	if plan.QueryPlan != nil {
		sb.WriteString(fmt.Sprintf(`<tr><td>Optimization Level</td><td>%s</td></tr>`, html.EscapeString(plan.QueryPlan.OptimizationLevel)))
		if plan.QueryPlan.CompileTimeMs > 0 {
			sb.WriteString(fmt.Sprintf(`<tr><td>Compile Time</td><td>%d ms</td></tr>`, plan.QueryPlan.CompileTimeMs))
		}
		if plan.QueryPlan.CachedPlanSize > 0 {
			sb.WriteString(fmt.Sprintf(`<tr><td>Cached Plan Size</td><td>%d KB</td></tr>`, plan.QueryPlan.CachedPlanSize))
		}
	}
	if plan.Metadata.RetrievedFromCache {
		sb.WriteString(`<tr><td>Retrieved From Cache</td><td>Yes</td></tr>`)
	}
	sb.WriteString(`</table></div>`)
	return sb.String()
}

func (r *HTMLReporter) renderRuntimeEvidenceMatrix(plan *models.PlanAnalysis) string {
	var sb strings.Builder
	sb.WriteString(`<div class="tab-panel">`)
	sb.WriteString(`<h3 class="sec-title">Runtime Evidence Matrix</h3>`)

	sb.WriteString(`<table class="dt"><tr><th>Operator</th><th>Actual Rows</th><th>Est. Rows</th><th>Variance</th><th>CPU (ms)</th><th>Elapsed (ms)</th><th>Logical Reads</th><th>Executions</th></tr>`)

	for _, op := range plan.Operators {
		if op.ActualRows == 0 && op.ActualExecutions == 0 && op.EstimatedTotalSubtreeCost == 0 {
			continue
		}
		variance := "-"
		if op.EstimateRows > 0 && op.ActualRows > 0 {
			ratio := float64(op.ActualRows) / op.EstimateRows
			if ratio >= 1 {
				variance = fmt.Sprintf("%.0fx", ratio)
			} else {
				variance = fmt.Sprintf("%.0fx", 1/ratio)
			}
		}

		opName := op.PhysicalOp
		tbl := r.opTableShort(&op)
		if tbl != "" {
			opName = tbl + " (" + op.PhysicalOp + ")"
		}

		sb.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%.2f</td><td>%.2f</td><td>%d</td><td>%d</td></tr>`,
			html.EscapeString(opName),
			r.fmtInt(op.ActualRows),
			r.fmtRows(op.EstimateRows),
			variance,
			op.ActualCPUms,
			op.ActualElapsedms,
			op.ActualLogicalReads,
			op.ActualExecutions,
		))
	}
	sb.WriteString(`</table>`)
	sb.WriteString(r.renderCardinalityVarianceAnalysis(plan))
	sb.WriteString(`</div>`)
	return sb.String()
}

func (r *HTMLReporter) renderCardinalityVarianceAnalysis(plan *models.PlanAnalysis) string {
	var sb strings.Builder
	sb.WriteString(`<h3 class="sec-title">Cardinality Variance Analysis</h3>`)
	sb.WriteString(`<table class="dt"><tr><th>Ratio Range</th><th>Severity</th><th>Count</th></tr>`)

	criticalCount := 0
	highCount := 0
	mediumCount := 0
	goodCount := 0

	for _, op := range plan.Operators {
		if op.EstimateRows <= 0 || op.ActualRows <= 0 {
			continue
		}
		ratio := float64(op.ActualRows) / op.EstimateRows
		if ratio < 1 {
			ratio = 1 / ratio
		}

		if ratio > 100 {
			criticalCount++
		} else if ratio > 10 {
			highCount++
		} else if ratio > 2 {
			mediumCount++
		} else {
			goodCount++
		}
	}

	sb.WriteString(fmt.Sprintf(`<tr><td>&lt; 2x</td><td><span class="sev-good">Good</span></td><td>%d</td></tr>`, goodCount))
	sb.WriteString(fmt.Sprintf(`<tr><td>2-10x</td><td><span class="sev-med">Medium</span></td><td>%d</td></tr>`, mediumCount))
	sb.WriteString(fmt.Sprintf(`<tr><td>10-100x</td><td><span class="sev-high">High</span></td><td>%d</td></tr>`, highCount))
	sb.WriteString(fmt.Sprintf(`<tr><td>&gt; 100x</td><td><span class="sev-crit">Critical</span></td><td>%d</td></tr>`, criticalCount))
	sb.WriteString(`</table>`)
	return sb.String()
}

func (r *HTMLReporter) renderPredicateAnalysis(plan *models.PlanAnalysis) string {
	var sb strings.Builder
	sb.WriteString(`<div class="tab-panel">`)
	sb.WriteString(`<h3 class="sec-title">Predicate Inspector</h3>`)

	hasPredicates := false
	for _, op := range plan.Operators {
		if op.Predicate != nil || len(op.SeekPredicates) > 0 || op.Hash != nil || op.NestedLoops != nil {
			hasPredicates = true
			break
		}
	}

	if !hasPredicates {
		sb.WriteString(`<p class="none">No predicates, seek details, or join conditions extracted.</p></div>`)
		return sb.String()
	}

	sb.WriteString(`<table class="dt"><tr><th>Operator</th><th>Type</th><th>Details</th></tr>`)

	for _, op := range plan.Operators {
		if op.Predicate != nil && op.Predicate.ScalarString != "" {
			sb.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>Residual Predicate</td><td><code>%s</code></td></tr>`,
				html.EscapeString(op.PhysicalOp), html.EscapeString(op.Predicate.ScalarString)))
		}
		if len(op.SeekPredicates) > 0 {
			for _, sp := range op.SeekPredicates {
				seekType := "Seek"
				if sp.SeekType != "" {
					seekType = sp.SeekType
				}
				for _, pp := range sp.PrefixPredicate {
					sb.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s Predicate</td><td><code>%s</code></td></tr>`,
						html.EscapeString(op.PhysicalOp), seekType, html.EscapeString(pp.ScalarString)))
				}
			}
		}
		if op.Hash != nil && len(op.Hash.HashKeysBuild) > 0 {
			keys := make([]string, 0)
			for _, k := range op.Hash.HashKeysBuild {
				keys = append(keys, k.Column)
			}
			sb.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>Hash Build Keys</td><td><code>%s</code></td></tr>`,
				html.EscapeString(op.PhysicalOp), html.EscapeString(strings.Join(keys, ", "))))
		}
		if op.Hash != nil && len(op.Hash.HashKeysProbe) > 0 {
			keys := make([]string, 0)
			for _, k := range op.Hash.HashKeysProbe {
				keys = append(keys, k.Column)
			}
			sb.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>Hash Probe Keys</td><td><code>%s</code></td></tr>`,
				html.EscapeString(op.PhysicalOp), html.EscapeString(strings.Join(keys, ", "))))
		}
		if op.NestedLoops != nil && op.NestedLoops.Predicate != "" {
			sb.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>Join Predicate</td><td><code>%s</code></td></tr>`,
				html.EscapeString(op.PhysicalOp), html.EscapeString(op.NestedLoops.Predicate)))
		}
		if op.Merge != nil && len(op.Merge.InnerSideJoinColumns) > 0 {
			cols := make([]string, 0)
			for _, c := range op.Merge.InnerSideJoinColumns {
				cols = append(cols, c.Column)
			}
			sb.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>Merge Inner Join</td><td><code>%s</code></td></tr>`,
				html.EscapeString(op.PhysicalOp), html.EscapeString(strings.Join(cols, ", "))))
		}
		if op.Merge != nil && len(op.Merge.OuterSideJoinColumns) > 0 {
			cols := make([]string, 0)
			for _, c := range op.Merge.OuterSideJoinColumns {
				cols = append(cols, c.Column)
			}
			sb.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>Merge Outer Join</td><td><code>%s</code></td></tr>`,
				html.EscapeString(op.PhysicalOp), html.EscapeString(strings.Join(cols, ", "))))
		}
	}
	sb.WriteString(`</table>`)
	sb.WriteString(`</div>`)
	return sb.String()
}

func (r *HTMLReporter) renderOptimizationForecast(plan *models.PlanAnalysis) string {
	var sb strings.Builder
	sb.WriteString(`<div class="tab-panel">`)
	sb.WriteString(`<h3 class="sec-title">Optimization Forecast</h3>`)

	totalCost := plan.CostSummary.TotalEstimatedCost
	hasTableScan := false
	hasKeyLookup := false
	for _, op := range plan.Operators {
		if op.TableScan != nil {
			hasTableScan = true
		}
		if strings.Contains(op.PhysicalOp, "Key Lookup") {
			hasKeyLookup = true
		}
	}

	if totalCost > 0 {
		forecastPct := 0.0
		if hasTableScan {
			forecastPct += 40.0
		}
		if hasKeyLookup {
			forecastPct += 20.0
		}
		if len(plan.MissingIndexes) > 0 {
			forecastPct += 15.0
		}
		if forecastPct > 90 {
			forecastPct = 90
		}

		if forecastPct > 0 {
			sb.WriteString(fmt.Sprintf(`<div class="s-box info-box"><strong>Estimated Improvement Potential:</strong> Up to %.0f%% reduction in query cost with recommended optimizations.</div>`, forecastPct))
		} else {
			sb.WriteString(`<div class="s-box"><strong>Estimated Improvement Potential:</strong> Query appears well-optimized. Marginal gains possible.</div>`)
		}
	}

	if len(plan.Recommendations) > 0 {
		sb.WriteString(`<h4>Priority Recommendations</h4>`)
		sb.WriteString(`<table class="dt"><tr><th>Priority</th><th>Recommendation</th><th>Impact</th><th>Effort</th></tr>`)
		for i, rec := range plan.Recommendations {
			if i > 5 {
				break
			}
			sb.WriteString(fmt.Sprintf(`<tr><td>%d</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
				rec.Priority, html.EscapeString(rec.Title),
				html.EscapeString(rec.Impact), html.EscapeString(rec.Effort)))
		}
		sb.WriteString(`</table>`)
	}

	sb.WriteString(`</div>`)
	return sb.String()
}

func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}

func (r *HTMLReporter) renderTabRadio(id, label string, checked bool, badge ...int) string {
	chk := ""
	if checked {
		chk = " checked"
	}
	bHTML := ""
	if len(badge) > 0 && badge[0] > 0 {
		bHTML = fmt.Sprintf(` <span class="tab-badge">%d</span>`, badge[0])
	}
	return fmt.Sprintf(`    <input type="radio" name="tabs" id="tab-%s"%s>
    <label for="tab-%s">%s%s</label>
`, id, chk, id, html.EscapeString(label), bHTML)
}

func (r *HTMLReporter) renderSummaryTab(plan *models.PlanAnalysis) string {
	var sb strings.Builder
	sb.WriteString(`<div class="tab-panel">`)

	hs := plan.HealthScore.OverallScore
	hc := r.healthClass(hs)

	sb.WriteString(fmt.Sprintf(`
<div class="summary-hero">
  <div class="hero-score">
    <div class="hero-circle %s">%d</div>
    <div class="hero-label">%s</div>
  </div>
  <div class="hero-info">
    <h2>Executive Summary</h2>
    <div class="score-bar-c"><div class="score-bar" style="width:%d%%;background:%s;"></div></div>
    <div class="hero-stats">
      <div class="hero-stat"><span class="stat-v">%d</span>Health Score</div>
      <div class="hero-stat"><span class="stat-v">%d</span>Operators</div>
      <div class="hero-stat"><span class="stat-v">%d</span>Findings</div>
      <div class="hero-stat"><span class="stat-v">%.2f</span>Est. Cost</div>
    </div>
  </div>
</div>`, hc, hs, r.healthLabel(hs), hs, r.healthColor(hs), hs, len(plan.Operators), len(plan.Findings), plan.CostSummary.TotalEstimatedCost))

	pe := plan.ExecutiveSummary.PlainEnglish
	if pe.Summary != "" {
		sb.WriteString(fmt.Sprintf(`<div class="s-box"><strong>Summary:</strong> %s</div>`, html.EscapeString(pe.Summary)))
	}
	if pe.Impact != "" {
		sb.WriteString(fmt.Sprintf(`<div class="s-box impact-box"><strong>Business Impact:</strong> %s</div>`, html.EscapeString(pe.Impact)))
	}
	if pe.Analogy != "" {
		sb.WriteString(fmt.Sprintf(`<blockquote><strong>Analogy:</strong> %s</blockquote>`, html.EscapeString(pe.Analogy)))
	}
	if len(pe.Problems) > 0 {
		sb.WriteString(`<div class="s-box warn-box"><strong>Issues:</strong><ul>`)
		for _, p := range pe.Problems {
			sb.WriteString(fmt.Sprintf(`<li>%s</li>`, html.EscapeString(p)))
		}
		sb.WriteString(`</ul></div>`)
	}
	if len(pe.ActionItems) > 0 {
		sb.WriteString(`<div class="s-box info-box"><strong>Action Items:</strong><ul>`)
		for _, a := range pe.ActionItems {
			sb.WriteString(fmt.Sprintf(`<li>%s</li>`, html.EscapeString(a)))
		}
		sb.WriteString(`</ul></div>`)
	}

	sb.WriteString(r.renderHealthTable(plan))
	sb.WriteString(r.renderCostTable(plan))
	sb.WriteString(r.renderResourceAnalysis(plan))

	sb.WriteString(`</div>`)
	return sb.String()
}

func (r *HTMLReporter) renderResourceAnalysis(plan *models.PlanAnalysis) string {
	if len(plan.Operators) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(`<h3 class="sec-title">Resource Analysis</h3>`)

	totalCost := plan.CostSummary.TotalEstimatedCost

	// Cost by operator type
	type opStat struct {
		label   string
		total   float64
		count   int
		maxOp   string
		maxCost float64
		icon    string
	}
	byType := make(map[string]*opStat)
	for _, op := range plan.Operators {
		cat := r.operatorCategory(op.PhysicalOp)
		icon := r.opIcon(op.PhysicalOp)
		if _, ok := byType[cat]; !ok {
			byType[cat] = &opStat{label: cat, icon: icon}
		}
		byType[cat].total += op.EstimatedTotalSubtreeCost
		byType[cat].count++
		if op.EstimatedTotalSubtreeCost > byType[cat].maxCost {
			byType[cat].maxCost = op.EstimatedTotalSubtreeCost
			opCopy := op
			byType[cat].maxOp = fmt.Sprintf("%s (%s)", op.PhysicalOp, r.opTableShort(&opCopy))
		}
	}

	type kv struct {
		k string
		v *opStat
	}
	var sorted []kv
	for k, v := range byType {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].v.total > sorted[j].v.total
	})

	sb.WriteString(`<table class="dt"><tr><th>Operator Type</th><th>Count</th><th>Total Cost</th><th>%</th><th>Bar</th><th>Most Expensive</th></tr>`)
	for _, kv := range sorted {
		pct := 0.0
		if totalCost > 0 {
			pct = kv.v.total / totalCost * 100
		}
		barPct := pct
		if barPct > 100 {
			barPct = 100
		}
		sb.WriteString(fmt.Sprintf(`<tr><td>%s %s</td><td>%d</td><td class="m">%.4f</td><td>%.0f%%</td><td><div class="bmini"><div class="bfill" style="width:%.0f%%;background:%s;"></div></div></td><td>%s</td></tr>`,
			kv.v.icon, html.EscapeString(kv.v.label), kv.v.count, kv.v.total, pct, barPct, r.costBarColor(pct), html.EscapeString(kv.v.maxOp)))
	}
	sb.WriteString(`</table>`)

	// Top CPU consumers (by EstimateCPUms)
	type cpuStat struct {
		ref int
		cpu float64
	}
	var cpuIdx []cpuStat
	for i, op := range plan.Operators {
		if op.EstimateCPUms > 0 {
			cpuIdx = append(cpuIdx, cpuStat{i, op.EstimateCPUms})
		}
	}
	sort.Slice(cpuIdx, func(i, j int) bool {
		return cpuIdx[i].cpu > cpuIdx[j].cpu
	})
	if len(cpuIdx) > 5 {
		cpuIdx = cpuIdx[:5]
	}

	if len(cpuIdx) > 0 {
		sb.WriteString(`<h3 class="sec-title">Top CPU Consumers</h3><table class="dt"><tr><th>#</th><th>Operator</th><th>Table</th><th>CPU Cost</th><th>Est. Cost</th></tr>`)
		for i, cs := range cpuIdx {
			op := plan.Operators[cs.ref]
			tbl := r.opTableShort(&op)
			sb.WriteString(fmt.Sprintf(`<tr><td>%d</td><td>%s</td><td>%s</td><td class="m">%.4f</td><td class="m">%.4f</td></tr>`,
				i+1, html.EscapeString(op.PhysicalOp), html.EscapeString(tbl), cs.cpu, op.EstimatedTotalSubtreeCost))
		}
		sb.WriteString(`</table>`)
	}

	// Memory info
	if plan.QueryPlan != nil && plan.QueryPlan.HasMemoryGrant {
		m := plan.QueryPlan.MemoryGrantInfo
		sb.WriteString(fmt.Sprintf(`<h3 class="sec-title">Memory Grant</h3><table class="dt"><tr><th>Metric</th><th>Value</th></tr>
<tr><td>Granted Memory</td><td class="m">%d KB</td></tr>
<tr><td>Max Used Memory</td><td class="m">%d KB</td></tr>
<tr><td>Ideal Memory</td><td class="m">%d KB</td></tr>
<tr><td>Serial Required Memory</td><td class="m">%d KB</td></tr>
</table>`, m.GrantedMemory, m.MaxUsedMemory, m.IdealMemory, m.SerialRequiredMemory))
	}

	return sb.String()
}

func (r *HTMLReporter) renderHealthTable(plan *models.PlanAnalysis) string {
	bd := plan.HealthScore.Breakdown
	if bd == nil {
		bd = map[string]int{}
	}
	for _, k := range []string{"AccessMethods", "MemoryUsage", "JoinStrategy", "Parallelism", "Cardinality"} {
		if _, ok := bd[k]; !ok {
			bd[k] = 0
		}
	}
	return fmt.Sprintf(`
<h3 class="sec-title">Health Score Breakdown</h3>
<table class="dt">
  <tr><th>Category</th><th>Score</th><th>Max</th><th>Bar</th></tr>
  <tr><td>Access Methods</td><td>%d</td><td>40</td><td><div class="bmini"><div class="bfill" style="width:%d%%"></div></div></td></tr>
  <tr><td>Memory Usage</td><td>%d</td><td>20</td><td><div class="bmini"><div class="bfill" style="width:%d%%"></div></div></td></tr>
  <tr><td>Join Strategy</td><td>%d</td><td>20</td><td><div class="bmini"><div class="bfill" style="width:%d%%"></div></div></td></tr>
  <tr><td>Parallelism</td><td>%d</td><td>10</td><td><div class="bmini"><div class="bfill" style="width:%d%%"></div></div></td></tr>
  <tr><td>Cardinality</td><td>%d</td><td>10</td><td><div class="bmini"><div class="bfill" style="width:%d%%"></div></div></td></tr>
  <tr class="tr-total"><td><strong>Total</strong></td><td><strong>%d</strong></td><td><strong>100</strong></td><td><div class="bmini"><div class="bfill" style="width:%d%%;background:%s;"></div></div></td></tr>
</table>`,
		bd["AccessMethods"], bd["AccessMethods"]*100/40,
		bd["MemoryUsage"], bd["MemoryUsage"]*100/20,
		bd["JoinStrategy"], bd["JoinStrategy"]*100/20,
		bd["Parallelism"], bd["Parallelism"]*100/10,
		bd["Cardinality"], bd["Cardinality"]*100/10,
		plan.HealthScore.OverallScore, plan.HealthScore.OverallScore, r.healthColor(plan.HealthScore.OverallScore))
}

func (r *HTMLReporter) renderCostTable(plan *models.PlanAnalysis) string {
	if plan.CostSummary.OperatorCount == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`
<h3 class="sec-title">Cost Summary</h3>
<table class="dt">
  <tr><th>Metric</th><th>Value</th></tr>
  <tr><td>Total Estimated Cost</td><td class="m">%.4f</td></tr>
  <tr><td>CPU Cost</td><td class="m">%.4f</td></tr>
  <tr><td>I/O Cost</td><td class="m">%.4f</td></tr>
  <tr><td>Operator Count</td><td>%d</td></tr>
</table>`, plan.CostSummary.TotalEstimatedCost, plan.CostSummary.CPUCost, plan.CostSummary.IOCost, plan.CostSummary.OperatorCount))

	if len(plan.CostSummary.TopOperators) > 0 {
		sb.WriteString(`<h3 class="sec-title">Top 5 Costliest Operators</h3><table class="dt"><tr><th>#</th><th>Operator</th><th>Cost</th><th>Bar</th><th>Est. Rows</th><th>Actual Rows</th></tr>`)
		total := plan.CostSummary.TotalEstimatedCost
		for i, op := range plan.CostSummary.TopOperators {
			pct := 0.0
			if total > 0 {
				pct = op.TotalCost / total * 100
			}
			act := "-"
			if op.ActualRows > 0 {
				act = strconv.FormatInt(op.ActualRows, 10)
			}
			sb.WriteString(fmt.Sprintf(`<tr><td>%d</td><td>%s</td><td class="m">%.4f</td><td><div class="bmini"><div class="bfill" style="width:%.0f%%;background:#ef4444;"></div></div></td><td>%s</td><td>%s</td></tr>`,
				i+1, html.EscapeString(op.Name), op.TotalCost, pct, strconv.FormatInt(op.RowEstimate, 10), act))
		}
		sb.WriteString(`</table>`)
	}
	return sb.String()
}

func (r *HTMLReporter) renderFindingsTab(plan *models.PlanAnalysis) string {
	var sb strings.Builder
	sb.WriteString(`<div class="tab-panel">`)
	if len(plan.Findings) == 0 {
		sb.WriteString(`<p class="none">No performance issues detected.</p></div>`)
		return sb.String()
	}

	bySev := make(map[models.Severity][]models.Finding)
	for _, f := range plan.Findings {
		bySev[f.Severity] = append(bySev[f.Severity], f)
	}

	order := []models.Severity{models.SeverityCritical, models.SeverityHigh, models.SeverityMedium, models.SeverityLow}
	sc := r.sevColors()
	si := r.sevIcons()

	for _, sev := range order {
		ff := bySev[sev]
		if len(ff) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf(`
<div class="sg">
  <div class="sg-h" style="border-color:%s;">
    <span class="sg-i">%s</span>
    <span class="sg-t">%d %s Severity Finding(s)</span>
  </div>`, sc[sev], si[sev], len(ff), sev))

		for _, f := range ff {
			col := sc[f.Severity]
			confPct := int(f.Confidence * 100)
			confClass := "conf-low"
			if f.Confidence >= 0.7 {
				confClass = "conf-high"
			} else if f.Confidence >= 0.4 {
				confClass = "conf-med"
			}

			sb.WriteString(fmt.Sprintf(`
  <div class="fc" style="border-left-color:%s;">
    <div class="fc-header">
      <div class="fc-t">%s</div>
      <span class="conf-badge %s">%d%%</span>
    </div>`, col, html.EscapeString(f.Title), confClass, confPct))

			// Confidence badge tooltip
			sb.WriteString(fmt.Sprintf(`<div class="conf-tip">Evidence confidence: %d%%</div>`, confPct))

			if f.FindingType != "" {
				sb.WriteString(fmt.Sprintf(`<span class="fc-tag">%s</span>`, html.EscapeString(f.FindingType)))
			}
			if len(f.OperatorIDs) > 0 {
				sb.WriteString(fmt.Sprintf(`<span class="fc-tag">Operators: %v</span>`, f.OperatorIDs))
			}
			if f.Explanation != "" {
				sb.WriteString(fmt.Sprintf(`<div class="fc-l"><strong>Explanation:</strong> %s</div>`, html.EscapeString(f.Explanation)))
			}
			if f.Recommendation != "" {
				sb.WriteString(fmt.Sprintf(`<div class="fc-l fc-rec"><strong>Recommendation:</strong> %s</div>`, html.EscapeString(f.Recommendation)))
			}
			if f.Impact != "" {
				sb.WriteString(fmt.Sprintf(`<div class="fc-l"><strong>Impact:</strong> %s</div>`, html.EscapeString(f.Impact)))
			}
			if f.OperatorName != "" {
				sb.WriteString(fmt.Sprintf(`<div class="fc-l fc-op"><strong>Operator:</strong> %s (ID: %d)</div>`, html.EscapeString(f.OperatorName), f.OperatorID))
			}

			// Evidence trace
			if len(f.EvidenceTrace) > 0 {
				sb.WriteString(`<div class="ev-trace"><strong>Evidence:</strong> `)
				for i, ev := range f.EvidenceTrace {
					if i > 0 {
						sb.WriteString(`, `)
					}
					sb.WriteString(fmt.Sprintf(`<span class="ev-item" title="%s">%s</span>`,
						html.EscapeString(ev.Description), html.EscapeString(string(ev.Source))))
				}
				sb.WriteString(`</div>`)
			}

			sb.WriteString(`</div>`)
		}
		sb.WriteString(`</div>`)
	}
	sb.WriteString(`</div>`)
	return sb.String()
}

// planViewerData is the JSON blob embedded in the Plan Viewer tab for the JS renderer.
type planViewerData struct {
	Tree                  *models.Operator   `json:"tree"`
	TotalCost             float64            `json:"total_cost"`
	IsBatch               bool               `json:"is_batch"`
	Statements            []models.Statement `json:"statements,omitempty"`
	NonParallelPlanReason string             `json:"non_parallel_plan_reason,omitempty"`
	CompileTimeMs         int                `json:"compile_time_ms,omitempty"`
	CachedPlanSizeKB      int                `json:"cached_plan_size_kb,omitempty"`
	DegreeOfParallelism   int                `json:"degree_of_parallelism,omitempty"`
	OptimizationLevel     string             `json:"optimization_level,omitempty"`
}

func (r *HTMLReporter) renderPlanViewerTab(plan *models.PlanAnalysis) string {
	var sb strings.Builder
	sb.WriteString(`<div class="tab-panel" id="pv-root">`)

	if len(plan.Operators) == 0 || plan.QueryPlan == nil || plan.QueryPlan.RelOp == nil {
		// Fallback table when no tree is available
		sb.WriteString(`<p class="none">No operator tree found. Showing flat operator list.</p>`)
		sb.WriteString(`<table class="dt"><tr><th>ID</th><th>Physical Op</th><th>Logical Op</th><th>Est. Cost</th><th>Est. Rows</th><th>Actual Rows</th></tr>`)
		for _, op := range plan.Operators {
			act := "\u2014"
			if op.ActualRows > 0 { act = strconv.FormatInt(op.ActualRows, 10) }
			sb.WriteString(fmt.Sprintf(`<tr><td>%d</td><td>%s</td><td>%s</td><td class="m">%.4f</td><td>%s</td><td>%s</td></tr>`,
				op.ID, html.EscapeString(op.PhysicalOp), html.EscapeString(op.LogicalOp),
				op.EstimatedTotalSubtreeCost, fmt.Sprintf("%.0f", op.EstimateRows), act))
		}
		sb.WriteString(`</table></div>`)
		return sb.String()
	}

	// Build JSON payload for the renderer
	pvd := planViewerData{
		Tree:      plan.QueryPlan.RelOp,
		TotalCost: plan.CostSummary.TotalEstimatedCost,
		IsBatch:   plan.IsBatch,
		Statements: plan.Statements,
	}
	if plan.QueryPlan != nil {
		pvd.NonParallelPlanReason = plan.QueryPlan.NonParallelPlanReason
		pvd.CompileTimeMs = plan.QueryPlan.CompileTimeMs
		pvd.CachedPlanSizeKB = plan.QueryPlan.CachedPlanSize
		pvd.DegreeOfParallelism = plan.QueryPlan.DegreeOfParallelism
		pvd.OptimizationLevel = plan.QueryPlan.OptimizationLevel
	}
	jsonBytes, _ := json.Marshal(pvd)

	// Plan header bar
	sb.WriteString(`<div class="pv-plan-hdr" id="pv-plan-header"></div>`)

	// Toolbar
	sb.WriteString(`<div class="pv-toolbar">`)
	if plan.IsBatch && len(plan.Statements) > 1 {
		sb.WriteString(`<select id="pv-stmt-select" class="pv-search" title="Statement"></select>`)
	}
	sb.WriteString(`<button id="pv-fit" class="pv-btn" title="Fit to screen">Fit</button>`)
	sb.WriteString(`<button id="pv-zoom-in" class="pv-btn" title="Zoom in">+</button>`)
	sb.WriteString(`<button id="pv-zoom-out" class="pv-btn" title="Zoom out">-</button>`)
	sb.WriteString(`<input id="pv-search" class="pv-search" type="search" placeholder="Search operator\u2026" title="Highlight matching nodes">`)
	sb.WriteString(`</div>`)

	// Main canvas + side panels
	sb.WriteString(`<div class="pv-layout">`)

	// Top-5 sidebar
	sb.WriteString(`<div class="pv-top5-panel" id="pv-top5"></div>`)

	// SVG canvas wrapper
	sb.WriteString(`<div class="pv-canvas-wrap"><div class="pv-canvas" id="pv-canvas"></div>`)

	// Hover tooltip (hidden by default)
	sb.WriteString(`<div id="pv-tt" class="pv-tt"></div>`)

	// Properties panel (hidden until node clicked)
	sb.WriteString(`<div class="pv-props-panel" id="pv-props-panel"></div>`)
	sb.WriteString(`</div>`) // canvas-wrap

	sb.WriteString(`</div>`) // layout

	// Embedded plan data
	sb.WriteString(`<script type="application/json" id="plan-data">`)
	sb.Write(jsonBytes)
	sb.WriteString(`</script>`)

	// Inline SVG renderer
	sb.WriteString(r.planViewerScript())

	sb.WriteString(`</div>`)
	return sb.String()
}

// planViewerScript returns the self-contained vanilla-JS SVG renderer.
func (r *HTMLReporter) planViewerScript() string {
	return `<script>
(function(){
'use strict';
/* SSMS-style: root (SELECT) on LEFT, leaves on RIGHT, arrows flow right\u2192left */
var NODE_W=196,NODE_H=110,H_GAP=56,V_GAP=16,TOTAL_W=NODE_W+H_GAP;
var DATA=null,currentStmt=0,_maxRows=1,_searchTerm='',_initialized=false;
var _svgW=0,_svgH=0,_sc=1;

function loadData(){
  var el=document.getElementById('plan-data');
  if(!el)return null;
  try{return JSON.parse(el.textContent);}catch(e){return null;}
}

/* ---- Layout (SSMS: root at depth=0 \u2192 leftmost x=0) ---- */
function computeSize(n){
  if(!n.children||!n.children.length){n._size=1;return 1;}
  var s=0;for(var i=0;i<n.children.length;i++)s+=computeSize(n.children[i]);
  n._size=s;return s;
}
function assignPos(n,startY,depth){
  n._depth=depth;
  var SLOT=NODE_H+V_GAP;
  if(!n.children||!n.children.length){n._y=startY+SLOT/2-NODE_H/2;return depth;}
  var y=startY,maxD=depth;
  for(var i=0;i<n.children.length;i++){
    var d=assignPos(n.children[i],y,depth+1);
    if(d>maxD)maxD=d;
    y+=n.children[i]._size*SLOT;
  }
  var fy=n.children[0]._y,ly=n.children[n.children.length-1]._y;
  n._y=(fy+ly)/2;
  n._maxDepth=maxD;return maxD;
}
function assignX(n,maxD){
  /* SSMS: root at x=0, each child level moves right */
  n._x=n._depth*TOTAL_W;
  if(n.children)for(var i=0;i<n.children.length;i++)assignX(n.children[i],maxD);
}
function collectNodes(n,arr){
  arr.push(n);
  if(n.children)for(var i=0;i<n.children.length;i++)collectNodes(n.children[i],arr);
  return arr;
}
function findMaxRows(n){
  var m=Math.max(n.estimate_rows||0,n.actual_rows||0);
  if(n.children)for(var i=0;i<n.children.length;i++){var c=findMaxRows(n.children[i]);if(c>m)m=c;}
  return m;
}
/* Mark critical path: root \u2192 max-cost child at each step */
function markCriticalPath(n){
  n._critical=true;
  if(!n.children||!n.children.length)return;
  var best=n.children[0];
  for(var i=1;i<n.children.length;i++)
    if((n.children[i].estimated_total_subtree_cost||0)>(best.estimated_total_subtree_cost||0))best=n.children[i];
  markCriticalPath(best);
}

/* ---- Helpers ---- */
function fmtN(n){
  if(n===null||n===undefined||n===0)return '0';
  if(n>=1e9)return (n/1e9).toFixed(1)+'B';
  if(n>=1e6)return (n/1e6).toFixed(1)+'M';
  if(n>=1e3)return (n/1e3).toFixed(1)+'K';
  return ''+Math.round(n);
}
function trunc(s,max){if(!s)return '';return s.length>max?s.substring(0,max-1)+'\u2026':s;}
function edgeW(rows){
  if(!rows||!_maxRows||_maxRows===0)return 1.5;
  return Math.max(1.5,Math.min(10,(Math.log2(rows+1)/Math.log2(_maxRows+1))*10));
}
function costColor(pct,warn){
  if(warn)return '#a855f7';
  if(pct>=20)return '#ef4444';
  if(pct>=10)return '#f97316';
  if(pct>=5)return '#eab308';
  return '#22c55e';
}
function getTableName(n){
  if(n.index_scan&&n.index_scan.object){
    var o=n.index_scan.object;
    return o.table||(o.schema?o.schema+'.'+o.table:null);
  }
  if(n.table_scan&&n.table_scan.object)return n.table_scan.object.table;
  return null;
}
function getIndexName(n){
  if(n.index_scan&&n.index_scan.object&&n.index_scan.object.index)return n.index_scan.object.index;
  return null;
}
function getPredSnippet(n){
  if(n.predicate&&n.predicate.scalar_string)return n.predicate.scalar_string;
  if(n.seek_predicates&&n.seek_predicates.length>0){
    var sp=n.seek_predicates[0];
    if(sp.prefix_predicate&&sp.prefix_predicate.length>0)return sp.prefix_predicate[0].scalar_string||null;
  }
  return null;
}
function getOutputCols(n){
  if(n.output_list&&n.output_list.length>0)
    return n.output_list.map(function(c){return c.column;}).join(', ');
  return null;
}

/* ---- SVG helpers ---- */
var SVG_NS='http://www.w3.org/2000/svg';
function svgEl(tag,attrs){
  var el=document.createElementNS(SVG_NS,tag);
  if(attrs)Object.keys(attrs).forEach(function(k){el.setAttribute(k,attrs[k]);});
  return el;
}
function svgText(x,y,txt,attrs){
  var el=svgEl('text',Object.assign({x:x,y:y},attrs||{}));
  el.textContent=txt;return el;
}

/* ---- Tooltip ---- */
function showTooltip(n,pct,e){
  var tt=document.getElementById('pv-tt');if(!tt)return;
  var opName=n.physical_op||n.logical_op||'';
  var tbl=getTableName(n);
  var idx=getIndexName(n);
  var pred=getPredSnippet(n);
  var outs=getOutputCols(n);
  var hasAct=n.actual_rows>0||n.actual_executions>0;
  var html='<div class="pv-tt-op">'+escH(opName)+'</div>';
  if(tbl){
    var objStr=tbl+(idx?' \u25b8 '+idx:'');
    html+='<div class="pv-tt-row"><span class="pv-tt-lbl">Object</span><span class="pv-tt-val">'+escH(objStr)+'</span></div>';
  }
  html+='<div class="pv-tt-row"><span class="pv-tt-lbl">Cost</span><span class="pv-tt-val" style="color:'+costColor(pct,false)+'">'+pct.toFixed(2)+'% &middot; '+(n.estimated_total_subtree_cost||0).toFixed(4)+'</span></div>';
  if(hasAct){
    html+='<div class="pv-tt-row"><span class="pv-tt-lbl">Rows</span><span class="pv-tt-val">Est '+fmtN(n.estimate_rows)+' &middot; Act '+fmtN(n.actual_rows)+'</span></div>';
  }else{
    html+='<div class="pv-tt-row"><span class="pv-tt-lbl">Est Rows</span><span class="pv-tt-val">'+fmtN(n.estimate_rows)+'</span></div>';
  }
  if(n.avg_row_size)html+='<div class="pv-tt-row"><span class="pv-tt-lbl">Row Size</span><span class="pv-tt-val">'+n.avg_row_size+' B</span></div>';
  if(pred)html+='<div class="pv-tt-pred">'+escH(trunc(pred,72))+'</div>';
  if(outs)html+='<div class="pv-tt-out">'+escH(trunc(outs,72))+'</div>';
  html+='<div class="pv-tt-hint">Click for full details</div>';
  tt.innerHTML=html;
  var vw=window.innerWidth,vh=window.innerHeight;
  var lx=e.clientX+16,ly=e.clientY-10;
  tt.style.display='block';
  /* keep inside viewport */
  var tw=tt.offsetWidth||260,th=tt.offsetHeight||120;
  if(lx+tw>vw-8)lx=e.clientX-tw-8;
  if(ly+th>vh-8)ly=vh-th-8;
  tt.style.left=lx+'px';tt.style.top=ly+'px';
}
function hideTooltip(){
  var tt=document.getElementById('pv-tt');if(tt)tt.style.display='none';
}
function escH(s){
  if(!s)return '';
  return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

/* ---- Draw a single node ---- */
function drawNode(n,totalCost,searchTerm){
  var pct=totalCost>0?(n.estimated_total_subtree_cost||0)/totalCost*100:0;
  var hasWarn=(n.warnings&&n.warnings.length>0)||(n.op_wait_stats&&n.op_wait_stats.length>0);
  var color=costColor(pct,hasWarn);
  var hasAct=(n.actual_rows>0||n.actual_executions>0);
  var opName=n.physical_op||n.logical_op||'Unknown';
  var tblName=getTableName(n);
  var idxName=getIndexName(n);
  var pred=getPredSnippet(n);
  var outs=getOutputCols(n);

  var g=svgEl('g',{transform:'translate('+n._x+','+n._y+')','class':'pv-node','data-nid':n.id});

  /* critical path: dashed orange glow rect behind node */
  if(n._critical){
    g.appendChild(svgEl('rect',{x:-2,y:-2,width:NODE_W+4,height:NODE_H+4,rx:8,
      fill:'none',stroke:'#f97316','stroke-width':2.5,'stroke-dasharray':'6,3','pointer-events':'none'}));
  }

  /* background */
  g.appendChild(svgEl('rect',{x:0,y:0,width:NODE_W,height:NODE_H,rx:6,
    fill:'var(--pv-bg)',stroke:color,'stroke-width':hasWarn?2.5:1.5}));
  /* left stripe */
  g.appendChild(svgEl('rect',{x:0,y:0,width:4,height:NODE_H,rx:3,fill:color}));

  /* operator name */
  g.appendChild(svgText(10,16,trunc(opName,23),{
    fill:'var(--tx)','font-size':11,'font-weight':700,'font-family':'inherit'}));

  /* warning / critical badge */
  if(hasWarn||n._critical){
    var badge=n._critical&&!hasWarn?{bg:'#f97316',txt:'!'}:{bg:'#a855f7',txt:'!'};
    if(hasWarn)badge={bg:'#f97316',txt:'!'};
    var wc=svgEl('circle',{cx:NODE_W-12,cy:10,r:7,fill:badge.bg});
    var wt=svgText(NODE_W-12,14,badge.txt,{'text-anchor':'middle',fill:'#fff','font-size':8,'font-weight':700});
    g.appendChild(wc);g.appendChild(wt);
  }

  /* divider */
  g.appendChild(svgEl('line',{x1:8,y1:22,x2:NODE_W-8,y2:22,stroke:'var(--bd)','stroke-width':1}));

  /* object: table + index */
  var yOff=0;
  if(tblName){
    var objTxt=idxName?trunc(idxName,22)+' \u25b8 '+trunc(tblName,14):trunc(tblName,26);
    g.appendChild(svgText(10,34,objTxt,{fill:'#60a5fa','font-size':10,'font-family':'inherit'}));
    yOff=13;
  }

  /* cost bar */
  var barY=34+yOff;
  g.appendChild(svgEl('rect',{x:8,y:barY,width:NODE_W-16,height:4,rx:2,fill:'var(--pv-bar)'}));
  var bw=Math.max(0,Math.min(NODE_W-16,(pct/100)*(NODE_W-16)));
  if(bw>0)g.appendChild(svgEl('rect',{x:8,y:barY,width:bw,height:4,rx:2,fill:color}));

  /* cost% \u00b7 subtree cost */
  var cy1=barY+14;
  g.appendChild(svgText(8,cy1,pct.toFixed(1)+'% \u00b7 '+(n.estimated_total_subtree_cost||0).toFixed(4),
    {fill:color,'font-size':10,'font-weight':700,'font-family':'monospace'}));

  /* rows */
  var cy2=cy1+13;
  var rowTxt;
  if(hasAct){
    var ratio=(n.estimate_rows>0&&n.actual_rows>0)?n.actual_rows/n.estimate_rows:0;
    var ratioStr=ratio>=2?'\u00d7'+fmtN(ratio)+'\u2191':(ratio>0&&ratio<0.5?'\u00d7'+fmtN(1/ratio)+'\u2193':'');
    rowTxt='Est:'+fmtN(n.estimate_rows)+' Act:'+fmtN(n.actual_rows)+(ratioStr?' '+ratioStr:'');
  }else{
    rowTxt='Est: '+fmtN(n.estimate_rows)+' rows'+(n.avg_row_size?' ('+n.avg_row_size+'B)':'');
  }
  g.appendChild(svgText(8,cy2,trunc(rowTxt,30),{fill:'var(--tx2)','font-size':10,'font-family':'inherit'}));

  /* CPU / reads / spills */
  var cy3=cy2+12;
  var parts=[];
  if(n.actual_cpu_ms>0)parts.push('CPU:'+n.actual_cpu_ms.toFixed(0)+'ms');
  if(n.actual_logical_reads>0)parts.push('Rds:'+fmtN(n.actual_logical_reads));
  if(n.actual_spills>0)parts.push('Spill:'+n.actual_spills);
  if(parts.length>0)
    g.appendChild(svgText(8,cy3,trunc(parts.join(' '),30),{fill:'#f97316','font-size':9,'font-family':'inherit'}));

  /* predicate snippet */
  var cy4=cy3+11;
  if(pred){
    g.appendChild(svgText(8,cy4,'\u25b8 '+trunc(pred,28),{fill:'var(--tx3)','font-size':9,'font-style':'italic','font-family':'inherit'}));
  }else if(outs){
    var colList=outs.split(',').slice(0,3).join(',')+(outs.split(',').length>3?'\u2026':'');
    g.appendChild(svgText(8,cy4,'\u25ba '+trunc(colList,28),{fill:'var(--tx3)','font-size':9,'font-family':'monospace'}));
  }

  /* search highlight / fade */
  var matchSearch=searchTerm&&(opName.toLowerCase().indexOf(searchTerm)>=0||
    (tblName&&tblName.toLowerCase().indexOf(searchTerm)>=0)||
    (pred&&pred.toLowerCase().indexOf(searchTerm)>=0));
  if(searchTerm){
    if(matchSearch){
      g.appendChild(svgEl('rect',{x:0,y:0,width:NODE_W,height:NODE_H,rx:6,
        fill:'rgba(37,99,235,0.12)',stroke:'#2563eb','stroke-width':2.5,'pointer-events':'none'}));
    }else{
      g.setAttribute('opacity','0.22');
    }
  }

  g.style.cursor='pointer';
  (function(node,p){
    g.addEventListener('mousemove',function(e){showTooltip(node,p,e);});
    g.addEventListener('mouseleave',function(){hideTooltip();});
    g.addEventListener('click',function(e){e.stopPropagation();hideTooltip();showProps(node,p,totalCost);});
  })(n,pct);

  return g;
}

/* ---- Draw edge (SSMS style: right\u2192left; child.left \u2192 parent.right) ---- */
function drawEdge(parent,child){
  var rows=child.actual_rows||child.estimate_rows||0;
  var sw=edgeW(rows);
  var hasAct=child.actual_rows>0;
  var clr=hasAct?'rgba(37,99,235,0.70)':'rgba(148,163,184,0.60)';
  var dash=hasAct?'':'6,4';
  /* child is RIGHT of parent: arrow from child.left \u2192 parent.right */
  var sx=child._x,       sy=child._y+NODE_H/2;
  var tx=parent._x+NODE_W, ty=parent._y+NODE_H/2;
  var mx=(sx+tx)/2;
  var g=svgEl('g',{'class':'pv-edge'});
  var path=svgEl('path',{
    d:'M '+sx+' '+sy+' C '+mx+' '+sy+', '+mx+' '+ty+', '+tx+' '+ty,
    fill:'none',stroke:clr,'stroke-width':sw,'stroke-dasharray':dash,'stroke-linecap':'round'});
  g.appendChild(path);
  /* arrowhead at parent right edge, pointing INTO the parent (pointing left) */
  var ah=svgEl('polygon',{
    points:tx+','+ty+' '+(tx+8)+','+(ty-4)+' '+(tx+8)+','+(ty+4),
    fill:clr,'pointer-events':'none'});
  g.appendChild(ah);
  /* row label on thick edges */
  if(sw>=4&&rows>0){
    var lx=(sx+tx)/2,ly=(sy+ty)/2;
    var bg=svgEl('rect',{x:lx-20,y:ly-8,width:40,height:14,rx:3,
      fill:'var(--bg)',opacity:0.9,'pointer-events':'none'});
    var lt=svgText(lx,ly+3,fmtN(rows),{'text-anchor':'middle',fill:clr,
      'font-size':9,'font-weight':700,'pointer-events':'none','font-family':'monospace'});
    g.appendChild(bg);g.appendChild(lt);
  }
  return g;
}

/* ---- Render full plan ---- */
function renderPlan(root,container,searchTerm){
  container.innerHTML='';
  if(!root)return;
  computeSize(root);
  var maxD=assignPos(root,0,0);
  assignX(root,maxD);
  markCriticalPath(root);
  _maxRows=findMaxRows(root);
  var allNodes=collectNodes(root,[]);
  var totalCost=root.estimated_total_subtree_cost||DATA.total_cost||1;
  if(totalCost<=0)totalCost=1;
  var VPAD=28;
  /* natural SVG coordinate space — no pre-scaling */
  _svgH=root._size*(NODE_H+V_GAP)+V_GAP*4+VPAD*2;
  _svgW=(maxD+1)*TOTAL_W+NODE_W+20;
  /* initial scale: fit canvas HEIGHT so nodes are full-size vertically;
     clamp between 0.72 (floor for readability) and 1.0 */
  var cH=container.clientHeight||560;
  _sc=Math.min(1.0,Math.max(0.72,(cH-24)/_svgH));
  var svg=svgEl('svg',{
    viewBox:'0 0 '+_svgW+' '+_svgH,
    width:Math.round(_svgW*_sc),
    height:Math.round(_svgH*_sc)
  });
  svg.style.display='block'; /* remove inline-block gap */
  /* edges first */
  var eG=svgEl('g',{'class':'pv-edges'});
  for(var i=0;i<allNodes.length;i++){
    var n=allNodes[i];
    if(n.children)for(var j=0;j<n.children.length;j++)eG.appendChild(drawEdge(n,n.children[j]));
  }
  svg.appendChild(eG);
  /* nodes on top */
  var nG=svgEl('g',{'class':'pv-nodes'});
  for(var i=0;i<allNodes.length;i++)nG.appendChild(drawNode(allNodes[i],totalCost,searchTerm));
  svg.appendChild(nG);
  container.appendChild(svg);
  /* scroll to top-left so SELECT (root) is visible */
  container.scrollLeft=0;container.scrollTop=0;
  renderTop5(allNodes,totalCost);
}

/* ---- Fit / zoom / pan ---- */
/* applyT: resize SVG via width/height attrs so overflow:auto scrollbars reflect true size */
var _dragging=false,_lx=0,_ly=0;
function applyT(container){
  var svg=container.querySelector('svg');
  if(!svg||!_svgW||!_svgH)return;
  svg.setAttribute('width',Math.round(_svgW*_sc));
  svg.setAttribute('height',Math.round(_svgH*_sc));
}
function fitToScreen(container){
  if(!_svgW||!_svgH)return;
  var cW=container.clientWidth||800,cH=container.clientHeight||560;
  _sc=Math.min(1.0,Math.min((cW-12)/_svgW,(cH-12)/_svgH));
  applyT(container);
  container.scrollLeft=0;container.scrollTop=0;
}
function setupZoom(container){
  /* wheel zoom: zoom toward cursor position */
  container.addEventListener('wheel',function(e){
    e.preventDefault();
    var oldSc=_sc;
    _sc=Math.max(0.15,Math.min(4,_sc*(e.deltaY>0?0.88:1.14)));
    /* adjust scroll to keep point under cursor fixed */
    var rect=container.getBoundingClientRect();
    var cx=e.clientX-rect.left+container.scrollLeft;
    var cy=e.clientY-rect.top+container.scrollTop;
    applyT(container);
    container.scrollLeft=cx*(_sc/oldSc)-( e.clientX-rect.left);
    container.scrollTop =cy*(_sc/oldSc)-( e.clientY-rect.top);
  },{passive:false});
  /* drag to pan */
  container.addEventListener('mousedown',function(e){
    if(e.target.closest('.pv-node')||e.button!==0)return;
    _dragging=true;_lx=e.clientX;_ly=e.clientY;
    container.style.cursor='grabbing';e.preventDefault();
  });
  document.addEventListener('mousemove',function(e){
    if(!_dragging)return;
    container.scrollLeft-=(e.clientX-_lx);
    container.scrollTop -=(e.clientY-_ly);
    _lx=e.clientX;_ly=e.clientY;
  });
  document.addEventListener('mouseup',function(){
    if(_dragging){_dragging=false;container.style.cursor='grab';}
  });
  container.style.cursor='grab';
  /* buttons */
  var fitBtn=document.getElementById('pv-fit');
  if(fitBtn)fitBtn.onclick=function(){fitToScreen(container);};
  var ziBtn=document.getElementById('pv-zoom-in');
  if(ziBtn)ziBtn.onclick=function(){_sc=Math.min(4,_sc*1.25);applyT(container);};
  var zoBtn=document.getElementById('pv-zoom-out');
  if(zoBtn)zoBtn.onclick=function(){_sc=Math.max(0.15,_sc/1.25);applyT(container);};
}

/* ---- Properties panel ---- */
function showProps(node,costPct,totalCost){
  var panel=document.getElementById('pv-props-panel');if(!panel)return;
  panel.innerHTML=buildPropsHTML(node,costPct,totalCost);
  panel.classList.add('open');
}
function propRow(label,val){
  if(val===null||val===undefined||val===''||(typeof val==='number'&&val===0))return '';
  return '<div class="pv-pr"><span class="pv-pl">'+label+'</span><span class="pv-pv">'+val+'</span></div>';
}
function propSection(title,body){
  if(!body)return '';
  return '<details class="pv-sec" open><summary class="pv-sec-hdr">'+title+'</summary><div class="pv-sec-body">'+body+'</div></details>';
}
function buildPropsHTML(n,pct,totalCost){
  var hasAct=n.actual_rows>0||n.actual_executions>0;

  var gen=propRow('Node ID',n.node_id||n.id)
    +propRow('Physical Op',n.physical_op)
    +propRow('Logical Op',n.logical_op)
    +propRow('Exec Mode',n.estimated_execution_mode)
    +propRow('Storage',n.storage)
    +propRow('Parallel',n.parallel?'Yes ('+(n.parallel_thread_count||'?')+' threads)':null)
    +propRow('Avg Row Size',n.avg_row_size?n.avg_row_size+' bytes':null)
    +propRow('Adaptive Threshold',n.adaptive_threshold_rows||null)
    +propRow('Critical Path',n._critical?'Yes (highest-cost chain)':null);

  var io=propRow('Est Rows',fmtN(n.estimate_rows))
    +propRow('Est Rows Read',n.estimated_rows_read>0?fmtN(n.estimated_rows_read):null)
    +propRow('Act Rows',hasAct?fmtN(n.actual_rows):'\u2014')
    +propRow('Act Executions',hasAct?n.actual_executions:null)
    +propRow('Act CPU ms',n.actual_cpu_ms>0?n.actual_cpu_ms.toFixed(2):null)
    +propRow('Act Elapsed ms',n.actual_elapsed_ms>0?n.actual_elapsed_ms.toFixed(2):null)
    +propRow('Logical Reads',n.actual_logical_reads>0?fmtN(n.actual_logical_reads):null)
    +propRow('Physical Reads',n.actual_physical_reads>0?fmtN(n.actual_physical_reads):null)
    +propRow('Spills',n.actual_spills>0?n.actual_spills:null)
    +propRow('Rebinds',n.estimate_rebinds>0?n.estimate_rebinds:null)
    +propRow('Rewinds',n.estimate_rewinds>0?n.estimate_rewinds:null);

  var cost=propRow('Est Cost',n.estimated_total_subtree_cost?(n.estimated_total_subtree_cost).toFixed(6):null)
    +propRow('CPU Cost',n.estimate_cpu_ms>0?n.estimate_cpu_ms.toFixed(6):null)
    +propRow('I/O Cost',n.estimated_ios>0?n.estimated_ios.toFixed(6):null)
    +propRow('Cost % of Plan',pct.toFixed(2)+'%')
    +propRow('Table Cardinality',n.table_cardinality>0?fmtN(n.table_cardinality):null);

  var idx='';
  if(n.index_scan&&n.index_scan.object){
    var o=n.index_scan.object;
    idx=propRow('Schema',o.schema||null)
      +propRow('Table',o.table)
      +propRow('Index',o.index||null)
      +propRow('Index Kind',n.index_scan.index_kind)
      +propRow('Scan Type',n.index_scan.scan_type)
      +propRow('Ordered',n.index_scan.ordered?'Yes':null)
      +propRow('Forced Index',n.index_scan.forced_index?'Yes':null);
  }else if(n.table_scan&&n.table_scan.object){
    var ts=n.table_scan.object;
    idx=propRow('Schema',ts.schema||null)+propRow('Table',ts.table);
  }

  var pred='';
  if(n.predicate&&n.predicate.scalar_string)
    pred+=propRow('Residual','<code>'+escH(n.predicate.scalar_string)+'</code>');
  if(n.seek_predicates){
    for(var i=0;i<n.seek_predicates.length;i++){
      var sp=n.seek_predicates[i];
      if(sp.prefix_predicate)for(var j=0;j<sp.prefix_predicate.length;j++)
        pred+=propRow('Seek Prefix','<code>'+escH(sp.prefix_predicate[j].scalar_string||'')+'</code>');
    }
  }

  var join='';
  if(n.hash){
    var bk=(n.hash.hash_keys_build||[]).map(function(c){return c.column;}).join(', ');
    var pk=(n.hash.hash_keys_probe||[]).map(function(c){return c.column;}).join(', ');
    join=propRow('Build Keys',bk)+propRow('Probe Keys',pk)
      +propRow('Probe Residual',n.hash.probe_residual)
      +propRow('Build Residual',n.hash.build_residual);
  }else if(n.nested_loops){
    var refs=(n.nested_loops.outer_references||[]).map(function(c){return c.column;}).join(', ');
    join=propRow('Outer References',refs)
      +propRow('Predicate',n.nested_loops.predicate)
      +propRow('Adaptive',n.nested_loops.outer_is_adaptive?'Yes':null);
  }else if(n.merge){
    var ic=(n.merge.inner_side_join_columns||[]).map(function(c){return c.column;}).join(', ');
    var oc=(n.merge.outer_side_join_columns||[]).map(function(c){return c.column;}).join(', ');
    join=propRow('Inner Join Cols',ic)+propRow('Outer Join Cols',oc);
  }

  var mem='';
  if(n.memory_fractions){
    mem=propRow('Input Fraction',n.memory_fractions.input?(n.memory_fractions.input).toFixed(3):null)
      +propRow('Output Fraction',n.memory_fractions.output?(n.memory_fractions.output).toFixed(3):null);
  }

  var out='';
  if(n.output_list&&n.output_list.length>0){
    var cols=n.output_list.map(function(c){return c.column;}).join(', ');
    out=propRow('Columns','<code>'+escH(cols)+'</code>');
  }

  var threads='';
  if(n.runtime_counters&&n.runtime_counters.length>1){
    var tbl2='<table class="pv-thr-tbl"><tr><th>Thd</th><th>Act Rows</th><th>CPU ms</th><th>Reads</th></tr>';
    for(var i=0;i<n.runtime_counters.length;i++){
      var rc=n.runtime_counters[i];
      tbl2+='<tr><td>'+(rc.thread||i)+'</td><td>'+fmtN(rc.actual_rows)+'</td><td>'+(rc.actual_cpu_ms||0).toFixed(1)+'</td><td>'+fmtN(rc.actual_logical_reads)+'</td></tr>';
    }
    tbl2+='</table>';threads=propRow('Per-thread',tbl2);
  }

  var waits='';
  if(n.op_wait_stats&&n.op_wait_stats.length>0){
    for(var i=0;i<n.op_wait_stats.length;i++){
      var w=n.op_wait_stats[i];
      waits+=propRow(w.wait_type,(w.wait_time_ms||0)+'ms (count:'+(w.wait_count||0)+')');
    }
  }

  var stats='';
  if(n.op_statistics_info&&n.op_statistics_info.length>0){
    for(var i=0;i<n.op_statistics_info.length;i++){
      var s=n.op_statistics_info[i];
      stats+=propRow(s.object,'Updated: '+(s.last_update||'unknown')+
        (s.sampling_percent?' Sample: '+s.sampling_percent.toFixed(1)+'%':'')+
        (s.modification_count?' Mods: '+s.modification_count:''));
    }
  }

  var closeBtn='<button onclick="document.getElementById(\'pv-props-panel\').classList.remove(\'open\')" class="pv-close-btn">\u2715</button>';
  return '<div class="pv-props-hdr"><span class="pv-props-title">'+escH(trunc(n.physical_op||'Operator',24))+'</span>'+closeBtn+'</div>'
    +propSection('General',gen)
    +propSection('I/O &amp; Rows',io)
    +propSection('Cost',cost)
    +propSection('Object',idx)
    +propSection('Predicates',pred)
    +propSection('Output Columns',out)
    +propSection('Join Details',join)
    +propSection('Memory Fractions',mem)
    +propSection('Parallelism (Per-Thread)',threads)
    +propSection('Wait Statistics',waits)
    +propSection('Statistics Objects',stats);
}

/* ---- Top-5 grid panel ---- */
function renderTop5(allNodes,totalCost){
  var el=document.getElementById('pv-top5');if(!el)return;
  var sorted=allNodes.slice().sort(function(a,b){return (b.estimated_total_subtree_cost||0)-(a.estimated_total_subtree_cost||0);});
  var top=sorted.slice(0,5);
  var html='<div class="pv-t5-hdr">Top 5 by Cost</div>'
    +'<div class="pv-t5-head-row">'
    +'<span class="pv-t5-col-rank">#</span>'
    +'<span class="pv-t5-col-pct">%</span>'
    +'<span class="pv-t5-col-name">Operator &middot; Object</span>'
    +'<span class="pv-t5-col-rows">Est Rows</span>'
    +'</div>';
  for(var i=0;i<top.length;i++){
    var n=top[i];
    var pct=totalCost>0?(n.estimated_total_subtree_cost||0)/totalCost*100:0;
    var tn=getTableName(n);
    var ix=getIndexName(n);
    var color=costColor(pct,false);
    var opLabel=n.physical_op||n.logical_op||'?';
    var objLabel=ix?ix+(tn?' \u25b8 '+tn:''):(tn||'');
    html+='<div class="pv-t5-row" data-nid="'+n.id+'">'
      +'<span class="pv-t5-col-rank" style="color:'+color+'">'+(i+1)+'</span>'
      +'<div class="pv-t5-col-pct">'
        +'<div class="pv-t5-bar"><div class="pv-t5-bar-fill" style="width:'+Math.min(100,pct).toFixed(1)+'%;background:'+color+'"></div></div>'
        +'<span class="pv-t5-pct-txt" style="color:'+color+'">'+pct.toFixed(1)+'%</span>'
      +'</div>'
      +'<div class="pv-t5-col-name">'
        +'<div class="pv-t5-op">'+escH(opLabel)+'</div>'
        +(objLabel?'<div class="pv-t5-obj">'+escH(trunc(objLabel,26))+'</div>':'')
      +'</div>'
      +'<span class="pv-t5-col-rows">'+fmtN(n.estimate_rows)+'</span>'
      +'</div>';
  }
  el.innerHTML=html;
  el.querySelectorAll('.pv-t5-row').forEach(function(item){
    item.addEventListener('click',function(){
      var nid=this.getAttribute('data-nid');
      var g=document.querySelector('#pv-canvas [data-nid="'+nid+'"]');
      if(g)g.dispatchEvent(new MouseEvent('click',{bubbles:false}));
    });
  });
}

/* ---- Plan header ---- */
function renderPlanHeader(){
  var el=document.getElementById('pv-plan-header');if(!el||!DATA)return;
  var parts=[];
  if(DATA.optimization_level)parts.push('Opt: '+DATA.optimization_level);
  if(DATA.degree_of_parallelism)parts.push('DOP: '+DATA.degree_of_parallelism);
  if(DATA.compile_time_ms)parts.push('Compile: '+DATA.compile_time_ms+'ms');
  if(DATA.cached_plan_size_kb)parts.push('Cache: '+DATA.cached_plan_size_kb+'KB');
  if(DATA.non_parallel_plan_reason)parts.push('Serial: '+DATA.non_parallel_plan_reason);
  el.textContent=parts.join(' \u00b7 ');
}

/* ---- Statement selector ---- */
function setupStatements(container){
  var sel=document.getElementById('pv-stmt-select');
  if(!sel||!DATA||!DATA.statements||DATA.statements.length<=1)return;
  sel.innerHTML='';
  for(var i=0;i<DATA.statements.length;i++){
    var s=DATA.statements[i];
    var opt=document.createElement('option');
    opt.value=i;
    opt.textContent='Stmt '+(i+1)+': '+trunc(s.statement_text||'',40)+' ('+((s.cost_percent||0).toFixed(1))+'%)';
    sel.appendChild(opt);
  }
  sel.addEventListener('change',function(){
    currentStmt=parseInt(this.value);init();
  });
}

/* ---- Search ---- */
function setupSearch(container){
  var si=document.getElementById('pv-search');if(!si)return;
  si.addEventListener('input',function(){
    _searchTerm=(this.value||'').toLowerCase();
    var root=getRoot();
    if(root)renderPlan(root,container,_searchTerm);
  });
}

function getRoot(){
  if(!DATA)return null;
  if(DATA.is_batch&&DATA.statements&&DATA.statements.length>0){
    var s=DATA.statements[currentStmt];
    return s?s.root_operator:DATA.tree;
  }
  return DATA.tree;
}

/* ---- Init ---- */
function init(){
  var container=document.getElementById('pv-canvas');if(!container)return;
  if(!DATA){DATA=loadData();if(!DATA)return;}
  var root=getRoot();
  if(!root){container.innerHTML='<p class="none">No plan tree available.</p>';return;}
  renderPlan(root,container,_searchTerm);
  if(!_initialized){
    setupZoom(container);
    setupSearch(container);
    setupStatements(container);
    renderPlanHeader();
    _initialized=true;
  }
}

/* Activate when tab radio is checked */
var radio=document.getElementById('tab-planviewer');
if(radio){radio.addEventListener('change',function(){if(this.checked)setTimeout(init,30);});}
/* Also init immediately if already on this tab */
if(radio&&radio.checked)init();
})();
</script>`
}

func (r *HTMLReporter) renderRecommendationsTab(plan *models.PlanAnalysis) string {
	var sb strings.Builder
	sb.WriteString(`<div class="tab-panel">`)
	if len(plan.Recommendations) == 0 {
		sb.WriteString(`<p class="none">No recommendations at this time.</p></div>`)
		return sb.String()
	}

	for i, rec := range plan.Recommendations {
		sc := r.severityColor(rec.Severity)
		sb.WriteString(fmt.Sprintf(`
<div class="rc">
  <div class="rc-h" style="border-left-color:%s;">
    <span class="rc-n">%d</span>
    <span class="rc-t">%s</span>
    <span class="rc-b" style="background:%s;">%s</span>
  </div>
  <div class="rc-body">
    <div class="rc-meta"><strong>Type:</strong> %s &middot; <strong>Effort:</strong> %s &middot; <strong>Impact:</strong> %s</div>`,
			sc, i+1, html.EscapeString(rec.Title), sc, rec.Severity,
			html.EscapeString(rec.Type), html.EscapeString(rec.Effort), html.EscapeString(rec.Impact)))
		if rec.Description != "" {
			sb.WriteString(fmt.Sprintf(`<div class="rc-d">%s</div>`, html.EscapeString(rec.Description)))
		}
		if rec.SQL != "" {
			sb.WriteString(fmt.Sprintf(`<pre class="rc-sql">%s</pre>`, html.EscapeString(rec.SQL)))
		}
		sb.WriteString(`</div></div>`)
	}
	sb.WriteString(`</div>`)
	return sb.String()
}

func (r *HTMLReporter) renderMissingIndexesTab(plan *models.PlanAnalysis) string {
	var sb strings.Builder
	sb.WriteString(`<div class="tab-panel">`)
	if len(plan.MissingIndexes) == 0 {
		sb.WriteString(`<p class="none">No missing indexes detected.</p></div>`)
		return sb.String()
	}
	sb.WriteString(`<div class="pv-scroll"><table class="dt"><tr><th>Database</th><th>Table</th><th>Score</th><th>Key Columns</th><th>Include Columns</th><th>CREATE INDEX Statement</th></tr>`)
	for _, mi := range plan.MissingIndexes {
		keyCols := make([]string, 0)
		eqCols := make([]string, 0)
		ineqCols := make([]string, 0)
		for _, c := range mi.Columns {
			if c.Inequality {
				ineqCols = append(ineqCols, c.Column)
			} else if c.Equality {
				eqCols = append(eqCols, c.Column)
			} else {
				keyCols = append(keyCols, c.Column)
			}
		}
		allKey := append(ineqCols, eqCols...)
		allKey = append(allKey, keyCols...)

		keyStr := strings.Join(allKey, ", ")
		incStr := strings.Join(mi.IncludedColumns, ", ")

		createStmt := ""
		tableName := strings.Trim(mi.Table, "[]")
		schemaName := strings.Trim(mi.Schema, "[]")
		if tableName != "" {
			parts := make([]string, 0)
			for _, c := range allKey {
				name := strings.Trim(c, "[]")
				parts = append(parts, name)
			}
			idxName := "IX_" + tableName
			if len(parts) > 0 {
				idxName += "_" + strings.Join(parts, "_")
			}
			fullTable := tableName
			if schemaName != "" {
				fullTable = "[" + schemaName + "].[" + tableName + "]"
			} else {
				fullTable = "[" + tableName + "]"
			}
			createStmt = "CREATE NONCLUSTERED INDEX [" + idxName + "] ON " + fullTable
			if keyStr != "" {
				createStmt += " (" + keyStr + ")"
			}
			if incStr != "" {
				createStmt += " INCLUDE (" + incStr + ")"
			}
		}

		sb.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td class="m">%d</td><td>%s</td><td>%s</td><td><code>%s</code></td></tr>`,
			html.EscapeString(mi.Database), html.EscapeString(mi.Table), mi.Score,
			html.EscapeString(keyStr), html.EscapeString(incStr),
			html.EscapeString(createStmt)))
	}
	sb.WriteString(`</table></div></div>`)
	return sb.String()
}

func (r *HTMLReporter) renderWarningsTab(plan *models.PlanAnalysis) string {
	var sb strings.Builder
	sb.WriteString(`<div class="tab-panel">`)
	if len(plan.Warnings) == 0 {
		sb.WriteString(`<p class="none">No warnings.</p></div>`)
		return sb.String()
	}

	type warningInfo struct {
		icon   string
		detail string
		action string
	}
	infoMap := map[models.WarningType]warningInfo{
		models.WarningTypeSpillToTempDB: {
			icon: "\U0001F4A5",
			detail: "Query operation exceeded its memory grant and was forced to write intermediate results to tempdb on disk. This is 100x slower than in-memory operations.",
			action: "Increase memory grant via hints, optimize query to reduce memory footprint, add indexes to pre-sort data, or simplify the query plan.",
		},
		models.WarningTypeCardinalityEst: {
			icon: "\u26A0",
			detail: "The optimizer detected a potential cardinality estimation problem. The estimated number of rows may not match the actual data distribution.",
			action: "Update statistics, review data skew, consider using OPTION (RECOMPILE) or trace flags for legacy CE.",
		},
		models.WarningTypeNoJoinPredicate: {
			icon: "\u274C",
			detail: "Query joins tables without an ON clause, producing a Cartesian product. Every row from one table matches every row from the other.",
			action: "Add a proper JOIN condition with ON clause or a WHERE filter to eliminate the Cartesian product.",
		},
		models.WarningTypeTypeConversion: {
			icon: "\U0001F504",
			detail: "An implicit type conversion prevents the optimizer from using an index seek. The column data type differs from the parameter data type.",
			action: "Cast the parameter explicitly to match the column data type. E.g., WHERE col = CAST(@param AS col_type).",
		},
		models.WarningTypeNoStatistics: {
			icon: "\u2753",
			detail: "Missing statistics on one or more tables/columns. The optimizer used default estimates (row count guesses) instead of actual data distribution.",
			action: "Create missing statistics manually or run UPDATE STATISTICS on the affected tables.",
		},
	}

	for _, w := range plan.Warnings {
		info := infoMap[w.Type]
		if info.icon == "" {
			info = warningInfo{icon: "\u26A0", detail: "", action: ""}
		}
		sc := r.severityColor(w.Severity)

		sb.WriteString(fmt.Sprintf(`
<div class="fc" style="border-left-color:%s;">
  <div class="fc-t">%s %s</div>
  <span class="fc-tag">%s</span>
  <span class="fc-tag" style="background:%s;color:#fff;">%s</span>`,
			sc, info.icon, html.EscapeString(string(w.Type)), html.EscapeString(string(w.Type)), sc, w.Severity))

		sb.WriteString(fmt.Sprintf(`<div class="fc-l"><strong>Message:</strong> %s</div>`, html.EscapeString(w.Message)))

		if info.detail != "" {
			sb.WriteString(fmt.Sprintf(`<div class="fc-l"><strong>What Happens:</strong> %s</div>`, info.detail))
		}
		if info.action != "" {
			sb.WriteString(fmt.Sprintf(`<div class="fc-l fc-rec"><strong>Action:</strong> %s</div>`, info.action))
		}

		// Extract table/column/expression context from the message when available
		detailParts := make([]string, 0)
		if strings.Contains(w.Message, "CONVERT(") || strings.Contains(w.Message, "convert") {
			detailParts = append(detailParts, "Affects index usage and cardinality estimates for the converted column")
		}
		if strings.Contains(string(w.Type), "Spill") {
			detailParts = append(detailParts, "Affected operators may include Sorts, Hash Joins, and other memory-intensive operations")
		}
		for _, d := range detailParts {
			sb.WriteString(fmt.Sprintf(`<div class="fc-l" style="color:var(--tx4);font-size:0.8rem;">%s</div>`, d))
		}

		sb.WriteString(`</div>`)
	}
	sb.WriteString(`</div>`)
	return sb.String()
}

func (r *HTMLReporter) renderStyles() string {
	return `
<style>
:root {
  --bg: #ffffff; --bg2: #f5f5f5; --bg3: #f9fafb; --bg4: #ffffff;
  --tx: #1f2937; --tx2: #4b5563; --tx3: #6b7280; --tx4: #9ca3af;
  --bd: #e5e7eb; --bd2: #f3f4f6;
  --ac: #2563eb; --ac2: rgba(37,99,235,0.08);
  --hd: #f9fafb; --tb: #ffffff;
  --tb-i: #6b7280; --tb-a: #2563eb; --tb-h: #f3f4f6;
  --cd: #f3f4f6; --cc: #be185d; --mo: #059669;
  --rcbg: #f9fafb; --hv: #f3f4f6;
  --sg: #f9fafb; --sh: #ffffff; --pb: #f9fafb;
  --pv-bg: #ffffff; --pv-bd: #e5e7eb; --pv-hd: #f9fafb;
  --pv-bar: #e5e7eb;
}
#theme-toggle:checked ~ .container {
  --bg: #1e293b; --bg2: #0f172a; --bg3: #1e293b; --bg4: #0f172a;
  --tx: #f1f5f9; --tx2: #e2e8f0; --tx3: #94a3b8; --tx4: #64748b;
  --bd: #334155; --bd2: #1e293b;
  --ac: #38bdf8; --ac2: rgba(56,189,248,0.08);
  --hd: #0f172a; --tb: #1e293b;
  --tb-i: #94a3b8; --tb-a: #38bdf8; --tb-h: #334155;
  --cd: #1e293b; --cc: #f472b6; --mo: #7dd3fc;
  --rcbg: #0f172a; --hv: #334155;
  --sg: #0f172a; --sh: #0f172a; --pb: #0f172a;
  --pv-bg: #0f172a; --pv-bd: #334155; --pv-hd: #1e293b;
  --pv-bar: #334155;
}
*,*::before,*::after{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Oxygen,Ubuntu,sans-serif;background:var(--bg2);color:var(--tx);line-height:1.6;padding:20px}
#theme-toggle{display:none}
.container{max-width:1280px;margin:0 auto;background:var(--bg);border-radius:12px;box-shadow:0 4px 24px rgba(0,0,0,0.08);overflow:hidden}
.header{padding:20px 24px;background:var(--hd);border-bottom:1px solid var(--bd)}
.header-top{display:flex;align-items:center;gap:12px;margin-bottom:4px;flex-wrap:wrap}
.header h1{font-size:1.5rem;color:var(--tx);font-weight:700}
.header-badge{padding:4px 14px;border-radius:20px;font-size:1rem;font-weight:700;color:#fff}
.header-badge.green{background:linear-gradient(135deg,#22c55e,#16a34a)}
.header-badge.yellow{background:linear-gradient(135deg,#eab308,#ca8a04)}
.header-badge.red{background:linear-gradient(135deg,#ef4444,#dc2626)}
.theme-btn{cursor:pointer;font-size:1.3rem;padding:4px 8px;border-radius:6px;background:var(--bg3);border:1px solid var(--bd);margin-left:auto;user-select:none;transition:background .15s}
.theme-btn:hover{background:var(--hv)}
.meta{color:var(--tx3);font-size:0.82rem}
.tabs{position:relative;background:var(--bg)}
.tabs input[type="radio"]{display:none}
.tabs label{display:inline-block;padding:10px 16px;font-size:0.82rem;font-weight:600;color:var(--tb-i);cursor:pointer;border-bottom:2px solid transparent;transition:all .15s;user-select:none;white-space:nowrap}
.tabs label:hover{color:var(--tx);background:var(--tb-h)}
.tabs input:checked+label{color:var(--tb-a);border-bottom-color:var(--tb-a);background:var(--ac2)}
.tab-badge{display:inline-flex;align-items:center;justify-content:center;min-width:18px;height:18px;padding:0 5px;border-radius:9px;background:var(--bd);color:var(--tx3);font-size:0.65rem;font-weight:700;margin-left:4px;vertical-align:middle}
input:checked+label .tab-badge{background:var(--tb-a);color:#fff}
.tab-content{display:none;padding:24px;background:var(--bg);overflow-x:auto}
#tab-summary:checked~#content-summary{display:block}
#tab-findings:checked~#content-findings{display:block}
#tab-planviewer:checked~#content-planviewer{display:block}
#tab-recs:checked~#content-recs{display:block}
#tab-indexes:checked~#content-indexes{display:block}
#tab-warnings:checked~#content-warnings{display:block}
.tab-panel{animation:fade .2s ease}
@keyframes fade{from{opacity:.3}to{opacity:1}}

.summary-hero{display:flex;gap:24px;align-items:center;margin-bottom:20px;padding:24px;background:var(--pb);border-radius:12px;border:1px solid var(--bd)}
.hero-score{text-align:center;flex-shrink:0}
.hero-circle{width:76px;height:76px;border-radius:50%;display:flex;align-items:center;justify-content:center;font-size:1.6rem;font-weight:800;color:#fff;margin:0 auto 6px}
.hero-circle.green{background:linear-gradient(135deg,#22c55e,#16a34a)}
.hero-circle.yellow{background:linear-gradient(135deg,#eab308,#ca8a04)}
.hero-circle.red{background:linear-gradient(135deg,#ef4444,#dc2626)}
.hero-label{font-size:0.8rem;color:var(--tx3);font-weight:600;text-transform:uppercase;letter-spacing:.5px}
.hero-info{flex:1;min-width:200px}
.hero-info h2{font-size:1.15rem;color:var(--tx);margin-bottom:10px}
.score-bar-c{width:100%;height:10px;background:var(--bd);border-radius:5px;overflow:hidden;margin-bottom:14px}
.score-bar{height:100%;border-radius:5px;transition:width .5s ease}
.hero-stats{display:flex;gap:20px;flex-wrap:wrap}
.hero-stat{font-size:0.82rem;color:var(--tx3)}
.stat-v{font-size:1.2rem;font-weight:700;color:var(--tx);display:block}

.s-box{padding:12px 16px;background:var(--pb);border-radius:8px;margin-bottom:10px;font-size:0.92rem;border:1px solid var(--bd)}
.impact-box{border-left:4px solid #f97316}
blockquote{border-left:4px solid var(--ac);padding:12px 16px;margin-bottom:10px;background:var(--pb);border-radius:0 8px 8px 0;color:var(--tx3);font-style:italic;font-size:0.92rem;border:1px solid var(--bd);border-left-width:4px}
.warn-box{background:rgba(239,68,68,0.06);border-color:rgba(239,68,68,0.2)}
.warn-box ul{margin:6px 0 0 20px;color:#dc2626}
.info-box{background:rgba(37,99,235,0.06);border-color:rgba(37,99,235,0.2)}
.info-box ul{margin:6px 0 0 20px;color:var(--ac)}
.sec-title{font-size:0.95rem;color:var(--tx);margin:18px 0 8px;padding-bottom:5px;border-bottom:1px solid var(--bd)}

.dt{width:100%;border-collapse:collapse;margin-bottom:14px;font-size:0.88rem}
.dt th{background:var(--hd);color:var(--tx3);font-size:0.72rem;text-transform:uppercase;padding:7px 10px;text-align:left;border-bottom:2px solid var(--bd);letter-spacing:.4px}
.dt td{padding:7px 10px;border-bottom:1px solid var(--bd);color:var(--tx2)}
.dt tr:hover td{background:var(--hv)}
.tr-total td{background:var(--hd)}
.m{font-family:'SF Mono',Monaco,Consolas,monospace;font-size:0.82rem;color:var(--mo)}
.bmini{width:70px;height:7px;background:var(--bd);border-radius:4px;overflow:hidden;display:inline-block;vertical-align:middle}
.bfill{height:100%;background:var(--ac);border-radius:4px}

.sg{margin-bottom:16px}
.sg-h{display:flex;align-items:center;gap:8px;padding:8px 14px;background:var(--sg);border-radius:4px;margin-bottom:6px;border:1px solid var(--bd)}
.sg-i{font-size:1.1rem}
.sg-t{font-weight:700;font-size:0.9rem;color:var(--tx)}
.fc{padding:12px 14px;margin-bottom:6px;background:var(--pb);border-radius:6px;border:1px solid var(--bd);border-left-width:4px}
.fc-header{display:flex;align-items:center;gap:8px;margin-bottom:4px}
.fc-t{font-weight:700;font-size:0.92rem;color:var(--tx);flex:1}
.fc-tag{display:inline-block;padding:1px 7px;border-radius:3px;background:var(--cd);color:var(--tx3);font-size:0.72rem;font-weight:600;text-transform:uppercase;margin-bottom:4px;margin-right:4px}
.fc-l{font-size:0.85rem;color:var(--tx2);margin:2px 0}
.fc-rec{color:#059669;font-weight:500}
.fc-op{color:var(--tx4);font-size:0.8rem;margin-top:3px}
.conf-badge{display:inline-flex;align-items:center;justify-content:center;min-width:36px;padding:2px 8px;border-radius:10px;font-size:0.72rem;font-weight:700;color:#fff;flex-shrink:0}
.conf-high{background:#22c55e}
.conf-med{background:#eab308}
.conf-low{background:#ef4444}
.conf-tip{font-size:0.7rem;color:var(--tx4);margin-bottom:4px}
.ev-trace{font-size:0.78rem;color:var(--tx3);margin-top:6px;padding-top:4px;border-top:1px dashed var(--bd)}
.ev-item{display:inline-block;padding:0 4px;background:var(--ac2);border-radius:2px;cursor:help;border-bottom:1px dotted var(--tx4)}
.sev-good{color:#22c55e;font-weight:600}
.sev-med{color:#eab308;font-weight:600}
.sev-high{color:#f97316;font-weight:600}
.sev-crit{color:#ef4444;font-weight:600}

/* Plan Viewer — SVG tree renderer (SSMS-style) */
.pv-plan-hdr{font-size:0.78rem;color:var(--tx3);padding:4px 0 8px;min-height:20px}
.pv-toolbar{display:flex;gap:6px;align-items:center;flex-wrap:wrap;margin-bottom:8px}
.pv-btn{padding:4px 10px;border:1px solid var(--bd);border-radius:4px;background:var(--bg2);color:var(--tx);cursor:pointer;font-size:0.78rem;font-weight:600;transition:background .12s}
.pv-btn:hover{background:var(--hv)}
.pv-search{padding:4px 10px;border:1px solid var(--bd);border-radius:4px;background:var(--bg2);color:var(--tx);font-size:0.82rem;flex:1;max-width:220px}
.pv-layout{display:flex;gap:8px;align-items:flex-start}
/* Top-5 sidebar with proper grid */
.pv-top5-panel{width:248px;flex-shrink:0;background:var(--bg2);border:1px solid var(--bd);border-radius:6px;padding:8px 6px;font-size:0.77rem}
.pv-t5-hdr{font-weight:700;color:var(--tx3);font-size:0.72rem;text-transform:uppercase;letter-spacing:.4px;margin-bottom:6px;padding:0 4px}
.pv-t5-head-row{display:grid;grid-template-columns:18px 72px 1fr 44px;gap:2px 4px;padding:2px 4px;color:var(--tx3);font-size:0.68rem;font-weight:700;text-transform:uppercase;letter-spacing:.3px;border-bottom:1px solid var(--bd);margin-bottom:2px}
.pv-t5-row{display:grid;grid-template-columns:18px 72px 1fr 44px;gap:2px 4px;align-items:center;padding:4px 4px;border-radius:4px;cursor:pointer;margin-bottom:1px}
.pv-t5-row:hover{background:var(--hv)}
.pv-t5-col-rank{font-weight:700;font-size:0.78rem;text-align:center;flex-shrink:0}
.pv-t5-col-pct{display:flex;flex-direction:column;gap:2px;min-width:0}
.pv-t5-bar{height:5px;background:var(--bd2);border-radius:3px;overflow:hidden}
.pv-t5-bar-fill{height:100%;border-radius:3px;transition:width .2s}
.pv-t5-pct-txt{font-size:0.74rem;font-weight:700;line-height:1}
.pv-t5-col-name{min-width:0;overflow:hidden}
.pv-t5-op{font-weight:600;color:var(--tx);font-size:0.77rem;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.pv-t5-obj{color:var(--tx3);font-size:0.70rem;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.pv-t5-col-rows{font-size:0.73rem;color:var(--tx2);text-align:right;font-family:monospace;white-space:nowrap}
/* Canvas */
.pv-canvas-wrap{flex:1;position:relative;min-width:0}
.pv-canvas{width:100%;height:580px;border:1px solid var(--bd);border-radius:8px;overflow:auto;background:var(--pv-bg);position:relative;cursor:grab}
.pv-canvas::-webkit-scrollbar{height:8px;width:8px}
.pv-canvas::-webkit-scrollbar-track{background:var(--bg2)}
.pv-canvas::-webkit-scrollbar-thumb{background:var(--bd);border-radius:4px}
.pv-canvas::-webkit-scrollbar-thumb:hover{background:var(--tx3)}
.pv-node{transition:opacity .15s}
.pv-node:hover rect:first-of-type{filter:brightness(0.94)}
/* Hover tooltip */
.pv-tt{position:fixed;z-index:9999;display:none;background:var(--bg);border:1px solid var(--bd);border-radius:7px;padding:9px 12px;font-size:0.78rem;max-width:300px;box-shadow:0 4px 16px rgba(0,0,0,0.22);pointer-events:none}
.pv-tt-op{font-weight:700;font-size:0.86rem;color:var(--tx);margin-bottom:5px}
.pv-tt-row{display:flex;gap:8px;margin-bottom:3px;line-height:1.3}
.pv-tt-lbl{color:var(--tx3);flex:0 0 62px;font-weight:600;font-size:0.74rem}
.pv-tt-val{color:var(--tx2);font-size:0.76rem;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.pv-tt-pred{margin-top:5px;padding:4px 6px;background:var(--cd);border-radius:4px;font-size:0.73rem;color:var(--cc);font-family:monospace;word-break:break-all;line-height:1.4}
.pv-tt-out{margin-top:4px;font-size:0.72rem;color:var(--tx3);font-family:monospace;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.pv-tt-hint{margin-top:6px;font-size:0.70rem;color:var(--tx4);text-align:right;font-style:italic}
/* Properties panel */
.pv-props-panel{position:absolute;right:0;top:0;bottom:0;width:300px;background:var(--bg);border-left:1px solid var(--bd);overflow-y:auto;display:none;z-index:20;border-radius:0 8px 8px 0;font-size:0.78rem}
.pv-props-panel.open{display:block}
.pv-props-hdr{display:flex;align-items:center;justify-content:space-between;padding:8px 12px;background:var(--hd);border-bottom:1px solid var(--bd);position:sticky;top:0;z-index:2}
.pv-props-title{font-weight:700;color:var(--tx);font-size:0.85rem}
.pv-close-btn{background:none;border:none;cursor:pointer;font-size:1rem;color:var(--tx3);padding:2px 4px;border-radius:3px}
.pv-close-btn:hover{background:var(--hv)}
.pv-sec{border-bottom:1px solid var(--bd2)}
.pv-sec-hdr{padding:5px 12px;background:var(--hd);font-weight:700;color:var(--tx3);font-size:0.72rem;text-transform:uppercase;letter-spacing:.4px;cursor:pointer;list-style:none}
.pv-sec-hdr::-webkit-details-marker{display:none}
.pv-sec-body{padding:2px 0}
.pv-pr{display:flex;padding:3px 12px;border-bottom:1px solid var(--bd2)}
.pv-pl{color:var(--tx3);flex:0 0 44%;font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:0.76rem}
.pv-pv{color:var(--tx2);flex:1;word-break:break-all;overflow:hidden;font-size:0.76rem}
.pv-pv code{font-size:0.76rem;background:var(--cd);padding:1px 4px;border-radius:2px;color:var(--cc);word-break:break-all}
.pv-thr-tbl{width:100%;border-collapse:collapse;font-size:0.75rem}
.pv-thr-tbl th{background:var(--hd);padding:2px 4px;color:var(--tx3)}
.pv-thr-tbl td{padding:2px 4px;border-bottom:1px solid var(--bd2)}

.rc{border:1px solid var(--bd);border-radius:8px;margin-bottom:10px;overflow:hidden}
.rc-h{display:flex;align-items:center;gap:10px;padding:9px 14px;background:var(--hd);border-left:4px solid}
.rc-n{display:inline-flex;align-items:center;justify-content:center;width:24px;height:24px;border-radius:50%;background:var(--ac);color:#fff;font-weight:700;font-size:0.75rem;flex-shrink:0}
.rc-t{font-weight:600;flex:1;color:var(--tx);font-size:0.88rem}
.rc-b{padding:2px 9px;border-radius:10px;color:#fff;font-size:0.65rem;font-weight:700;text-transform:uppercase}
.rc-body{padding:10px 14px;font-size:0.85rem}
.rc-meta{color:var(--tx3);margin-bottom:5px}
.rc-d{color:var(--tx2);margin-top:5px;padding-top:5px;border-top:1px solid var(--bd)}
.rc-sql{background:#1f2937;color:#e5e7eb;padding:10px;border-radius:5px;overflow-x:auto;font-size:0.82rem;margin:6px 0}
.none{color:var(--tx4);font-style:italic;padding:20px;text-align:center}
code{background:var(--cd);padding:1px 5px;border-radius:3px;font-size:0.82rem;color:var(--cc)}
</style>`
}

func (r *HTMLReporter) operatorCategory(physOp string) string {
	lo := strings.ToLower(physOp)
	switch {
	case strings.Contains(lo, "scan") || strings.Contains(lo, "seek"):
		return "Scans & Seeks"
	case strings.Contains(lo, "join") || strings.Contains(lo, "nested loops") || strings.Contains(lo, "hash match") || strings.Contains(lo, "merge"):
		return "Joins"
	case strings.Contains(lo, "sort"):
		return "Sorts"
	case strings.Contains(lo, "aggregate") || strings.Contains(lo, "stream"):
		return "Aggregates"
	case strings.Contains(lo, "parallelism") || strings.Contains(lo, "exchange"):
		return "Parallelism"
	case strings.Contains(lo, "spool"):
		return "Spools"
	case strings.Contains(lo, "compute scalar"):
		return "Compute"
	case strings.Contains(lo, "lookup"):
		return "Lookups"
	case strings.Contains(lo, "filter"):
		return "Filters"
	case strings.Contains(lo, "udx") || strings.Contains(lo, "table-valued"):
		return "UDF/UDX"
	default:
		return "Other"
	}
}

func (r *HTMLReporter) opIcon(physOp string) string {
	lo := strings.ToLower(physOp)
	switch {
	case strings.Contains(lo, "clustered") && strings.Contains(lo, "seek"):
		return "\U0001F3AF"
	case strings.Contains(lo, "clustered") && strings.Contains(lo, "scan"):
		return "\U0001F4C2"
	case strings.Contains(lo, "index seek") || strings.Contains(lo, "index scan"):
		return "\U0001F50D"
	case strings.Contains(lo, "table scan"):
		return "\U0001F4CB"
	case strings.Contains(lo, "nested loops"):
		return "\U0001F504"
	case strings.Contains(lo, "hash match"):
		return "\U0001F517"
	case strings.Contains(lo, "merge join"):
		return "\U0001F500"
	case strings.Contains(lo, "sort"):
		return "\U0001F4C8"
	case strings.Contains(lo, "parallelism"):
		return "\u26A1"
	case strings.Contains(lo, "compute scalar"):
		return "\u270F"
	case strings.Contains(lo, "stream aggregate") || strings.Contains(lo, "hash aggregate") || strings.Contains(lo, "aggregate"):
		return "\U0001F4CA"
	case strings.Contains(lo, "spool"):
		return "\U0001F4BE"
	case strings.Contains(lo, "sequence"):
		return "\U0001F4C5"
	case strings.Contains(lo, "segment"):
		return "\U0001F4C6"
	case strings.Contains(lo, "top"):
		return "\U0001F51D"
	case strings.Contains(lo, "key lookup") || strings.Contains(lo, "rid lookup"):
		return "\U0001F50E"
	case strings.Contains(lo, "udx"):
		return "\u2699"
	case strings.Contains(lo, "concatenation"):
		return "\U0001F500"
	case strings.Contains(lo, "bitmap"):
		return "\U0001F5BC"
	case strings.Contains(lo, "constant"):
		return "\U0001F4B0"
	case strings.Contains(lo, "filter"):
		return "\U0001F6D1"
	case strings.Contains(lo, "window"):
		return "\U0001F4C4"
	case strings.Contains(lo, "assert"):
		return "\u26A0"
	default:
		return "\u25B6"
	}
}

func (r *HTMLReporter) opTableShort(op *models.Operator) string {
	if op.IndexScan != nil {
		obj := op.IndexScan.Object
		if obj.Table != "" {
			s := obj.Table
			if obj.Index != "" {
				s = obj.Index + " \u2192 " + s
			}
			return s
		}
	}
	if op.TableScan != nil && op.TableScan.Object.Table != "" {
		return op.TableScan.Object.Table
	}
	return ""
}

func (r *HTMLReporter) costBarColor(pct float64) string {
	if pct > 50 {
		return "#ef4444"
	} else if pct > 20 {
		return "#f97316"
	} else if pct > 5 {
		return "#eab308"
	}
	return "#22c55e"
}

func (r *HTMLReporter) healthColor(score int) string {
	if score >= 70 {
		return "#22c55e"
	} else if score >= 40 {
		return "#eab308"
	}
	return "#ef4444"
}

func (r *HTMLReporter) healthLabel(score int) string {
	if score >= 70 {
		return "Good"
	} else if score >= 40 {
		return "Warning"
	}
	return "Critical"
}

func (r *HTMLReporter) healthClass(score int) string {
	if score >= 70 {
		return "green"
	} else if score >= 40 {
		return "yellow"
	}
	return "red"
}

func (r *HTMLReporter) severityColor(sev models.Severity) string {
	switch sev {
	case models.SeverityCritical:
		return "#ef4444"
	case models.SeverityHigh:
		return "#f97316"
	case models.SeverityMedium:
		return "#eab308"
	case models.SeverityLow:
		return "#22c55e"
	default:
		return "#6b7280"
	}
}

func (r *HTMLReporter) sevColors() map[models.Severity]string {
	return map[models.Severity]string{
		models.SeverityCritical: "#ef4444",
		models.SeverityHigh:     "#f97316",
		models.SeverityMedium:   "#eab308",
		models.SeverityLow:      "#22c55e",
	}
}

func (r *HTMLReporter) sevIcons() map[models.Severity]string {
	return map[models.Severity]string{
		models.SeverityCritical: "\U0001F534",
		models.SeverityHigh:     "\U0001F7E0",
		models.SeverityMedium:   "\U0001F7E1",
		models.SeverityLow:      "\U0001F7E2",
	}
}

func (r *HTMLReporter) fmtInt(n int64) string {
	if n <= 0 {
		return "-"
	}
	return strconv.FormatInt(n, 10)
}

func (r *HTMLReporter) fmtRows(n float64) string {
	if n <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f", n)
}
