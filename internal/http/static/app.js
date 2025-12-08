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

    // Refresh logs button
    if (refreshBtn) {
        refreshBtn.addEventListener('click', async function () {
            try {
                refreshBtn.disabled = true;
                refreshBtn.textContent = 'Loading...';

                const response = await fetch(`/api/projects/${projectId}/logs?lines=100`);
                const data = await response.json();

                if (data.logs) {
                    logOutput.textContent = data.logs;
                    scrollToBottom(logOutput);
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

        logOutput.textContent = '';

        eventSource.onmessage = function (event) {
            logOutput.textContent += event.data + '\n';
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
