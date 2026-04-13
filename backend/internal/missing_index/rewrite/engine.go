package rewrite

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/rsharma155/sql_optima/internal/missing_index/logging"
	"github.com/rsharma155/sql_optima/internal/missing_index/types"
)

const (
	RulePriorityHigh   = 100
	RulePriorityMedium = 50
	RulePriorityLow    = 10
)

type Matcher func(query string) bool
type Rewriter func(query string, projectedCols []string) (string, bool)
type Validator func(query string) bool

type Rule struct {
	Name      string
	Priority  int
	Matcher   Matcher
	Rewriter  Rewriter
	Validator Validator
}

type Engine struct {
	rules []Rule
}

func New() *Engine {
	engine := &Engine{}
	engine.registerRules()
	return engine
}

func (e *Engine) registerRules() {
	e.rules = []Rule{
		{
			Name:      "select_star_to_explicit",
			Priority:  RulePriorityHigh,
			Matcher:   matchSelectStar,
			Rewriter:  rewriteSelectStarToExplicit,
			Validator: validateRewrite,
		},
		{
			Name:      "not_in_to_not_exists",
			Priority:  RulePriorityMedium,
			Matcher:   matchNotIn,
			Rewriter:  rewriteNotInToNotExists,
			Validator: validateRewrite,
		},
		{
			Name:      "offset_keyset_note",
			Priority:  RulePriorityMedium,
			Matcher:   matchOffset,
			Rewriter:  rewriteOffsetWithLeadingComment,
			Validator: validateRewrite,
		},
		{
			Name:      "cte_materialize_note",
			Priority:  RulePriorityLow,
			Matcher:   matchWithCTE,
			Rewriter:  rewriteCTELeadingComment,
			Validator: validateRewrite,
		},
	}
	sortRulesByPriority(e.rules)
}

func (e *Engine) Apply(ctx context.Context, query string, analysis *types.QueryAnalysis) *Result {
	result := &Result{
		OriginalQuery: query,
		QueryVariants: []QueryVariant{
			{Query: query, AppliedRules: []string{}, IsOriginal: true},
		},
	}

	if analysis == nil {
		logging.Debug(ctx, "No analysis available, skipping rewrites", nil)
		return result
	}

	var allVariants []QueryVariant

	allVariants = append(allVariants, QueryVariant{Query: query, AppliedRules: []string{}, IsOriginal: true})

	for _, rule := range e.rules {
		if !rule.Matcher(query) {
			continue
		}

		projectedCols := analysis.ProjectedCols
		if projectedCols == nil {
			projectedCols = []string{}
		}

		rewritten, ok := rule.Rewriter(query, projectedCols)
		if !ok {
			logging.Debug(ctx, "Rewrite returned false", map[string]any{"rule": rule.Name})
			continue
		}

		if !rule.Validator(rewritten) {
			logging.Debug(ctx, "Validation failed", map[string]any{"rule": rule.Name})
			continue
		}

		if rewritten != query {
			allVariants = append(allVariants, QueryVariant{
				Query:        rewritten,
				AppliedRules: []string{rule.Name},
				IsOriginal:   false,
			})
			logging.Info(ctx, "Applied rewrite rule", map[string]any{
				"rule": rule.Name,
				"from": query[:min(len(query), 50)],
				"to":   rewritten[:min(len(rewritten), 50)],
			})
		}
	}

	result.QueryVariants = allVariants

	return result
}

func (e *Engine) GetRules() []Rule {
	rules := make([]Rule, len(e.rules))
	copy(rules, e.rules)
	return rules
}

func matchSelectStar(query string) bool {
	upper := strings.ToUpper(query)
	selectPattern := regexp.MustCompile(`(?i)SELECT\s+\*\s+FROM`)
	return selectPattern.MatchString(query) && !strings.Contains(upper, "SELECT COUNT")
}

