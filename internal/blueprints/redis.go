package blueprints

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"

	"servio/internal/storage"
)

// =============================================================================
// REDIS BLUEPRINT CONFIGURATION
// =============================================================================
// To add a new Redis version:
// 1. Add the version to `versions` slice below
// 2. Update `defaultVersion` if needed
// =============================================================================

var redisVersions = []string{"7", "6"}
var redisDefaultVersion = "7"

// RedisBlueprint provides configuration for Redis services
type RedisBlueprint struct{}

func (r *RedisBlueprint) Type() string {
	return "redis"
}

func (r *RedisBlueprint) Metadata() BlueprintMetadata {
	return BlueprintMetadata{
		Type:        "redis",
		DisplayName: "Redis",
		Description: "In-memory data structure store for caching and messaging",
		Icon:        "🔴",
		Versions:    redisVersions,
		Default:     redisDefaultVersion,
	}
}

func (r *RedisBlueprint) Defaults(version string) BlueprintDefaults {
	return BlueprintDefaults{
		Command:    "/usr/bin/redis-server /etc/redis/redis.conf",
		User:       "redis",
		WorkingDir: "/var/lib/redis",
		Hint:       "Redis with default configuration file.",
	}
}

func (r *RedisBlueprint) GenerateCommand(service *storage.Service) string {
	return "/usr/bin/redis-server /etc/redis/redis.conf"
}

func (r *RedisBlueprint) GenerateEnvironment(service *storage.Service) string {
	return ""
}

func (r *RedisBlueprint) GenerateSystemdOverrides(service *storage.Service) string {
	return `[Service]
Type=notify
User=redis
Group=redis
RuntimeDirectory=redis
RuntimeDirectoryMode=0755
LimitNOFILE=65535`
}

func (r *RedisBlueprint) InstallDependencies(ctx context.Context, version string) error {
	if version == "" {
		version = redisDefaultVersion
	}

	slog.Info("Installing Redis", "version", version)

	// Try Amazon Linux 2023 / RHEL first
	cmd := exec.CommandContext(ctx, "sudo", "dnf", "install", "-y", "redis")
	if err := cmd.Run(); err != nil {
		// Fallback to apt for Debian/Ubuntu
		slog.Debug("dnf failed, trying apt", "error", err)
		cmd = exec.CommandContext(ctx, "sudo", "apt-get", "install", "-y", "redis-server")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to install redis: %w", err)
		}
	}

	return nil
}
