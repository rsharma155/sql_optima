package rewrite

type QueryVariant struct {
	Query        string
	AppliedRules []string
	IsOriginal   bool
}

type Result struct {
	OriginalQuery string
	QueryVariants []QueryVariant
}

func NewResult(query string) *Result {
	return &Result{
		OriginalQuery: query,
		QueryVariants: []QueryVariant{
			{Query: query, AppliedRules: []string{}, IsOriginal: true},
		},
	}
}
