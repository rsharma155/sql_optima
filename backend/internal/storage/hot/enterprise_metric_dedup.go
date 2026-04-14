// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Enterprise metrics deduplication to prevent duplicate entries.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
)

// Batch-kind keys for in-memory snapshot deduplication (per instance).
const (
	enterpriseKindLatchWaits    = "latch_waits"
	enterpriseKindProcedure     = "procedure_stats"
	enterpriseKindFileIO        = "file_io_latency"
	enterpriseKindSpinlock      = "spinlock_stats"
	enterpriseKindMemoryClerks  = "memory_clerks"
	enterpriseKindWaitsDelta    = "waits_delta"
)

func enterpriseHashKey(instance, kind string) string {
	return instance + "\x00" + kind
}

// enterpriseSnapshotUnchanged reports whether this scrape matches the last stored snapshot for (instance, kind).
func (tl *TimescaleLogger) enterpriseSnapshotUnchanged(instance, kind string, sig uint64) bool {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	prev, ok := tl.prevEnterpriseBatchHash[enterpriseHashKey(instance, kind)]
	return ok && prev == sig
}

func (tl *TimescaleLogger) rememberEnterpriseSnapshot(instance, kind string, sig uint64) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	tl.prevEnterpriseBatchHash[enterpriseHashKey(instance, kind)] = sig
}

func normalizeMapValue(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'g', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(t), 'g', -1, 32)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case int32:
		return strconv.FormatInt(int64(t), 10)
	case uint32:
		return strconv.FormatUint(uint64(t), 10)
	case uint64:
		return strconv.FormatUint(t, 10)
	case bool:
		return strconv.FormatBool(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

func compositeRowKey(r map[string]interface{}, keyNames []string) string {
	var b strings.Builder
	for _, k := range keyNames {
		b.WriteString(normalizeMapValue(r[k]))
		b.WriteByte(0)
	}
	return b.String()
}

// fingerprintMapRows hashes the full batch in stable row order for duplicate scrape detection.
func fingerprintMapRows(instance, kind string, rows []map[string]interface{}, keyNames, valueNames []string) uint64 {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%s|%s|%d|", instance, kind, len(rows))
	if len(rows) == 0 {
		return h.Sum64()
	}
	order := make([]int, len(rows))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool {
		return compositeRowKey(rows[order[i]], keyNames) < compositeRowKey(rows[order[j]], keyNames)
	})
	for _, idx := range order {
		r := rows[idx]
		for _, k := range valueNames {
			_, _ = fmt.Fprintf(h, "%s|", normalizeMapValue(r[k]))
		}
		_, _ = fmt.Fprintf(h, ";")
	}
	return h.Sum64()
}

// waitDeltaSnapshotFingerprint hashes category totals for a single scrape (see ComputeAndLogWaitDeltas).
func waitDeltaSnapshotFingerprint(instance string, rows []WaitDeltaRow) uint64 {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%s|%s|%d|", instance, enterpriseKindWaitsDelta, len(rows))
	if len(rows) == 0 {
		return h.Sum64()
	}
	cp := append([]WaitDeltaRow(nil), rows...)
	sort.Slice(cp, func(i, j int) bool {
		if cp[i].WaitCategory != cp[j].WaitCategory {
			return cp[i].WaitCategory < cp[j].WaitCategory
		}
		return cp[i].WaitTimeMsDelta < cp[j].WaitTimeMsDelta
	})
	for _, r := range cp {
		_, _ = fmt.Fprintf(h, "%s:%g|", r.WaitCategory, r.WaitTimeMsDelta)
	}
	return h.Sum64()
}
