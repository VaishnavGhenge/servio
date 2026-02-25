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

// GenerateServiceFile produces a systemd unit file for the given service.
// If SystemdRaw is set it is returned as-is (expert override).
func (m *Manager) GenerateServiceFile(service *storage.Service) (string, error) {
	if service.SystemdRaw != "" {
		slog.Info("Using raw systemd override", "service", service.Name)
		return service.SystemdRaw, nil
	}

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

	command := service.Command

	// Resolve relative executable paths against the working directory.
	cmdParts := strings.Fields(command)
	if len(cmdParts) > 0 {
		exe := cmdParts[0]
		if !strings.HasPrefix(exe, "/") {
			abs := filepath.Join(workingDir, exe)
			command = strings.Replace(command, exe, abs, 1)
		}
	}

	// Build Environment= lines.
	envSection := ""
	for _, line := range strings.Split(service.Environment, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && strings.Contains(line, "=") {
			envSection += fmt.Sprintf("Environment=\"%s\"\n", line)
		}
	}

	content := fmt.Sprintf(`[Unit]
Description=Managed Service: %s
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
`,
		service.Name,
		user,
		workingDir,
		command,
		restart,
		"servio-"+service.Name,
		envSection,
	)

	return content, nil
}

// InstallService writes the unit file and reloads systemd.
func (m *Manager) InstallService(ctx context.Context, service *storage.Service) error {
	slog.Info("Installing service", "service", service.Name, "user", service.User)

	// Verify the system user exists (skip for root).
	if service.User != "" && service.User != "root" {
		if err := exec.Command("id", "-u", service.User).Run(); err != nil {
			return fmt.Errorf("system user '%s' does not exist", service.User)
		}
	}

	// Verify the executable exists if an absolute path is given.
	cmdParts := strings.Fields(service.Command)
	if len(cmdParts) > 0 && strings.HasPrefix(cmdParts[0], "/") {
		if _, err := os.Stat(cmdParts[0]); os.IsNotExist(err) {
			return fmt.Errorf("executable '%s' not found", cmdParts[0])
		}
	}

	content, err := m.GenerateServiceFile(service)
	if err != nil {
		return fmt.Errorf("failed to generate service file: %w", err)
	}

	// Ensure working directory exists.
	if service.WorkingDir != "" && service.WorkingDir != "/" {
		if err := os.MkdirAll(service.WorkingDir, 0755); err != nil {
			return fmt.Errorf("failed to create working directory: %w", err)
		}
		if service.User != "" && service.User != "root" {
			exec.Command("chown", service.User, service.WorkingDir).Run()
		}
	}

	servicePath := filepath.Join(serviceDir, service.ServiceName())
	if err := os.WriteFile(servicePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	return m.Reload(ctx)
}

// UninstallService stops, disables, and removes the unit file.
func (m *Manager) UninstallService(ctx context.Context, serviceName string) error {
	m.Stop(ctx, serviceName)
	m.Disable(ctx, serviceName)

	servicePath := filepath.Join(serviceDir, serviceName)
	if err := os.Remove(servicePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove service file: %w", err)
	}

	return m.Reload(ctx)
}

// ServiceExists reports whether the unit file is present.
func (m *Manager) ServiceExists(serviceName string) bool {
	_, err := os.Stat(filepath.Join(serviceDir, serviceName))
	return err == nil
}
