// Servio - JavaScript Application

document.addEventListener('DOMContentLoaded', function () {
    initLogControls();
});

// Initialize log controls on project detail page
function initLogControls() {
    const logOutput = document.getElementById('log-output');
    const refreshBtn = document.getElementById('refresh-logs');
    const streamBtn = document.getElementById('stream-logs');

    if (!logOutput) return;

    const projectId = logOutput.dataset.projectId;
    let eventSource = null;
    let isStreaming = false;

    // Initialize click handlers for existing logs
    initLogLineHandlers();

    // Copy logs button
    const copyBtn = document.getElementById('copy-logs');
    if (copyBtn) {
        copyBtn.addEventListener('click', function () {
            // Get all log lines text
            const lines = Array.from(logOutput.querySelectorAll('.log-line'))
                .map(div => div.textContent)
                .join('\n');

            if (!lines) return;

            copyToClipboard(lines).then(() => {
                const originalText = copyBtn.textContent;
                copyBtn.textContent = 'Copied!';
                copyBtn.classList.add('btn-success');
                copyBtn.classList.remove('btn-secondary');

                setTimeout(() => {
                    copyBtn.textContent = originalText;
                    copyBtn.classList.remove('btn-success');
                    copyBtn.classList.add('btn-secondary');
                }, 2000);
            }).catch(err => {
                console.error('Failed to copy logs:', err);
                // Visual feedback for failure
                copyBtn.textContent = 'Failed';
                setTimeout(() => {
                    copyBtn.textContent = 'Copy';
                }, 2000);
            });
        });
    }

    // Helper to copy text with fallback for HTTP/non-secure contexts
    function copyToClipboard(text) {
        if (navigator.clipboard && window.isSecureContext) {
            return navigator.clipboard.writeText(text);
        } else {
            // Fallback for non-secure contexts (HTTP)
            return new Promise((resolve, reject) => {
                try {
                    const textArea = document.createElement("textarea");
                    textArea.value = text;

                    // Ensure it's not visible but part of DOM
                    textArea.style.position = "fixed";
                    textArea.style.left = "-9999px";
                    textArea.style.top = "0";
                    document.body.appendChild(textArea);

                    textArea.focus();
                    textArea.select();

                    const successful = document.execCommand('copy');
                    document.body.removeChild(textArea);

                    if (successful) {
                        resolve();
                    } else {
                        reject(new Error('execCommand copy failed'));
                    }
                } catch (err) {
                    reject(err);
                }
            });
        }
    }

    // Refresh logs button
    if (refreshBtn) {
        refreshBtn.addEventListener('click', async function () {
            try {
                refreshBtn.disabled = true;
                refreshBtn.textContent = 'Loading...';

                const response = await fetch(`/api/projects/${projectId}/logs`);
                const data = await response.json();

                if (data.logs) {
                    renderLogs(data.logs);
                }
            } catch (error) {
                console.error('Failed to refresh logs:', error);
            } finally {
                refreshBtn.disabled = false;
                refreshBtn.textContent = 'Refresh';
            }
        });
    }

    // Stream logs button
    if (streamBtn) {
        streamBtn.addEventListener('click', function () {
            if (isStreaming) {
                stopStreaming();
            } else {
                startStreaming();
            }
        });
    }

    function startStreaming() {
        if (eventSource) {
            eventSource.close();
        }

        eventSource = new EventSource(`/api/projects/${projectId}/logs/stream`);
        isStreaming = true;

        streamBtn.textContent = 'Stop Stream';
        streamBtn.classList.remove('btn-primary');
        streamBtn.classList.add('btn-danger');

        logOutput.innerHTML = ''; // Clear logs

        eventSource.onmessage = function (event) {
            appendLogLine(event.data);
            scrollToBottom(logOutput);
        };

        eventSource.onerror = function (error) {
            console.error('SSE Error:', error);
            stopStreaming();
        };
    }

    function stopStreaming() {
        if (eventSource) {
            eventSource.close();
            eventSource = null;
        }

        isStreaming = false;
        streamBtn.textContent = 'Stream Live';
        streamBtn.classList.remove('btn-danger');
        streamBtn.classList.add('btn-primary');
    }

    function renderLogs(logsText) {
        logOutput.innerHTML = '';
        if (!logsText) {
            logOutput.innerHTML = '<div class="empty-logs">No logs available</div>';
            return;
        }

        const lines = logsText.split('\n');
        lines.forEach(line => {
            if (line.trim()) {
                appendLogLine(line);
            }
        });
        scrollToBottom(logOutput);
    }

    function appendLogLine(text) {
        const div = document.createElement('div');
        div.className = 'log-line';
        div.innerHTML = formatLogLine(text); // Apply highlighting
        div.onclick = function () {
            this.classList.toggle('expanded');
        };
        logOutput.appendChild(div);

        // Remove empty state if present
        const emptyState = logOutput.querySelector('.empty-logs');
        if (emptyState) {
            emptyState.remove();
        }
    }

    function initLogLineHandlers() {
        // Highlight existing static logs
        const staticLines = logOutput.querySelectorAll('.log-line');
        staticLines.forEach(line => {
            line.innerHTML = formatLogLine(line.textContent);
        });

        logOutput.addEventListener('click', function (e) {
            // Check if clicked element is inside a log-line or is the log-line itself
            const line = e.target.closest('.log-line');
            if (line) {
                line.classList.toggle('expanded');
            }
        });
    }

    // Helper to highlight log syntax
    function formatLogLine(text) {
        if (!text) return '';

        let content = text;
        let timestamp = '';

        // Attempt to parse systemd output format: ISO_TIMESTAMP HOSTNAME PROCESS: MESSAGE
        // Regex looks for: Start -> Timestamp -> Space -> Host -> Space -> Process -> Colon -> Space -> Message
        const systemdRegex = /^(\S+)\s+\S+\s+[^:]+:\s+(.*)/;
        const match = text.match(systemdRegex);

        if (match) {
            // Found systemd format
            timestamp = match[1]; // The ISO timestamp
            content = match[2];   // The actual message

            // Format timestamp nicely (optional, e.g. drop date if needed, but keeping ISO for now)
            // Just wrap it
            timestamp = `<span class="log-timestamp">${timestamp}</span>`;
        } else {
            // Fallback for lines that don't match (maybe raw output or stack traces)
            // Try to find a timestamp at the start anyway
            content = text;
        }

        // Escape HTML content to prevent injection
        let escaped = content
            .replace(/&/g, "&amp;")
            .replace(/</g, "&lt;")
            .replace(/>/g, "&gt;")
            .replace(/"/g, "&quot;")
            .replace(/'/g, "&#039;");

        // Log Levels Highlighting within the message
        escaped = escaped.replace(/(INFO|Info|info)/g, '<span class="log-level-info">$1</span>');
        escaped = escaped.replace(/(ERROR|Error|error|FAIL|Fail|fail)/g, '<span class="log-level-error">$1</span>');
        escaped = escaped.replace(/(WARN|Warn|warn|WARNING)/g, '<span class="log-level-warn">$1</span>');
        escaped = escaped.replace(/(DEBUG|Debug|debug)/g, '<span class="log-level-debug">$1</span>');

        // HTTP Methods
        escaped = escaped.replace(/(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)/g, '<span class="log-method">$1</span>');

        // HTTP Status Codes
        escaped = escaped.replace(/\b(2\d{2})\b/g, '<span class="log-status-2xx">$1</span>');
        escaped = escaped.replace(/\b(3\d{2})\b/g, '<span class="log-status-3xx">$1</span>');
        escaped = escaped.replace(/\b(4\d{2})\b/g, '<span class="log-status-4xx">$1</span>');
        escaped = escaped.replace(/\b(5\d{2})\b/g, '<span class="log-status-5xx">$1</span>');

        // Prepend timestamp if we extracted one
        if (timestamp) {
            return timestamp + escaped;
        }

        // If we didn't match systemd format, look for embedded timestamps in standard formats
        return escaped.replace(/(\d{4}\/\d{2}\/\d{2}\s+\d{2}:\d{2}:\d{2})/g, '<span class="log-timestamp">$1</span>');
    }
}

function scrollToBottom(element) {
    element.scrollTop = element.scrollHeight;
}

// Auto-refresh status on dashboard
function initDashboardRefresh() {
    const serviceCards = document.querySelectorAll('.service-card');
    if (serviceCards.length === 0) return;

    // Refresh every 10 seconds
    setInterval(async function () {
        try {
            const response = await fetch('/api/projects');
            const projects = await response.json();

            projects.forEach(project => {
                const card = document.querySelector(`.service-card[data-id="${project.id}"]`);
                if (card) {
                    const badge = card.querySelector('.status-badge');
                    if (badge) {
                        badge.textContent = project.status;
                        badge.className = `status-badge status-${project.status}`;
                    }
                }
            });
        } catch (error) {
            console.error('Failed to refresh status:', error);
        }
    }, 10000);
}
