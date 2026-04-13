package api

import (
	"errors"
	"regexp"
	"strings"
)

var (
	ErrEmptyDSN            = errors.New("database_dsn is required")
	ErrEmptyQuery          = errors.New("query_text is required")
	ErrEmptyPlan           = errors.New("execution_plan_json is required")
	ErrInvalidPlan         = errors.New("execution_plan_json is not valid JSON")
	ErrMultipleStatements  = errors.New("only single-statement SQL is supported")
	ErrPlaceholderNoParams = errors.New("query has placeholders but query_params not provided")
)

// Validate validates the incoming request
func (r *IndexAdvisorRequest) Validate() error {
	if strings.TrimSpace(r.DatabaseDSN) == "" {
		return ErrEmptyDSN
	}

	if strings.TrimSpace(r.QueryText) == "" {
		return ErrEmptyQuery
	}

	if r.ExecutionPlanJSON == nil {
		return ErrEmptyPlan
	}

	stmtCount := countStatements(r.QueryText)
	if stmtCount != 1 {
		return ErrMultipleStatements
	}

	if hasPlaceholders(r.QueryText) && len(r.QueryParams) == 0 {
		return ErrPlaceholderNoParams
	}

	return nil
}

func countStatements(query string) int {
	query = strings.ToUpper(strings.TrimSpace(query))
	count := 0

	keywords := []string{"SELECT", "INSERT", "UPDATE", "DELETE"}
	for _, kw := range keywords {
		if strings.Contains(query, kw) {
			count++
		}
	}

	withPattern := regexp.MustCompile(`\bWITH\b`)
	if withPattern.MatchString(query) {
		count++
	}

	return count
}

func hasPlaceholders(query string) bool {
	return strings.Contains(query, "$1") ||
		strings.Contains(query, "$2") ||
		strings.Contains(query, "$3") ||
		strings.Contains(query, "?")
}
