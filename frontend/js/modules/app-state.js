/** Shared application state (ES module). Legacy views still access `window.appState` from entry.js. */
export const appState = {
    config: null,
    currentInstanceIdx: 0,
    currentDatabase: 'all',
    activeViewId: 'global',
    isAuthenticated: false,
    navigationHistory: []
};
