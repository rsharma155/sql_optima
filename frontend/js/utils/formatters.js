export const formatters = {
    number(val, decimals = 0) {
        if (val === null || val === undefined || isNaN(val)) return '--';
        return Number(val).toLocaleString(undefined, {
            minimumFractionDigits: decimals,
            maximumFractionDigits: decimals
        });
    },
    compactNumber(val) {
        if (val === null || val === undefined || isNaN(val)) return '--';
        return Intl.NumberFormat('en-US', {
            notation: "compact",
            maximumFractionDigits: 1
        }).format(val);
    },
    bytes(val) {
        if (val === null || val === undefined || isNaN(val)) return '--';
        const units = ['B', 'KB', 'MB', 'GB', 'TB'];
        let l = 0, n = parseInt(val, 10) || 0;
        while(n >= 1024 && ++l){
            n = n/1024;
        }
        return n.toFixed(n < 10 && l > 0 ? 1 : 0) + ' ' + units[l];
    }
};
