package blueprints

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"

	"servio/internal/storage"
)

// =============================================================================
// DJANGO/GUNICORN BLUEPRINT CONFIGURATION
// =============================================================================
// To add a new Gunicorn version:
// 1. Add the version to `versions` slice below
// 2. Update `defaultVersion` if needed
// =============================================================================

var djangoVersions = []string{"22.0", "21.2", "20.1"}
var djangoDefaultVersion = "22.0"
var djangoDefaultWorkers = 2
var djangoDefaultBind = "0.0.0.0:8000"

// DjangoConfig holds Django-specific configuration from the service Config JSON
type DjangoConfig struct {
	WsgiModule   string `json:"wsgi_module"`  // e.g., "myproject.wsgi:application"
	Workers      int    `json:"workers"`      // Number of gunicorn workers
	BindAddress  string `json:"bind_address"` // e.g., "0.0.0.0:8000"
	VenvPath     string `json:"venv_path"`    // Path to virtual environment
	Requirements string `json:"requirements"` // Path to requirements.txt
}

// DjangoBlueprint provides configuration for Django/Gunicorn services
type DjangoBlueprint struct{}

func (d *DjangoBlueprint) Type() string {
	return "django"
}

func (d *DjangoBlueprint) Metadata() BlueprintMetadata {
	return BlueprintMetadata{
		Type:        "django",
		DisplayName: "Django",
		Description: "Python web framework with Gunicorn WSGI server",
		Icon:        "🐍",
		Versions:    djangoVersions,
		Default:     djangoDefaultVersion,
	}
}

func (d *DjangoBlueprint) Defaults(version string) BlueprintDefaults {
	return BlueprintDefaults{
		Command:    fmt.Sprintf("gunicorn --workers %d --bind %s app.wsgi:application", djangoDefaultWorkers, djangoDefaultBind),
		User:       "www-data",
		WorkingDir: "/var/www/app",
		Hint:       "Gunicorn with default workers and bind address. Customize in the command.",
	}
}

func (d *DjangoBlueprint) parseConfig(service *storage.Service) DjangoConfig {
	cfg := DjangoConfig{
		WsgiModule:  "app.wsgi:application",
		Workers:     djangoDefaultWorkers,
		BindAddress: djangoDefaultBind,
	}

	if service.Config != "" {
		if err := json.Unmarshal([]byte(service.Config), &cfg); err != nil {
			slog.Warn("Failed to parse Django config", "error", err)
		}
	}

	return cfg
}

func (d *DjangoBlueprint) GenerateCommand(service *storage.Service) string {
	cfg := d.parseConfig(service)

	workDir := service.WorkingDir
	if workDir == "" {
		workDir = "/var/www/app"
	}

	// If venv is specified, use the gunicorn from venv
	gunicornPath := "gunicorn"
	if cfg.VenvPath != "" {
		gunicornPath = filepath.Join(cfg.VenvPath, "bin", "gunicorn")
	}

	workers := cfg.Workers
	if workers <= 0 {
		workers = djangoDefaultWorkers
	}

	return fmt.Sprintf("%s --workers %d --bind %s %s",
		gunicornPath, workers, cfg.BindAddress, cfg.WsgiModule)
}

func (d *DjangoBlueprint) GenerateEnvironment(service *storage.Service) string {
	cfg := d.parseConfig(service)

	env := "DJANGO_SETTINGS_MODULE=app.settings\n"
	env += "PYTHONDONTWRITEBYTECODE=1\n"
	env += "PYTHONUNBUFFERED=1\n"

	if cfg.VenvPath != "" {
		env += fmt.Sprintf("VIRTUAL_ENV=%s\n", cfg.VenvPath)
		env += fmt.Sprintf("PATH=%s/bin:$PATH\n", cfg.VenvPath)
	}

	return env
}

func (d *DjangoBlueprint) GenerateSystemdOverrides(service *storage.Service) string {
	return `[Service]
Type=notify
KillMode=mixed
TimeoutStopSec=5`
}

func (d *DjangoBlueprint) InstallDependencies(ctx context.Context, version string) error {
	if version == "" {
		version = djangoDefaultVersion
	}

	slog.Info("Installing Gunicorn", "version", version)

	// Install Python and pip if not present
	cmd := exec.CommandContext(ctx, "sudo", "dnf", "install", "-y", "python3", "python3-pip")
	if err := cmd.Run(); err != nil {
		// Fallback to apt
		cmd = exec.CommandContext(ctx, "sudo", "apt-get", "install", "-y", "python3", "python3-pip")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to install python: %w", err)
		}
	}

	// Install gunicorn globally (user can override with venv)
	pipCmd := exec.CommandContext(ctx, "pip3", "install", fmt.Sprintf("gunicorn==%s", version))
	if err := pipCmd.Run(); err != nil {
		slog.Warn("Failed to install specific gunicorn version, trying latest", "error", err)
		pipCmd = exec.CommandContext(ctx, "pip3", "install", "gunicorn")
		if err := pipCmd.Run(); err != nil {
			return fmt.Errorf("failed to install gunicorn: %w", err)
		}
	}

	return nil
}
