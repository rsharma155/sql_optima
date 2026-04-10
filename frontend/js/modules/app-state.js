/** Shared application state (ES module). Legacy views still access `window.appState` from entry.js. */
export const appState = {
    config: null,
    currentInstanceIdx: 0,
    currentDatabase: 'all',
    activeViewId: 'global',
    isAuthenticated: false,
    /** When true, API expects JWT on /api/config and dashboard routes (AUTH_REQUIRED=1). */
    authRequired: false,
    authMode: 'local',
    navigationHistory: []
};
