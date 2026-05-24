package hot

import (
	"hash/fnv"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

func parsePercentToFloat(s string) float32 {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "%")
	val, err := strconv.ParseFloat(s, 32)
	if err != nil {
		return 0
	}
	return float32(val)
}

func getFloat64(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case float64:
			return val
		case int:
			return float64(val)
		case int64:
			return float64(val)
		}
	}
	return 0
}

func getInt(m map[string]interface{}, key string) int {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int:
			return val
		case int64:
			return int(val)
		case float64:
			return int(val)
		}
	}
	return 0
}

func getInt64FromMap(m map[string]interface{}, key string) int64 {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int64:
			return val
		case int:
			return int64(val)
		case float64:
			return int64(val)
		}
	}
	return 0
}

func getStr(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getBool(m map[string]interface{}, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func parseTimeRangeRFC3339(from, to string) (time.Time, time.Time, error) {
	start, err := time.Parse(time.RFC3339, from)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	end, err := time.Parse(time.RFC3339, to)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return start.UTC(), end.UTC(), nil
}

func parseTimeRange(from, to string) (time.Time, time.Time, error) {
	layout := "2006-01-02 15:04:05"
	start, err := time.Parse(layout, from)
	if err != nil {
		return parseTimeRangeRFC3339(from, to)
	}
	end, err := time.Parse(layout, to)
	if err != nil {
		return start, time.Now().UTC(), nil
	}
	return start.UTC(), end.UTC(), nil
}

func (tl *TimescaleLogger) ToSafeUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	v := make([]rune, 0, len(s))
	for i, r := range s {
		if r == utf8.RuneError {
			_, size := utf8.DecodeRuneInString(s[i:])
			if size == 1 {
				v = append(v, '?')
				continue
			}
		}
		v = append(v, r)
	}
	return string(v)
}

// volumeStatsHash hashes capacity metrics for a volume to support delta-logging.
func volumeStatsHash(availableGB, freePct float64) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(normalizeMapValue(availableGB)))
	_, _ = h.Write([]byte(normalizeMapValue(freePct)))
	return h.Sum64()
}

// diskRowHash hashes (data_mb, log_mb, free_mb) for a single database disk row.
func diskRowHash(dataMB, logMB, freeMB float64) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(normalizeMapValue(dataMB)))
	_, _ = h.Write([]byte(normalizeMapValue(logMB)))
	_, _ = h.Write([]byte(normalizeMapValue(freeMB)))
	return h.Sum64()
}

// fileIOHash hashes a single file IO row's rate values.
func fileIOHash(readLat, writeLat, readBPS, writeBPS float64) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(normalizeMapValue(readLat)))
	_, _ = h.Write([]byte(normalizeMapValue(writeLat)))
	_, _ = h.Write([]byte(normalizeMapValue(readBPS)))
	_, _ = h.Write([]byte(normalizeMapValue(writeBPS)))
	return h.Sum64()
}

func cpuSchedulerSnapshotHash(m map[string]interface{}) uint64 {
	h := fnv.New64a()
	// Stabilize by hashing key fields
	fields := []string{
		"max_workers_count", "scheduler_count", "cpu_count",
		"total_runnable_tasks_count", "total_work_queue_count",
		"total_current_workers_count", "active_workers_count",
		"total_active_request_count", "total_queued_request_count",
		"total_blocked_task_count",
	}
	for _, f := range fields {
		_, _ = h.Write([]byte(normalizeMapValue(m[f])))
	}
	return h.Sum64()
}
