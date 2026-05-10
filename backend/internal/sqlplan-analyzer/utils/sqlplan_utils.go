// File: internal/utils/utils.go
// Purpose: Utility functions for the project
// Package: github.com/rsharma155/sqlplan-analyzer/internal/utils
package utils

func MinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func MaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
