// SQL Optima — Shared chart utilities and color palettes
export const charts = {
    colors: [
        '#3b82f6', // blue
        '#10b981', // emerald
        '#f59e0b', // amber
        '#ef4444', // red
        '#8b5cf6', // violet
        '#ec4899', // pink
        '#06b6d4', // cyan
        '#f97316', // orange
        '#14b8a6', // teal
        '#6366f1'  // indigo
    ],
    
    // Helper to get a color by index with wraparound
    getColor(idx) {
        return this.colors[idx % this.colors.length];
    }
};
