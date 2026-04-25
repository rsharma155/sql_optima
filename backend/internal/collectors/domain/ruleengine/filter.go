package ruleengine

import (
	"regexp"
	"strings"
)

type IgnoreRule struct {
	Type  string
	Value string
}

type FilterService struct {
	mssqlRules []IgnoreRule
	pgRules    []IgnoreRule
}

func NewFilterService(mssqlRules []IgnoreRule, pgRules []IgnoreRule) *FilterService {
	return &FilterService{
		mssqlRules: mssqlRules,
		pgRules:    pgRules,
	}
}

func (s *FilterService) ShouldIgnoreMSSQL(database, login, program string) bool {
	for _, rule := range s.mssqlRules {
		switch rule.Type {
		case "database":
			if match(rule.Value, database) {
				return true
			}
		case "login":
			if match(rule.Value, login) {
				return true
			}
		case "program":
			if match(rule.Value, program) {
				return true
			}
		}
	}
	return false
}

func (s *FilterService) ShouldIgnorePG(database, application string) bool {
	for _, rule := range s.pgRules {
		switch rule.Type {
		case "database":
			if match(rule.Value, database) {
				return true
			}
		case "application":
			if match(rule.Value, application) {
				return true
			}
		}
	}
	return false
}

func match(pattern, value string) bool {
	if strings.Contains(pattern, "%") {
		regex := strings.ReplaceAll(pattern, "%", ".*")
		matched, _ := regexp.MatchString("(?i)^"+regex+"$", value)
		return matched
	}
	return strings.EqualFold(pattern, value)
}
