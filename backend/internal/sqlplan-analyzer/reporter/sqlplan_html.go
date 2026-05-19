package reporter

import (
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
			ratio := float64(op.ActualRows) / float64(op.EstimateRows)
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
			r.fmtInt(op.EstimateRows),
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
		ratio := float64(op.ActualRows) / float64(op.EstimateRows)
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

func (r *HTMLReporter) renderPlanViewerTab(plan *models.PlanAnalysis) string {
	var sb strings.Builder
	sb.WriteString(`<div class="tab-panel">`)
	if len(plan.Operators) == 0 {
		sb.WriteString(`<p class="none">No operators found.</p></div>`)
		return sb.String()
	}

	totalCost := plan.CostSummary.TotalEstimatedCost
	if totalCost <= 0 {
		for _, op := range plan.Operators {
			if op.EstimatedTotalSubtreeCost > totalCost {
				totalCost = op.EstimatedTotalSubtreeCost
			}
		}
	}

	root := plan.QueryPlan.RelOp
	if root != nil {
		// Collect operators by depth level for horizontal layout
		levels := make(map[int][]*models.Operator)
		r.collectLevels(root, 0, levels)

		// Get sorted depth keys in descending order (SSMS style: leaves on left, root on right)
		var depths []int
		for d := range levels {
			depths = append(depths, d)
		}
		sort.Slice(depths, func(i, j int) bool {
			return depths[i] > depths[j]
		})

		sb.WriteString(`<div class="pv-scroll"><div class="pv-hflow">`)

		for idx, depth := range depths {
			ops := levels[depth]
			if idx > 0 {
				// Arrow pointing from child (left) to parent (right)
				sb.WriteString(`<div class="pv-arrow-col"><div class="pv-arr">&#x25B6;</div></div>`)
			}
			sb.WriteString(`<div class="pv-col">`)
			for _, op := range ops {
				r.renderPlanCard(&sb, op, totalCost)
			}
			sb.WriteString(`</div>`)
		}

		sb.WriteString(`</div></div>`)
	} else {
		// Fallback to table
		sb.WriteString(`<table class="dt"><tr><th>ID</th><th>Physical Op</th><th>Logical Op</th><th>Est. Cost</th><th>Est. Rows</th><th>Actual Rows</th></tr>`)
		for _, op := range plan.Operators {
			act := "-"
			if op.ActualRows > 0 {
				act = strconv.FormatInt(op.ActualRows, 10)
			}
			sb.WriteString(fmt.Sprintf(`<tr><td>%d</td><td>%s</td><td>%s</td><td class="m">%.4f</td><td>%s</td><td>%s</td></tr>`,
				op.ID, html.EscapeString(op.PhysicalOp), html.EscapeString(op.LogicalOp), op.EstimatedTotalSubtreeCost, strconv.FormatInt(op.EstimateRows, 10), act))
		}
		sb.WriteString(`</table>`)
	}

	sb.WriteString(`</div>`)
	return sb.String()
}

func (r *HTMLReporter) collectLevels(op *models.Operator, depth int, levels map[int][]*models.Operator) {
	if op == nil {
		return
	}
	levels[depth] = append(levels[depth], op)
	for _, child := range op.Children {
		r.collectLevels(child, depth+1, levels)
	}
}

func (r *HTMLReporter) renderPlanCard(sb *strings.Builder, op *models.Operator, totalCost float64) {
	pct := 0.0
	if totalCost > 0 {
		pct = op.EstimatedTotalSubtreeCost / totalCost * 100
	}
	icon := r.opIcon(op.PhysicalOp)
	tbl := r.opTableShort(op)
	cardID := fmt.Sprintf("pv-d-%d", op.ID)

	memGrant := ""
	if op.MemoryFractions != nil {
		memGrant = fmt.Sprintf("Input: %.2f | Output: %.2f", op.MemoryFractions.Input, op.MemoryFractions.Output)
	}

	sb.WriteString(fmt.Sprintf(`
<div class="pv-card" title="PhysicalOp: %s | LogicalOp: %s | Est Rows: %d | Act Rows: %d | Est Cost: %.4f | CPU: %.4f | I/O: %.4f | Parallel: %v | NodeID: %d | Table: %s">
  <input type="checkbox" id="%s" class="pv-expand-toggle">
  <div class="pv-card-header">
    <span class="pv-icon">%s</span>
    <span class="pv-name">%s</span>
  </div>`, html.EscapeString(op.PhysicalOp), html.EscapeString(op.LogicalOp), op.EstimateRows, op.ActualRows, op.EstimatedTotalSubtreeCost, op.EstimateCPUms, op.EstimatedIOs, op.Parallel, op.NodeID, html.EscapeString(tbl), cardID, icon, html.EscapeString(op.PhysicalOp)))

	tableStr := ""
	if tbl != "" {
		tableStr = fmt.Sprintf(` %s %s`, "\u2192", html.EscapeString(tbl))
		sb.WriteString(fmt.Sprintf(`  <div class="pv-table">%s%s</div>`, html.EscapeString(op.PhysicalOp), tableStr))
	}
	if op.EstimatedTotalSubtreeCost > 0 {
		sb.WriteString(fmt.Sprintf(`  <div class="pv-bar-c"><div class="pv-bar" style="width:%.1f%%"></div></div>`, pct))
	}
	sb.WriteString(fmt.Sprintf(`  <div class="pv-cost">Cost: %.4f (%.0f%%)</div>`, op.EstimatedTotalSubtreeCost, pct))
	sb.WriteString(fmt.Sprintf(`  <div class="pv-rows">Est: %s | Act: %s</div>`, r.fmtInt(op.EstimateRows), r.fmtInt(op.ActualRows)))

	if op.EstimateCPUms > 0 || op.EstimatedIOs > 0 {
		sb.WriteString(fmt.Sprintf(`  <div class="pv-cpu">CPU: %.4f | I/O: %.4f</div>`, op.EstimateCPUms, op.EstimatedIOs))
	}

	sb.WriteString(fmt.Sprintf(`  <label for="%s" class="pv-expand-label">&#x25BC; Details</label>`, cardID))

	sb.WriteString(`  <div class="pv-details">`)
	sb.WriteString(`<table>`)
	if op.NodeID > 0 {
		sb.WriteString(fmt.Sprintf(`<tr><td>Node ID</td><td>%d</td></tr>`, op.NodeID))
	}
	sb.WriteString(fmt.Sprintf(`<tr><td>Physical Op</td><td>%s</td></tr>`, html.EscapeString(op.PhysicalOp)))
	sb.WriteString(fmt.Sprintf(`<tr><td>Logical Op</td><td>%s</td></tr>`, html.EscapeString(op.LogicalOp)))
	if op.EstimatedTotalSubtreeCost > 0 {
		sb.WriteString(fmt.Sprintf(`<tr><td>Est Cost</td><td>%.4f</td></tr>`, op.EstimatedTotalSubtreeCost))
	}
	sb.WriteString(fmt.Sprintf(`<tr><td>Est Rows</td><td>%s</td></tr>`, r.fmtInt(op.EstimateRows)))
	sb.WriteString(fmt.Sprintf(`<tr><td>Act Rows</td><td>%s</td></tr>`, r.fmtInt(op.ActualRows)))
	if op.ActualExecutions > 0 {
		sb.WriteString(fmt.Sprintf(`<tr><td>Executions</td><td>%d</td></tr>`, op.ActualExecutions))
	}
	if op.EstimateCPUms > 0 {
		sb.WriteString(fmt.Sprintf(`<tr><td>CPU Cost</td><td>%.4f</td></tr>`, op.EstimateCPUms))
	}
	if op.EstimatedIOs > 0 {
		sb.WriteString(fmt.Sprintf(`<tr><td>I/O Cost</td><td>%.4f</td></tr>`, op.EstimatedIOs))
	}
	if op.ActualSpills > 0 {
		sb.WriteString(fmt.Sprintf(`<tr><td>Spills</td><td>%d</td></tr>`, op.ActualSpills))
	}
	if op.ActualLogicalReads > 0 {
		sb.WriteString(fmt.Sprintf(`<tr><td>Logical Reads</td><td>%d</td></tr>`, op.ActualLogicalReads))
	}
	if op.ActualPhysicalReads > 0 {
		sb.WriteString(fmt.Sprintf(`<tr><td>Physical Reads</td><td>%d</td></tr>`, op.ActualPhysicalReads))
	}
	if op.Parallel {
		sb.WriteString(fmt.Sprintf(`<tr><td>Parallel</td><td>Yes (Threads: %d)</td></tr>`, op.ParallelThreadCount))
	} else if op.ParallelThreadCount > 0 {
		sb.WriteString(fmt.Sprintf(`<tr><td>Threads</td><td>%d</td></tr>`, op.ParallelThreadCount))
	}
	if op.TableCardinality > 0 {
		sb.WriteString(fmt.Sprintf(`<tr><td>Table Cardinality</td><td>%s</td></tr>`, r.fmtInt(op.TableCardinality)))
	}
	if op.EstimateRebinds > 0 {
		sb.WriteString(fmt.Sprintf(`<tr><td>Rebinds</td><td>%d</td></tr>`, op.EstimateRebinds))
	}
	if op.EstimateRewinds > 0 {
		sb.WriteString(fmt.Sprintf(`<tr><td>Rewinds</td><td>%d</td></tr>`, op.EstimateRewinds))
	}
	if memGrant != "" {
		sb.WriteString(fmt.Sprintf(`<tr><td>Memory Fractions</td><td>%s</td></tr>`, html.EscapeString(memGrant)))
	}
	if len(op.OutputList) > 0 {
		cols := make([]string, 0, len(op.OutputList))
		for _, c := range op.OutputList {
			cols = append(cols, c.Column)
		}
		sb.WriteString(fmt.Sprintf(`<tr><td>Output Columns</td><td>%s</td></tr>`, html.EscapeString(strings.Join(cols, ", "))))
	}
	if op.IndexScan != nil {
		obj := op.IndexScan.Object
		if obj.Index != "" {
			sb.WriteString(fmt.Sprintf(`<tr><td>Index</td><td>[%s].[%s].[%s]</td></tr>`, html.EscapeString(obj.Database), html.EscapeString(obj.Schema), html.EscapeString(obj.Index)))
		} else {
			sb.WriteString(fmt.Sprintf(`<tr><td>Table</td><td>[%s].[%s].[%s]</td></tr>`, html.EscapeString(obj.Database), html.EscapeString(obj.Schema), html.EscapeString(obj.Table)))
		}
	}
	sb.WriteString(`</table>`)
	sb.WriteString(`</div>`)
	sb.WriteString(fmt.Sprintf(`  <label for="%s" class="pv-expand-label pv-hide-label">&#x25B2; Hide</label>`, cardID))

	sb.WriteString(`</div>`)
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

.pv-scroll{overflow-x:auto;padding-bottom:8px}
.pv-hflow{display:flex;flex-direction:row;align-items:flex-start;gap:0;min-width:800px}
.pv-arrow-col{display:flex;align-items:center;padding:0 6px}
.pv-arr{font-size:1.2rem;color:var(--ac);opacity:.6}
.pv-col{display:flex;flex-direction:column;gap:8px}
.pv-card{background:var(--pv-bg);border:1px solid var(--pv-bd);border-radius:8px;padding:10px 12px;min-width:180px;max-width:220px;box-shadow:0 1px 3px rgba(0,0,0,0.06);cursor:default;transition:box-shadow .15s}
.pv-card:hover{box-shadow:0 2px 8px rgba(0,0,0,0.12);border-color:var(--ac)}
.pv-card-header{display:flex;align-items:center;gap:6px;margin-bottom:4px}
.pv-icon{font-size:1.1rem}
.pv-name{font-weight:700;font-size:0.82rem;color:var(--tx);word-break:break-word}
.pv-table{font-size:0.75rem;color:var(--tx3);margin-bottom:4px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.pv-bar-c{height:5px;background:var(--pv-bar);border-radius:3px;overflow:hidden;margin:4px 0}
.pv-bar{height:100%;background:linear-gradient(90deg,var(--ac),#818cf8);border-radius:3px}
.pv-cost{font-family:'SF Mono',Monaco,Consolas,monospace;font-size:0.75rem;color:var(--mo)}
.pv-rows{font-size:0.72rem;color:var(--tx4);margin-top:2px}
.pv-cpu{font-size:0.72rem;color:#f97316;margin-top:1px}
.pv-expand-toggle{display:none}
.pv-expand-label{display:inline-block;font-size:0.72rem;color:var(--ac);cursor:pointer;margin-top:3px;padding:1px 6px;border-radius:3px;user-select:none}
.pv-expand-label:hover{background:var(--hv);text-decoration:underline}
.pv-expand-toggle:checked~.pv-details{display:block}
.pv-expand-toggle:checked~.pv-expand-label{display:none}
.pv-expand-toggle:checked~.pv-hide-label{display:inline-block}
.pv-hide-label{display:none}
.pv-details{display:none;margin-top:6px;padding-top:6px;border-top:1px solid var(--bd);font-size:0.75rem}
.pv-details table{width:100%;border-collapse:collapse}
.pv-details td{padding:2px 4px;color:var(--tx2);vertical-align:top}
.pv-details td:first-child{font-weight:600;color:var(--tx3);white-space:nowrap;width:40%}

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