func rewriteSelectStarToExplicit(query string, projectedCols []string) (string, bool) {
	if len(projectedCols) == 0 {
		return query, false
	}

	selectStarPattern := regexp.MustCompile(`(?i)SELECT\s+\*\s+FROM`)
	if !selectStarPattern.MatchString(query) {
		return query, false
	}

	if projectedCols[0] == "*" {
		return query, false
	}

	cols := strings.Join(projectedCols, ", ")
	replaced := selectStarPattern.ReplaceAllString(query, "SELECT "+cols+" FROM")

	return replaced, true
}

func matchNotIn(query string) bool {
	return regexp.MustCompile(`(?i)\s+NOT\s+IN\s*\(`).MatchString(query)
}

func rewriteNotInToNotExists(query string, projectedCols []string) (string, bool) {
	notInPattern := regexp.MustCompile(`(?i)(\w+)\s+NOT\s+IN\s*\(([^)]+)\)`)
	matches := notInPattern.FindStringSubmatch(query)
	if len(matches) < 3 {
		return query, false
	}

	column := matches[1]
	values := matches[2]

	subqueryPattern := regexp.MustCompile(`(?i)(FROM\s+\w+(?:\.\w+)?(?:\s+\w+)?)`)
	fromMatch := subqueryPattern.FindString(query)
	if fromMatch == "" {
		return query, false
	}

	newSubquery := fmt.Sprintf("SELECT 1 %s WHERE %s = ANY(ARRAY[%s])", fromMatch, column, values)

	wherePattern := regexp.MustCompile(`(?i)WHERE\s+`)
	rewritten := wherePattern.ReplaceAllString(query, "WHERE NOT EXISTS ("+newSubquery+") AND ")

	existsPattern := regexp.MustCompile(`(?i)AND\s+NOT\s+EXISTS\s*\(`)
	existsCheck := existsPattern.MatchString(rewritten)
	if existsCheck {
		return query, false
	}

	return rewritten, true
}

func matchOffset(query string) bool {
	return regexp.MustCompile(`(?i)OFFSET\s+\d+`).MatchString(query)
}

func rewriteOffsetWithLeadingComment(query string, projectedCols []string) (string, bool) {
	if !matchOffset(query) {
		return query, false
	}
	note := "-- Query rewrite hint: OFFSET skips rows on each page; prefer keyset pagination (WHERE (sort_col, id) > ($1,$2) ORDER BY ... LIMIT).\n"
	return note + query, true
}

func matchWithCTE(query string) bool {
	return regexp.MustCompile(`(?is)\bWITH\s+\w+\s+AS\s*\(`).MatchString(query)
}

func rewriteCTELeadingComment(query string, projectedCols []string) (string, bool) {
	if !matchWithCTE(query) {
		return query, false
	}
	note := "-- Query rewrite hint: review CTE materialization (PostgreSQL 12+ may inline); test WITH vs subquery for your workload.\n"
	return note + query, true
}

func stripLeadingSQLComments(q string) string {
	q = strings.TrimSpace(q)
	for {
		if strings.HasPrefix(q, "--") {
			i := strings.Index(q, "\n")
			if i < 0 {
				return ""
			}
			q = strings.TrimSpace(q[i+1:])
			continue
		}
		if strings.HasPrefix(q, "/*") {
			end := strings.Index(q, "*/")
			if end < 0 {
				return q
			}
			q = strings.TrimSpace(q[end+2:])
			continue
		}
		break
	}
	return q
}

func validateRewrite(query string) bool {
	q := stripLeadingSQLComments(query)
	if q == "" {
		return false
	}
	upper := strings.ToUpper(q)
	return strings.HasPrefix(upper, "SELECT") || strings.HasPrefix(upper, "WITH")
}

func sortRulesByPriority(rules []Rule) {
	for i := 0; i < len(rules)-1; i++ {
		for j := i + 1; j < len(rules); j++ {
			if rules[j].Priority > rules[i].Priority {
				rules[i], rules[j] = rules[j], rules[i]
			}
		}
	}
}

