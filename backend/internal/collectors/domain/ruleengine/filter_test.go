package ruleengine

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestFilter_ShouldIgnoreMSSQL(t *testing.T) {
	rules := []IgnoreRule{
		{Type: "database", Value: "master"},
		{Type: "program", Value: "SQLAgent%"},
	}
	filter := NewFilterService(rules, nil)

	assert.True(t, filter.ShouldIgnoreMSSQL("master", "sa", "other"))
	assert.True(t, filter.ShouldIgnoreMSSQL("userdb", "sa", "SQLAgent - Job x"))
	assert.False(t, filter.ShouldIgnoreMSSQL("userdb", "user", "App"))
}
