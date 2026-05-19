/*
 * SQL Optima — Collector Configuration Utilities
 */
(function() {
    window.collectorConfig = {
        intervals: {},
        
        async loadIntervals() {
            try {
                const resp = await window.apiClient.authenticatedFetch('/api/health/intervals');
                if (resp.ok) {
                    this.intervals = await resp.json();
                }
            } catch (e) {
                console.error("[CollectorConfig] Failed to load intervals", e);
            }
        },

        getInterval(name, defaultMs) {
            if (this.intervals[name]) {
                return this.intervals[name] * 1000;
            }
            return defaultMs;
        }
    };
})();
