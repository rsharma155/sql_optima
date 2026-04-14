// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Buffer I/O parser for storage analysis.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package parser

import (
	"regexp"

	"github.com/rsharma155/sql_optima/internal/explain/types"
)

func ParseBuffers(line string) *types.Buffers {
	line = RemovePrefix(line, "Buffers:")
	line = TrimSpace(line)

	b := &types.Buffers{}

	re := regexp.MustCompile(`shared hit[=:]?\s*(\d+)`)
	m := re.FindStringSubmatch(line)
	if m != nil {
		b.SharedHit, _ = atoi(m[1])
	}

	re = regexp.MustCompile(`shared read[=:]?\s*(\d+)`)
	m = re.FindStringSubmatch(line)
	if m != nil {
		b.SharedRead, _ = atoi(m[1])
	}

	re = regexp.MustCompile(`shared written[=:]?\s*(\d+)`)
	m = re.FindStringSubmatch(line)
	if m != nil {
		b.SharedWritten, _ = atoi(m[1])
	}

	re = regexp.MustCompile(`temp read[=:]?\s*(\d+)`)
	m = re.FindStringSubmatch(line)
	if m != nil {
		b.TempRead, _ = atoi(m[1])
	}

	re = regexp.MustCompile(`temp written[=:]?\s*(\d+)`)
	m = re.FindStringSubmatch(line)
	if m != nil {
		b.TempWritten, _ = atoi(m[1])
	}

	return b
}
