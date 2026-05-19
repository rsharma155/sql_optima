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

	"github.com/google/uuid"
)

// Batch-kind keys for in-memory snapshot deduplication (per serverID).
const (
	enterpriseKindLatchWaits         = "latch_waits"
	enterpriseKindProcedure          = "procedure_stats"
	enterpriseKindSpinlock           = "spinlock_stats"
	enterpriseKindMemoryClerks       = "memory_clerks"
	enterpriseKindWaitsDelta         = "waits_delta"
	enterpriseKindPgIndexDefinitions = "pg_index_definitions"
	enterpriseKindTableSize          = "table_size"
	enterpriseKindPgSessionActivity  = "pg_session_activity"
	enterpriseKindPgQueryWaitProfile = "pg_query_wait_profile"
)

func enterpriseHashKey(serverID uuid.UUID, kind string, dbName ...string) string {
	instanceStr := serverID.String()
	if len(dbName) > 0 && dbName[0] != "" {
		return instanceStr + "\x00" + dbName[0] + "\x00" + kind
	}
	return instanceStr + "\x00" + kind
}

// EnterpriseSnapshotUnchanged reports whether this scrape matches the last stored snapshot for (serverID, kind).
func (tl *TimescaleLogger) EnterpriseSnapshotUnchanged(serverID uuid.UUID, kind string, sig uint64, dbName ...string) bool {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	key := enterpriseHashKey(serverID, kind, dbName...)
	prev, ok := tl.prevEnterpriseBatchHash[key]
	return ok && prev == sig
}

func (tl *TimescaleLogger) RememberEnterpriseSnapshot(serverID uuid.UUID, kind string, sig uint64, dbName ...string) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	key := enterpriseHashKey(serverID, kind, dbName...)
	tl.prevEnterpriseBatchHash[key] = sig
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
func fingerprintMapRows(serverID uuid.UUID, kind string, rows []map[string]interface{}, keyNames, valueNames []string) uint64 {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%s|%s|%d|", serverID.String(), kind, len(rows))
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
func waitDeltaSnapshotFingerprint(serverID uuid.UUID, rows []WaitDeltaRow) uint64 {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%s|%s|%d|", serverID.String(), enterpriseKindWaitsDelta, len(rows))
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


// IndexDefinitionCatalogRow is a minimal copy used to break an import cycle.
type IndexDefinitionCatalogRow struct {
	DBName           string
	SchemaName       string
	TableName        string
	IndexName        string
	KeyColumns       string
	IncludeColumns   string
	FilterDefinition struct {
		String string
	}
	IsUnique  bool
	IsPK      bool
	IndexType string
}

// FingerprintIndexDefinitionRows hashes index definitions for change detection.
func (tl *TimescaleLogger) FingerprintIndexDefinitionRows(serverID uuid.UUID, rows []IndexDefinitionCatalogRow) uint64 {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%s|%s|%d|", serverID.String(), enterpriseKindPgIndexDefinitions, len(rows))
	if len(rows) == 0 {
		return h.Sum64()
	}
	cp := append([]IndexDefinitionCatalogRow(nil), rows...)
	sort.Slice(cp, func(i, j int) bool {
		if cp[i].DBName != cp[j].DBName {
			return cp[i].DBName < cp[j].DBName
		}
		if cp[i].SchemaName != cp[j].SchemaName {
			return cp[i].SchemaName < cp[j].SchemaName
		}
		if cp[i].TableName != cp[j].TableName {
			return cp[i].TableName < cp[j].TableName
		}
		return cp[i].IndexName < cp[j].IndexName
	})
	for _, r := range cp {
		_, _ = fmt.Fprintf(h, "%s.%s.%s.%s:%s:%s:%s:%v:%v:%s|", r.DBName, r.SchemaName, r.TableName, r.IndexName, r.KeyColumns, r.IncludeColumns, r.FilterDefinition.String, r.IsUnique, r.IsPK, r.IndexType)
	}
	return h.Sum64()
}

func (tl *TimescaleLogger) FingerprintTableSizeRows(serverID uuid.UUID, rows []TableSizeHistoryRow) uint64 {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%s|%s|%d|", serverID.String(), enterpriseKindTableSize, len(rows))
	if len(rows) == 0 {
		return h.Sum64()
	}
	cp := append([]TableSizeHistoryRow(nil), rows...)
	sort.Slice(cp, func(i, j int) bool {
		if cp[i].DatabaseName != cp[j].DatabaseName {
			return cp[i].DatabaseName < cp[j].DatabaseName
		}
		if cp[i].SchemaName != cp[j].SchemaName {
			return cp[i].SchemaName < cp[j].SchemaName
		}
		return cp[i].TableName < cp[j].TableName
	})
	for _, r := range cp {
		_, _ = fmt.Fprintf(h, "%s:%s:%s:%d:%g:%g:%g|", r.DatabaseName, r.SchemaName, r.TableName, r.RowCount, r.TotalMB, r.DataMB, r.IndexMB)
	}
	return h.Sum64()
}
