import { apiClient } from '../modules/app-client.js';

export const api = {
    async get(url) {
        const response = await apiClient.apiFetch(url);
        if (!response.ok) {
            const err = await response.json().catch(() => ({}));
            throw new Error(err.error || `HTTP ${response.status}`);
        }
        return await response.json();
    },
    async post(url, body) {
        const response = await apiClient.apiFetch(url, {
            method: 'POST',
            body: JSON.stringify(body)
        });
        if (!response.ok) {
            const err = await response.json().catch(() => ({}));
            throw new Error(err.error || `HTTP ${response.status}`);
        }
        return await response.json();
    }
};
