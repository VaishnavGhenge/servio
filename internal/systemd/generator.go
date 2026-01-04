package systemd

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

const serviceDir = "/etc/systemd/system"

// GenerateServiceFile creates a systemd service file from a service entity
func (m *Manager) GenerateServiceFile(service *storage.Service) (string, error) {
	if service.SystemdRaw != "" {
		slog.Info("Using raw systemd override", "service", service.Name)
		return service.SystemdRaw, nil
	}

	// Check if this service type has a blueprint
	var hasBlueprint bool
	command := service.Command
	environment := service.Environment
	systemdOverrides := ""

	if m.blueprints != nil && service.Type != "" {
		if bpInterface, ok := m.blueprints.Get(service.Type); ok {
			hasBlueprint = true
			slog.Info("Using blueprint for service generation", "service", service.Name, "type", service.Type)

			// Type assert to access blueprint methods
			type blueprintMethods interface {
				GenerateCommand(service *storage.Service) string
				GenerateEnvironment(service *storage.Service) string
				GenerateSystemdOverrides(service *storage.Service) string
			}

			if bp, ok := bpInterface.(blueprintMethods); ok {
				// Use blueprint-generated command if service command is empty
				if command == "" {
					command = bp.GenerateCommand(service)
				}

				// Merge blueprint environment with service environment
				blueprintEnv := bp.GenerateEnvironment(service)
				if blueprintEnv != "" {
					if environment != "" {
						environment = blueprintEnv + "\n" + environment
					} else {
						environment = blueprintEnv
					}
				}

				// Get blueprint-specific systemd overrides
				systemdOverrides = bp.GenerateSystemdOverrides(service)
			}
		}
	}

	// Basic defaults
	restart := "no"
	if service.AutoRestart {
		restart = "on-failure"
	}

	workingDir := service.WorkingDir
	if workingDir == "" {
		workingDir = "/"
	}

	user := service.User
	if user == "" {
		user = "root"
	}

	// Build environment section
	envSection := ""
	if environment != "" {
		lines := strings.Split(environment, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && strings.Contains(line, "=") {
				envSection += fmt.Sprintf("Environment=\"%s\"\n", line)
			}
		}
	}

	slog.Debug("Generating service", "service", service.Name, "command", command, "working_dir", workingDir, "has_blueprint", hasBlueprint)

	// Resolve executable path if it's not absolute
	// Only for non-blueprint services or when command doesn't start with /
	cmdParts := strings.Fields(command)
	if len(cmdParts) > 0 {
		exe := cmdParts[0]
		if !strings.HasPrefix(exe, "/") && !hasBlueprint {
			absExe := filepath.Join(workingDir, exe)
			slog.Debug("Resolving relative path", "exe", exe, "abs_exe", absExe)
			command = strings.Replace(command, exe, absExe, 1)
		}
	}

	// Build the systemd unit file
	// If we have blueprint overrides, use them; otherwise use simple template
	if systemdOverrides != "" {
		// Parse overrides and merge with base config
		content := fmt.Sprintf(`[Unit]
Description=Managed Service: %s
After=network.target

%s
ExecStart=%s
Restart=%s
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=%s
%s

[Install]
WantedBy=multi-user.target
`,
			service.Name,
			systemdOverrides,
			command,
			restart,
			"servio-"+service.Name,
			envSection,
		)
		return content, nil
	}

	// Standard template for non-blueprint services
	template := `[Unit]
Description=%s
After=network.target

[Service]
Type=simple
User=%s
WorkingDirectory=%s
ExecStart=%s
Restart=%s
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=%s
%s

[Install]
WantedBy=multi-user.target
`

	content := fmt.Sprintf(template,
		"Managed Service: "+service.Name,
		user,
		workingDir,
		command,
		restart,
		"servio-"+service.Name,
		envSection,
	)

	return content, nil
}

// InstallService writes the service file and reloads systemd
func (m *Manager) InstallService(ctx context.Context, service *storage.Service) error {
	slog.Info("InstallService called", "service", service.Name, "user", service.User, "command", service.Command)

	// Pre-flight check: Verify User exists
	if service.User != "" && service.User != "root" {
		slog.Info("Checking if user exists", "user", service.User)
		cmd := exec.Command("id", "-u", service.User)
		if err := cmd.Run(); err != nil {
			slog.Error("User check failed", "user", service.User, "error", err)
			return fmt.Errorf("system user '%s' does not exist; please install the corresponding package (e.g. postgresql-server) or change the service user", service.User)
		}
		slog.Info("User exists", "user", service.User)
	}

	// Pre-flight check: Verify Executable exists
	cmdParts := strings.Fields(service.Command)
	if len(cmdParts) > 0 {
		exe := cmdParts[0]
		// If absolute path
		if strings.HasPrefix(exe, "/") {
			slog.Info("Checking if executable exists", "exe", exe)
			if _, err := os.Stat(exe); os.IsNotExist(err) {
				slog.Error("Executable not found", "exe", exe)
				return fmt.Errorf("executable '%s' not found on server", exe)
			}
			slog.Info("Executable exists", "exe", exe)
		} else {
			// If relative/command name, try looking it up (basic check)
			if _, err := exec.LookPath(exe); err != nil {
				// Don't fail strictly on LookPath as it might be in a path we don't know,
				// but warn or rely on previous behavior. For now, strict for absolute is safer.
			}
		}
	}

	content, err := m.GenerateServiceFile(service)
	if err != nil {
		return fmt.Errorf("failed to generate service file: %w", err)
	}

	// Ensure working directory exists (and create it if needed)
	workingDir := service.WorkingDir
	if workingDir != "" && workingDir != "/" {
		if err := os.MkdirAll(workingDir, 0755); err != nil {
			return fmt.Errorf("failed to create working directory '%s': %w", workingDir, err)
		}

		// Set ownership if a user is specified
		if service.User != "" && service.User != "root" {
			// Best effort chown
			cmd := exec.Command("chown", service.User, workingDir)
			if output, err := cmd.CombinedOutput(); err != nil {
				slog.Warn("Failed to chown working directory", "dir", workingDir, "user", service.User, "error", err, "output", string(output))
			}
		}
	}

	servicePath := filepath.Join(serviceDir, service.ServiceName())

	if err := os.WriteFile(servicePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	if err := m.Reload(ctx); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	return nil
}

// UninstallService removes the service file and reloads systemd
func (m *Manager) UninstallService(ctx context.Context, serviceName string) error {
	// Stop the service first
	m.Stop(ctx, serviceName)
	m.Disable(ctx, serviceName)

	servicePath := filepath.Join(serviceDir, serviceName)

	if err := os.Remove(servicePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove service file: %w", err)
	}

	return m.Reload(ctx)
}

// ServiceExists checks if a service file exists
func (m *Manager) ServiceExists(serviceName string) bool {
	servicePath := filepath.Join(serviceDir, serviceName)
	_, err := os.Stat(servicePath)
	return err == nil
}
