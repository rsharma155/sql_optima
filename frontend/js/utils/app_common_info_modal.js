/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Global utility for showing informational modals.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

(function () {
    'use strict';

    /**
     * Renders a generic information modal with markdown-lite support.
     * @param {string} title - The title of the modal.
     * @param {string} content - The text content (supports basic markdown).
     * @param {object} options - Customization options (width, maxHeight, etc.).
     */
    window.showAppInfoModal = function (title, content, options = {}) {
        const existingModal = document.getElementById('info-modal');
        if (existingModal) existingModal.remove();

        const modal = document.createElement('div');
        modal.id = 'info-modal';
        modal.className = 'modal-overlay';
        modal.style.zIndex = '100000';

        // Simple Markdown-lite to HTML conversion
        let htmlContent = content;
        if (typeof content === 'string') {
            htmlContent = content
                .replace(/^# (.*$)/gm, '<h1 style="border-bottom: 1px solid var(--border-color); padding-bottom: 0.5rem; margin-top: 1.5rem;">$1</h1>')
                .replace(/^### (.*$)/gm, '<h3 style="margin-top: 1.25rem; color: var(--accent-blue);">$1</h3>')
                .replace(/\n\n/g, '</p><p>')
                .replace(/\n/g, '<br>')
                .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
                .replace(/\*(.*?)\*/g, '<em>$1</em>')
                .replace(/`(.*?)`/g, '<code style="background: rgba(0,0,0,0.2); padding: 0.1rem 0.3rem; border-radius: 3px;">$1</code>');
            
            if (!htmlContent.startsWith('<h1')) {
                htmlContent = '<p>' + htmlContent + '</p>';
            }
        }

        const width = options.width || '500px';
        const maxHeight = options.maxHeight || '85vh';

        modal.innerHTML = `
            <div class="modal-content" style="max-width: ${width}; width: 92%; max-height: ${maxHeight}; display: flex; flex-direction: column; overflow: hidden; background: var(--bg-surface); color: var(--text-primary);">
                <div class="modal-header" style="flex-shrink: 0; display: flex; align-items: center; padding: 1rem 1.5rem; border-bottom: 1px solid var(--border-color);">
                    <i class="fa-solid fa-circle-info text-accent" style="margin-right: 0.75rem; font-size: 1.2rem;"></i>
                    <h3 style="flex: 1; margin: 0; font-size: 1.1rem;">${title}</h3>
                    <button class="btn btn-icon" data-action="close-id" data-target="info-modal" style="background: transparent; border: none; font-size: 1.5rem; color: var(--text-muted); cursor: pointer; padding: 0; line-height: 1;">&times;</button>
                </div>
                <div class="modal-body" style="overflow-y: auto; flex: 1; padding: 1.5rem; font-size: 0.95rem; line-height: 1.6;">
                    ${htmlContent}
                </div>
                <div class="modal-footer" style="padding: 1rem 1.5rem; border-top: 1px solid var(--border-color); text-align: right; background: rgba(0,0,0,0.05);">
                    <button class="btn btn-sm btn-outline" data-action="close-id" data-target="info-modal">Close</button>
                </div>
            </div>
        `;

        document.body.appendChild(modal);

        // Escape key to close
        const onKeyDown = (e) => {
            if (e.key === 'Escape') {
                modal.remove();
                document.removeEventListener('keydown', onKeyDown);
            }
        };
        document.addEventListener('keydown', onKeyDown);

        // Overlay click to close
        modal.addEventListener('click', (e) => {
            if (e.target === modal) {
                modal.remove();
                document.removeEventListener('keydown', onKeyDown);
            }
        });
    };
})();
