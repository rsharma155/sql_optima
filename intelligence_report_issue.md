# SQL Server Intelligence Report - Audit Findings & Improvement Plan

## 1. Executive Summary
A detailed technical audit of the SQL Server Intelligence Report feature reveals that while a functional data-gathering pipeline exists, the "intelligence" layer relies heavily on hardcoded defaults, synthetic timelines, and underutilized expert knowledge bases. This results in reports that can feel "static" or "dummy" when telemetry is sparse or hardware configurations differ from hardcoded assumptions.

---

## 2. Identified Issues

### A. Hardware & Configuration Blind Spots
*   **Issue:** The `GetRawDataSnapshot` function in `intelligence_report_service.go` fails to fetch critical hardware context (NUMA nodes, Max Workers, Hyperthreading).
*   **Impact:** The system falls back to a "Standard Template" (8 Cores, 32GB RAM). 
*   **DBA Risk:** Thresholds for "Worker Thread Exhaustion" or "MAXDOP Recommendations" become mathematically incorrect for any server not matching the 8-core default.

### B. Synthetic Incident Timelines
*   **Issue:** The `buildTemplateIncidentTimeline` in `template_data.go` does not use actual historical timestamps. It takes current active issues and subtracts 15-minute increments from the current time to "create" a timeline.
*   **Impact:** Users see a fake sequence of events that does not reflect when performance degradation actually started.

### C. Underutilized Expert Knowledge Base
*   **Issue:** `sqlserver_features.go` contains a rich library of DBA best practices (Query Store, AGs, Indexing, TDE), but these are never called during the `Analyze()` loop.
*   **Impact:** The report misses "Strategic" and "Feature" health checks, focusing only on raw performance metrics (CPU/Memory/Disk).

### D. Static UI Placeholders
*   **Issue:** Several tabs (Performance Analysis > Waits, Queries) display static text stating data is "Not Collected," even if that data exists in other parts of the platform.
*   **Impact:** The report feels incomplete and requires the user to jump to other dashboards to get the full root-cause picture.

---

## 3. Threshold & Logic Deficiencies (DBA Review)

| Metric | Current Logic | Recommended Fix |
| :--- | :--- | :--- |
| **PLE (Page Life Expectancy)** | Static 300s floor. | Use `(Buffer Pool GB / 4) * 300` or a dynamic baseline percentile. |
| **I/O Latency** | Static 10ms-20ms floor. | Detect storage type; alert at >5ms for SSD/NVMe. |
| **Max Workers** | Hardcoded at 1024. | Calculate based on Core Count following official SQL Server formulas. |
| **TempDB Growth** | Alerts at 80% usage. | Should alert on *rate of growth* and *vlf_count* for proactive detection. |

---

## 4. Implementation Recommendations

### Fix 1: Hardware-Aware Thresholds
Update `DefaultServerConfig` and the service layer to retrieve the full property set from `sqlserver_server_properties`.
```go
// Required columns currently missing in scan:
// max_workers_count, numa_nodes, hyperthread_ratio, is_virtual
```

### Fix 2: Real-Time Timeline Integration
Modify the analysis engine to query `sqlserver_risk_health` history to identify the *exact* moment a threshold was first breached within the 24h window.

### Fix 3: Best Practice Rules Integration
Create a bridge between the YAML rule engine and the `SQLServerFeatures` map. If a feature like "Always On" is detected in the properties, trigger the corresponding health checks defined in `sqlserver_features.go`.

### Fix 4: PLE Calculation Refinement
Update `computeMemoryThresholds` in `thresholds.go`:
```go
// Current:
t.MemoryPLEMinSeconds = math.Max(300, pleBaseline*0.2)

// Recommended:
ramThreshold := (float64(config.TotalRAMGB) / 4.0) * 300.0
t.MemoryPLEMinSeconds = math.Max(ramThreshold, 300)
```

---

## 5. Proposed Fix Strategy

1.  **Phase 1 (Data):** Enhance the SQL queries to fetch complete server metadata.
2.  **Phase 2 (Logic):** Replace static narrative strings with dynamic, hardware-relative observations.
3.  **Phase 3 (Timeline):** Refactor the report generator to use actual `capture_timestamp` for the incident feed.
4.  **Phase 4 (Expansion):** Port the 20+ feature checks from `sqlserver_features.go` into the active rule engine.
