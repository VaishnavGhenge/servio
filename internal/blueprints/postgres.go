package blueprints

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"

	"servio/internal/storage"
)

// =============================================================================
// POSTGRESQL BLUEPRINT CONFIGURATION
// =============================================================================
// To add a new PostgreSQL version:
// 1. Add the version to `versions` slice below
// 2. Update `defaultVersion` if needed
// 3. The rest is handled automatically
// =============================================================================

var postgresVersions = []string{"16", "15", "14", "13"}
var postgresDefaultVersion = "16"

// Helper functions for parsing service config JSON
func parseServiceConfig(configStr string) map[string]interface{} {
	if configStr == "" {
		return make(map[string]interface{})
	}
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(configStr), &config); err != nil {
		slog.Warn("Failed to parse service config", "error", err)
		return make(map[string]interface{})
	}
	return config
}

func getConfigInt(config map[string]interface{}, key string, defaultVal int) int {
	if val, ok := config[key]; ok {
		switch v := val.(type) {
		case float64:
			return int(v)
		case int:
			return v
		}
	}
	return defaultVal
}

func getConfigString(config map[string]interface{}, key string, defaultVal string) string {
	if val, ok := config[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return defaultVal
}

// PostgresBlueprint provides configuration for PostgreSQL services
type PostgresBlueprint struct{}

func (p *PostgresBlueprint) Type() string {
	return "postgres"
}

func (p *PostgresBlueprint) Metadata() BlueprintMetadata {
	return BlueprintMetadata{
		Type:        "postgres",
		DisplayName: "PostgreSQL",
		Description: "Powerful, open source object-relational database",
		Icon:        "🐘",
		Versions:    postgresVersions,
		Default:     postgresDefaultVersion,
	}
}

func (p *PostgresBlueprint) Defaults(version string) BlueprintDefaults {
	if version == "" {
		version = postgresDefaultVersion
	}
	// Amazon Linux 2023 uses /usr/bin/postgres, RHEL/CentOS uses /usr/pgsql-XX/bin/postgres
	// Try Amazon Linux path first as it's simpler
	return BlueprintDefaults{
		Command:    "/usr/bin/postgres -D /var/lib/pgsql/data",
		User:       "postgres",
		WorkingDir: "/var/lib/pgsql",
		Hint:       fmt.Sprintf("PostgreSQL %s with data directory configured.", version),
	}
}

func (p *PostgresBlueprint) GenerateCommand(service *storage.Service) string {
	// Parse config for custom port
	config := parseServiceConfig(service.Config)
	port := getConfigInt(config, "db_port", 5432)

	// Build command with custom port and optional config overrides
	cmd := "/usr/bin/postgres -D /var/lib/pgsql/data"

	if port != 5432 {
		cmd += fmt.Sprintf(" -p %d", port)
	}

	// Add configuration parameters as command-line options
	if maxConn := getConfigInt(config, "max_connections", 0); maxConn > 0 {
		cmd += fmt.Sprintf(" -c max_connections=%d", maxConn)
	}

	if sharedBuffers := getConfigString(config, "shared_buffers", ""); sharedBuffers != "" {
		cmd += fmt.Sprintf(" -c shared_buffers=%s", sharedBuffers)
	}

	if workMem := getConfigString(config, "work_mem", ""); workMem != "" {
		cmd += fmt.Sprintf(" -c work_mem=%s", workMem)
	}

	return cmd
}

func (p *PostgresBlueprint) GenerateEnvironment(service *storage.Service) string {
	// Parse config JSON for custom settings
	config := parseServiceConfig(service.Config)

	// Get custom port or use default
	port := getConfigInt(config, "db_port", 5432)

	return fmt.Sprintf("PGDATA=/var/lib/pgsql/data\nPGPORT=%d", port)
}

func (p *PostgresBlueprint) GenerateSystemdOverrides(service *storage.Service) string {
	return `[Service]
Type=notify
User=postgres
Group=postgres
OOMScoreAdjust=-1000
LimitNOFILE=65536`
}

func (p *PostgresBlueprint) InstallDependencies(ctx context.Context, version string) error {
	if version == "" {
		version = postgresDefaultVersion
	}

	slog.Info("Installing PostgreSQL", "version", version)

	// Detect package manager and install accordingly
	var installErr error
	var isDebian bool

	// Try Amazon Linux 2023 / RHEL first
	cmd := exec.CommandContext(ctx, "sudo", "dnf", "install", "-y", fmt.Sprintf("postgresql%s-server", version))
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Fallback to apt for Debian/Ubuntu
		slog.Info("dnf not available, trying apt", "error", string(output))
		isDebian = true
		cmd = exec.CommandContext(ctx, "sudo", "apt-get", "update")
		cmd.Run() // Update package lists

		cmd = exec.CommandContext(ctx, "sudo", "apt-get", "install", "-y", fmt.Sprintf("postgresql-%s", version))
		output, installErr = cmd.CombinedOutput()
		if installErr != nil {
			return fmt.Errorf("failed to install postgresql: %s - %w", string(output), installErr)
		}
	}

	// Initialize database
	// On Debian/Ubuntu, PostgreSQL auto-initializes during package installation
	// On RHEL/Amazon Linux, we need to run the setup script
	if !isDebian {
		// Try multiple init methods for different RHEL/Amazon Linux versions
		// Method 1: postgresql-setup (Amazon Linux 2023)
		initCmd := exec.CommandContext(ctx, "sudo", "postgresql-setup", "--initdb")
		if err := initCmd.Run(); err != nil {
			slog.Debug("postgresql-setup failed, trying version-specific", "error", err)

			// Method 2: Version-specific setup script
			initCmd = exec.CommandContext(ctx, "sudo", fmt.Sprintf("/usr/pgsql-%s/bin/postgresql-%s-setup", version, version), "initdb")
			if err := initCmd.Run(); err != nil {
				slog.Debug("Version-specific setup failed, trying initdb directly", "error", err)

				// Method 3: Direct initdb as postgres user
				initCmd = exec.CommandContext(ctx, "sudo", "-u", "postgres", "initdb", "-D", "/var/lib/pgsql/data")
				if err := initCmd.Run(); err != nil {
					slog.Warn("Database init failed (may already exist)", "error", err)
				}
			}
		}
	}

	slog.Info("PostgreSQL installation completed", "version", version, "debian", isDebian)
	return nil
}
