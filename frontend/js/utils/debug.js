/**
 * Opt-in client debug logging. Set localStorage APP_DEBUG=1 and reload to enable.
 * Default is off so DevTools consoles stay clean in production.
 */
(function () {
    function readDebug() {
        try {
            return localStorage.getItem('APP_DEBUG') === '1';
        } catch (e) {
            return false;
        }
    }
    window.__APP_DEBUG__ = readDebug();
    window.appDebug = function () {
        if (window.__APP_DEBUG__) {
            console.log.apply(console, arguments);
        }
    };
})();
