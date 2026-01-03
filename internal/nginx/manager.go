package nginx

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"servio/internal/storage"
)

// =============================================================================
// NGINX CONFIGURATION
// =============================================================================
// Modify these paths based on your Linux distribution:
// - Amazon Linux / RHEL: /etc/nginx/conf.d/
// - Ubuntu / Debian: /etc/nginx/sites-available/ + sites-enabled/
// =============================================================================

var (
	// SitesAvailableDir is where site configs are stored
	SitesAvailableDir = "/etc/nginx/conf.d"

	// SitesEnabledDir is for symlinks (Ubuntu style), empty to disable
	SitesEnabledDir = ""

	// NginxBinary is the path to nginx
	NginxBinary = "nginx"
)

// Manager handles Nginx site configuration
type Manager struct {
	sitesAvailableDir string
	sitesEnabledDir   string
}

// NewManager creates a new Nginx manager
func NewManager() *Manager {
	m := &Manager{
		sitesAvailableDir: "/etc/nginx/conf.d",
		sitesEnabledDir:   "",
	}
	return m
}

// Configure sets paths based on the provided distro
func (m *Manager) Configure(distro string) {
	if distro == "ubuntu" || distro == "debian" {
		m.sitesAvailableDir = "/etc/nginx/sites-available"
		m.sitesEnabledDir = "/etc/nginx/sites-enabled"
		slog.Info("Distro set to Ubuntu/Debian, using sites-available pattern")
	} else {
		m.sitesAvailableDir = "/etc/nginx/conf.d"
		m.sitesEnabledDir = ""
		slog.Info("Distro set to Amazon Linux/RHEL, using conf.d pattern")
	}
}

// GenerateSiteConfig generates an Nginx site configuration for a project, respecting Project.NginxRaw if set
func (m *Manager) GenerateSiteConfig(project *storage.Project) (string, error) {
	if project.NginxRaw != "" {
		return project.NginxRaw, nil
	}
	return m.GenerateDefaultConfig(project)
}

// GenerateDefaultConfig generates the default Nginx site configuration
func (m *Manager) GenerateDefaultConfig(project *storage.Project) (string, error) {
	if project.Domain == "" {
		return "", fmt.Errorf("project has no domain configured")
	}

	// Build upstream blocks for services with ports
	var upstreams []string
	var locations []string
	var primaryPort int

	for _, svc := range project.Services {
		if svc.Port > 0 {
			if primaryPort == 0 {
				primaryPort = svc.Port
			}
			upstreams = append(upstreams, fmt.Sprintf(`    # %s
    server 127.0.0.1:%d;`, svc.Name, svc.Port))
		}
	}

	// Default to port 8000 if no services have ports configured
	if primaryPort == 0 {
		primaryPort = 8000
	}

	// Default location proxies to primary service
	locations = append(locations, fmt.Sprintf(`    location / {
        proxy_pass http://127.0.0.1:%d;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 86400;
    }`, primaryPort))

	// Static files location (common pattern)
	locations = append(locations, `    location /static/ {
        alias /var/www/static/;
        expires 30d;
        add_header Cache-Control "public, immutable";
    }`)

	config := fmt.Sprintf(`# Managed by Servio - Project: %s
# Generated: Do not edit manually, changes will be overwritten

server {
    listen 80;
    server_name %s;

    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;

    # Logging
    access_log /var/log/nginx/%s.access.log;
    error_log /var/log/nginx/%s.error.log;

%s

    # Error pages
    error_page 502 503 504 /50x.html;
    location = /50x.html {
        root /usr/share/nginx/html;
    }
}
`, project.Name, project.Domain, project.Name, project.Name, strings.Join(locations, "\n\n"))

	return config, nil
}

// SiteConfigPath returns the path where the site config will be written
func (m *Manager) SiteConfigPath(project *storage.Project) string {
	filename := fmt.Sprintf("servio-%d-%s.conf", project.ID, sanitizeName(project.Name))
	return filepath.Join(m.sitesAvailableDir, filename)
}

// InstallSite writes the site config and reloads Nginx
func (m *Manager) InstallSite(ctx context.Context, project *storage.Project) error {
	config, err := m.GenerateSiteConfig(project)
	if err != nil {
		return fmt.Errorf("failed to generate config: %w", err)
	}

	configPath := m.SiteConfigPath(project)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write config file
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	slog.Info("Wrote nginx config", "path", configPath, "project", project.Name)

	// Create symlink if using sites-enabled pattern
	if m.sitesEnabledDir != "" {
		enabledPath := filepath.Join(m.sitesEnabledDir, filepath.Base(configPath))
		os.Remove(enabledPath) // Remove existing symlink
		if err := os.Symlink(configPath, enabledPath); err != nil {
			return fmt.Errorf("failed to create symlink: %w", err)
		}
		slog.Info("Created symlink", "path", enabledPath)
	}

	// Test configuration
	if err := m.TestConfig(ctx); err != nil {
		// Rollback: remove the config
		os.Remove(configPath)
		return fmt.Errorf("nginx config test failed: %w", err)
	}

	// Reload Nginx
	if err := m.Reload(ctx); err != nil {
		return fmt.Errorf("failed to reload nginx: %w", err)
	}

	return nil
}

// UninstallSite removes the site config and reloads Nginx
func (m *Manager) UninstallSite(ctx context.Context, project *storage.Project) error {
	configPath := m.SiteConfigPath(project)

	// Remove symlink if exists
	if m.sitesEnabledDir != "" {
		enabledPath := filepath.Join(m.sitesEnabledDir, filepath.Base(configPath))
		os.Remove(enabledPath)
	}

	// Remove config file
	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove config: %w", err)
	}

	slog.Info("Removed nginx config", "path", configPath, "project", project.Name)

	// Reload Nginx
	return m.Reload(ctx)
}

// TestConfig tests the Nginx configuration
func (m *Manager) TestConfig(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "sudo", NginxBinary, "-t")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("config test failed: %s", string(output))
	}
	return nil
}

// Reload reloads the Nginx configuration
func (m *Manager) Reload(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "sudo", "systemctl", "reload", "nginx")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to reload nginx: %w", err)
	}
	slog.Info("Reloaded nginx")
	return nil
}

// IsInstalled checks if Nginx is installed
func (m *Manager) IsInstalled() bool {
	_, err := exec.LookPath(NginxBinary)
	return err == nil
}

// SiteExists checks if a site config exists for the project
func (m *Manager) SiteExists(project *storage.Project) bool {
	_, err := os.Stat(m.SiteConfigPath(project))
	return err == nil
}

// sanitizeName removes special characters from a name for use in filenames
func sanitizeName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	// Keep only alphanumeric and hyphens
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	return result.String()
}
