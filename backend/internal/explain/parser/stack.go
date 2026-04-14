// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Stack-based parser for nested plan nodes.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package parser

import "github.com/rsharma155/sql_optima/internal/explain/types"

type stackEntry struct {
	node    *types.PlanNode
	indent  int
	isArrow bool
}

type stack struct {
	entries []*stackEntry
}

func newStack() *stack {
	return &stack{entries: make([]*stackEntry, 0)}
}

func (s *stack) push(node *types.PlanNode, indent int, isArrow bool) {
	s.entries = append(s.entries, &stackEntry{node: node, indent: indent, isArrow: isArrow})
}

func (s *stack) pop() *stackEntry {
	if len(s.entries) == 0 {
		return nil
	}
	last := s.entries[len(s.entries)-1]
	s.entries = s.entries[:len(s.entries)-1]
	return last
}

func (s *stack) top() *stackEntry {
	if len(s.entries) == 0 {
		return nil
	}
	return s.entries[len(s.entries)-1]
}

func (s *stack) isEmpty() bool {
	return len(s.entries) == 0
}

func (s *stack) popUntil(targetIndent int) {
	for len(s.entries) > 0 && s.entries[len(s.entries)-1].indent >= targetIndent {
		s.entries = s.entries[:len(s.entries)-1]
	}
}

func (s *stack) len() int {
	return len(s.entries)
}
