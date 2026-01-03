package blueprints

import (
	"context"
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
	return BlueprintDefaults{
		Command:    fmt.Sprintf("/usr/pgsql-%s/bin/postgres -D /var/lib/pgsql/%s/data", version, version),
		User:       "postgres",
		WorkingDir: "/var/lib/pgsql",
		Hint:       fmt.Sprintf("PostgreSQL %s with data directory configured.", version),
	}
}

func (p *PostgresBlueprint) GenerateCommand(service *storage.Service) string {
	version := service.Version
	if version == "" {
		version = postgresDefaultVersion
	}
	return fmt.Sprintf("/usr/pgsql-%s/bin/postgres -D /var/lib/pgsql/%s/data", version, version)
}

func (p *PostgresBlueprint) GenerateEnvironment(service *storage.Service) string {
	version := service.Version
	if version == "" {
		version = postgresDefaultVersion
	}
	return fmt.Sprintf("PGDATA=/var/lib/pgsql/%s/data\nPGPORT=5432", version)
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

	// Try Amazon Linux 2023 / RHEL first
	cmd := exec.CommandContext(ctx, "sudo", "dnf", "install", "-y", fmt.Sprintf("postgresql%s-server", version))
	if err := cmd.Run(); err != nil {
		// Fallback to apt for Debian/Ubuntu
		slog.Debug("dnf failed, trying apt", "error", err)
		cmd = exec.CommandContext(ctx, "sudo", "apt-get", "install", "-y", fmt.Sprintf("postgresql-%s", version))
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to install postgresql: %w", err)
		}
	}

	// Initialize database
	initCmd := exec.CommandContext(ctx, "sudo", fmt.Sprintf("/usr/pgsql-%s/bin/postgresql-%s-setup", version, version), "--initdb")
	if err := initCmd.Run(); err != nil {
		slog.Warn("Database init failed (may already exist)", "error", err)
	}

	return nil
}
