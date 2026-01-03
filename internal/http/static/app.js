// Servio - JavaScript Application
// Servio - JavaScript Application

// Robust syntax highlighter that avoids nesting by using placeholders
function highlightContent(text, mode) {
  if (!text) return "";

  // 1. Escape HTML entities that could break the display
  let content = text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");

  const tokens = [];
  const map = (cls, content) => {
    const id = `@@@TOKEN_${tokens.length}@@@`;
    tokens.push(`<span class="${cls}">${content}</span>`);
    return id;
  };

  if (mode === 'nginx') {
    // Comments
    content = content.replace(/(#.*)/g, (m) => map('nx-comment', m));
    // Strings (quoted)
    content = content.replace(/(["'])(?:(?=(\\?))\2.)*?\1/g, (m) => map('nx-string', m));
    // Keywords
    const keywords = /\b(server|location|listen|server_name|root|index|proxy_pass|proxy_set_header|try_files|rewrite|if|return|set|include|upstream|client_max_body_size|access_log|error_log|ssl_certificate|ssl_certificate_key|ssl_protocols|ssl_ciphers|gzip|proxy_cache|proxy_buffering|fastcgi_pass|alias|auth_basic|allow|deny|map|limit_req|add_header|proxy_hide_header|proxy_read_timeout|proxy_connect_timeout)\b/g;
    content = content.replace(keywords, (m) => map('nx-keyword', m));
    // Variables
    content = content.replace(/(\$[a-zA-Z0-9_]+)/g, (m) => map('nx-variable', m));
    // Numbers
    content = content.replace(/\b(\d+)\b/g, (m) => map('nx-number', m));
  } else if (mode === 'systemd') {
    // Sections
    content = content.replace(/^(\[.*\])/gm, (m) => map('nx-directive', m));
    // Comments
    content = content.replace(/(#.*|;.*)/g, (m) => map('nx-comment', m));
    // Keys
    content = content.replace(/^([a-zA-Z0-9]+)=/gm, (m, k) => map('nx-keyword', k) + '=');
  }

  // Restore tokens in one pass to avoid nesting issues
  return content.replace(/@@@TOKEN_(\d+)@@@/g, (match, id) => {
    return tokens[parseInt(id)];
  });
}

function highlightNginx(text) { return highlightContent(text, 'nginx'); }
function highlightSystemd(text) { return highlightContent(text, 'systemd'); }

document.addEventListener("DOMContentLoaded", function () {
  initLogControls();
  initThemeToggle();
  initDashboardRefresh();
  initCodeHighlighting();
});

// Auto-highlight any element with class 'code-block' and 'data-language'
function initCodeHighlighting() {
  document.querySelectorAll(".code-block").forEach((el) => {
    const lang = el.dataset.language;
    if (lang === "nginx") {
      el.innerHTML = highlightNginx(el.textContent);
    } else if (lang === "systemd") {
      el.innerHTML = highlightSystemd(el.textContent);
    }
  });
}

// Initialize theme toggle
function initThemeToggle() {
  const toggle = document.getElementById("theme-toggle");
  if (!toggle) return;

  // Initial setup based on localStorage (already applied in head, but for UI consistency)
  const currentTheme = localStorage.getItem("theme") || "dark";
  
  toggle.addEventListener("click", () => {
    const newTheme = document.documentElement.getAttribute("data-theme") === "dark" ? "light" : "dark";
    
    document.documentElement.setAttribute("data-theme", newTheme);
    localStorage.setItem("theme", newTheme);
  });
}

// Initialize log controls on project detail page
function initLogControls() {
  const logOutput = document.getElementById("log-output");
  const refreshBtn = document.getElementById("refresh-logs");
  const streamBtn = document.getElementById("stream-logs");

  if (!logOutput) return;

  const projectId = logOutput.dataset.projectId;
  let eventSource = null;
  let isStreaming = false;

  // Initialize click handlers for existing logs
  initLogLineHandlers();

  // Copy logs button
  const copyBtn = document.getElementById("copy-logs");
  if (copyBtn) {
    copyBtn.addEventListener("click", function () {
      // Get all log lines text
      const lines = Array.from(logOutput.querySelectorAll(".log-line"))
        .map((div) => div.textContent)
        .join("\n");

      if (!lines) return;

      copyToClipboard(lines)
        .then(() => {
          const originalText = copyBtn.textContent;
          copyBtn.textContent = "Copied!";
          copyBtn.classList.add("btn-success");
          copyBtn.classList.remove("btn-secondary");

          setTimeout(() => {
            copyBtn.textContent = originalText;
            copyBtn.classList.remove("btn-success");
            copyBtn.classList.add("btn-secondary");
          }, 2000);
        })
        .catch((err) => {
          console.error("Failed to copy logs:", err);
          // Visual feedback for failure
          copyBtn.textContent = "Failed";
          setTimeout(() => {
            copyBtn.textContent = "Copy";
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

          const successful = document.execCommand("copy");
          document.body.removeChild(textArea);

          if (successful) {
            resolve();
          } else {
            reject(new Error("execCommand copy failed"));
          }
        } catch (err) {
          reject(err);
        }
      });
    }
  }

  // Refresh logs button
  if (refreshBtn) {
    refreshBtn.addEventListener("click", async function () {
      try {
        refreshBtn.disabled = true;
        refreshBtn.textContent = "Loading...";

        const response = await fetch(`/api/projects/${projectId}/logs`);
        const data = await response.json();

        if (data.logs) {
          renderLogs(data.logs);
        }
      } catch (error) {
        console.error("Failed to refresh logs:", error);
      } finally {
        refreshBtn.disabled = false;
        refreshBtn.textContent = "Refresh";
      }
    });
  }

  // Stream logs button
  if (streamBtn) {
    streamBtn.addEventListener("click", function () {
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

    streamBtn.textContent = "Stop Stream";
    streamBtn.classList.remove("btn-primary");
    streamBtn.classList.add("btn-danger");

    logOutput.innerHTML = ""; // Clear logs

    eventSource.onmessage = function (event) {
      appendLogLine(event.data);
      scrollToBottom(logOutput);
    };

    eventSource.onerror = function (error) {
      console.error("SSE Error:", error);
      stopStreaming();
    };
  }

  function stopStreaming() {
    if (eventSource) {
      eventSource.close();
      eventSource = null;
    }

    isStreaming = false;
    streamBtn.textContent = "Stream Live";
    streamBtn.classList.remove("btn-danger");
    streamBtn.classList.add("btn-primary");
  }

  function renderLogs(logsText) {
    logOutput.innerHTML = "";
    if (!logsText) {
      logOutput.innerHTML = '<div class="empty-logs">No logs available</div>';
      return;
    }

    const lines = logsText.split("\n");
    lines.forEach((line) => {
      if (line.trim()) {
        appendLogLine(line);
      }
    });
    scrollToBottom(logOutput);
  }

  function appendLogLine(text) {
    const div = document.createElement("div");
    div.className = "log-line";
    div.innerHTML = formatLogLine(text); // Apply highlighting
    div.onclick = function () {
      this.classList.toggle("expanded");
    };
    logOutput.appendChild(div);

    // Remove empty state if present
    const emptyState = logOutput.querySelector(".empty-logs");
    if (emptyState) {
      emptyState.remove();
    }
  }

  function initLogLineHandlers() {
    // Highlight existing static logs
    const staticLines = logOutput.querySelectorAll(".log-line");
    staticLines.forEach((line) => {
      line.innerHTML = formatLogLine(line.textContent);
    });

    logOutput.addEventListener("click", function (e) {
      // Check if clicked element is inside a log-line or is the log-line itself
      const line = e.target.closest(".log-line");
      if (line) {
        line.classList.toggle("expanded");
      }
    });
  }

  // Helper to highlight log syntax
  function formatLogLine(text) {
    if (!text) return "";

    let content = text;
    let timestamp = "";

    // Attempt to parse systemd output format: ISO_TIMESTAMP HOSTNAME PROCESS: MESSAGE
    // Regex looks for: Start -> Timestamp -> Space -> Host -> Space -> Process -> Colon -> Space -> Message
    const systemdRegex = /^(\S+)\s+\S+\s+[^:]+:\s+(.*)/;
    const match = text.match(systemdRegex);

    if (match) {
      // Found systemd format
      timestamp = match[1]; // The ISO timestamp
      content = match[2]; // The actual message

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
    escaped = escaped.replace(
      /(INFO|Info|info)/g,
      '<span class="log-level-info">$1</span>'
    );
    escaped = escaped.replace(
      /(ERROR|Error|error|FAIL|Fail|fail)/g,
      '<span class="log-level-error">$1</span>'
    );
    escaped = escaped.replace(
      /(WARN|Warn|warn|WARNING)/g,
      '<span class="log-level-warn">$1</span>'
    );
    escaped = escaped.replace(
      /(DEBUG|Debug|debug)/g,
      '<span class="log-level-debug">$1</span>'
    );

    // HTTP Methods
    escaped = escaped.replace(
      /(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)/g,
      '<span class="log-method">$1</span>'
    );

    // HTTP Status Codes
    escaped = escaped.replace(
      /\b(2\d{2})\b/g,
      '<span class="log-status-2xx">$1</span>'
    );
    escaped = escaped.replace(
      /\b(3\d{2})\b/g,
      '<span class="log-status-3xx">$1</span>'
    );
    escaped = escaped.replace(
      /\b(4\d{2})\b/g,
      '<span class="log-status-4xx">$1</span>'
    );
    escaped = escaped.replace(
      /\b(5\d{2})\b/g,
      '<span class="log-status-5xx">$1</span>'
    );

    // Prepend timestamp if we extracted one
    if (timestamp) {
      return timestamp + escaped;
    }

    // If we didn't match systemd format, look for embedded timestamps in standard formats
    return escaped.replace(
      /(\d{4}\/\d{2}\/\d{2}\s+\d{2}:\d{2}:\d{2})/g,
      '<span class="log-timestamp">$1</span>'
    );
  }
}

function scrollToBottom(element) {
  element.scrollTop = element.scrollHeight;
}

// Auto-refresh stats and status on dashboard
function initDashboardRefresh() {
  const pulseBar = document.querySelector(".pulse-bar");
  if (!pulseBar && document.querySelectorAll(".service-card").length === 0)
    return;

  // Refresh every 2 seconds for a "pulse" feel
  setInterval(async function () {
    try {
      // Update Stats
      const statsRes = await fetch("/api/stats");
      const stats = await statsRes.json();
      updatePulseBar(stats);

      // Update Projects (if on dashboard)
      const projectsRes = await fetch("/api/projects");
      const projects = await projectsRes.json();
      updateProjectCards(projects, stats);
    } catch (error) {
      console.error("Failed to refresh dashboard:", error);
    }
  }, 2000);
}

function updatePulseBar(stats) {
  // Update CPU
  const cpuVal = stats.cpu_usage || 0;
  const cpuFill = document.getElementById("cpu-fill");
  const cpuText = document.getElementById("cpu-value");
  if (cpuFill) {
    cpuFill.style.width = `${cpuVal}%`;
    cpuFill.classList.toggle("warning", cpuVal > 70);
    cpuFill.classList.toggle("danger", cpuVal > 90);
  }
  if (cpuText) cpuText.textContent = `${Math.round(cpuVal)}%`;

  // Update Memory
  const memVal = stats.memory_usage || 0;
  const memFill = document.getElementById("mem-fill");
  const memText = document.getElementById("mem-value");
  if (memFill) {
    memFill.style.width = `${memVal}%`;
    memFill.classList.toggle("warning", memVal > 70);
    memFill.classList.toggle("danger", memVal > 90);
  }
  if (memText && stats.memory_total) {
    memText.textContent = `${stats.memory_used.toFixed(1)} / ${stats.memory_total.toFixed(1)} GB`;
  } else if (memText) {
    memText.textContent = `${Math.round(memVal)}%`;
  }

  // Update Disk
  const diskVal = stats.disk_usage || 0;
  const diskFill = document.getElementById("disk-fill");
  const diskText = document.getElementById("disk-value");
  if (diskFill) {
    diskFill.style.width = `${diskVal}%`;
    diskFill.classList.toggle("warning", diskVal > 80);
    diskFill.classList.toggle("danger", diskVal > 95);
  }
  if (diskText && stats.disk_total) {
    diskText.textContent = `${stats.disk_used.toFixed(0)} / ${stats.disk_total.toFixed(0)} GB`;
  } else if (diskText) {
    diskText.textContent = `${Math.round(diskVal)}%`;
  }

  const uptimeText = document.getElementById("uptime-value");
  if (uptimeText) uptimeText.textContent = stats.uptime;

  // Update OS Name
  const osText = document.getElementById("os-name-badge");
  if (osText && stats.os_name) {
    osText.textContent = `${stats.os_name} ${stats.os_version || ""}`.trim();
  }
}

function updateProjectCards(projects, stats) {
  if (!projects || !Array.isArray(projects)) return;
  
  projects.forEach((project) => {
    if (!project.services) return;
    
    project.services.forEach((svc) => {
      const svcEl = document.getElementById(`svc-${svc.id}`);
      if (!svcEl) return;

      // Update dot status
      const dot = svcEl.querySelector(".dot");
      if (dot) {
        dot.className = `dot status-${svc.status}`;
      }

      // Update stats text
      const statsEl = document.getElementById(`stats-${svc.id}`);
      if (statsEl) {
        let statsHtml = "";
        const s = stats.services ? stats.services[`servio-${svc.name}.service`] : null;
        
        if (s && s.active_state === "active") {
          statsHtml = `<span class="status-text running">Running</span>`;
          statsHtml += `
            <div class="stat-item">
              <span class="pulse-icon-tiny">⚡</span>
              <span>${s.cpu_usage.toFixed(1)}%</span>
            </div>
            <div class="stat-item">
              <span class="pulse-icon-tiny">💾</span>
              <span>${Math.round(s.memory_usage)}MB</span>
            </div>
          `;
        } else {
          statsHtml = `<span class="status-text">${svc.status || "unknown"}</span>`;
        }
        statsEl.innerHTML = statsHtml;
      }
    });
  });
}

