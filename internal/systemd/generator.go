package systemd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"servio/internal/storage"
)

const serviceDir = "/etc/systemd/system"

// GenerateServiceFile creates a systemd service file from a project
func (m *Manager) GenerateServiceFile(project *storage.Project) (string, error) {
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

	restart := "no"
	if project.AutoRestart {
		restart = "on-failure"
	}

	workingDir := project.WorkingDir
	if workingDir == "" {
		workingDir = "/"
	}

	user := project.User
	if user == "" {
		user = "root"
	}

	// Build environment section
	envSection := ""
	if project.Environment != "" {
		lines := strings.Split(project.Environment, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && strings.Contains(line, "=") {
				envSection += fmt.Sprintf("Environment=\"%s\"\n", line)
			}
		}
	}

	slog.Debug("Generating service", "project", project.Name, "command", project.Command, "working_dir", workingDir)

	// Resolve executable path if it's not absolute
	// Systemd requires absolute paths for executables
	command := project.Command
	cmdParts := strings.Fields(command)
	if len(cmdParts) > 0 {
		exe := cmdParts[0]
		// Explicitly check for relative paths starting with ./ or no slash at all
		if !strings.HasPrefix(exe, "/") {
			absExe := filepath.Join(workingDir, exe)
			slog.Debug("Resolving relative path", "exe", exe, "abs_exe", absExe)
			command = strings.Replace(command, exe, absExe, 1)
		} else {
			slog.Debug("Path is already absolute", "exe", exe)
		}
	}

	content := fmt.Sprintf(template,
		project.Description,
		user,
		workingDir,
		command,
		restart,
		"servio-"+project.Name,
		envSection,
	)

	return content, nil
}

// InstallService writes the service file and reloads systemd
func (m *Manager) InstallService(ctx context.Context, project *storage.Project) error {
	content, err := m.GenerateServiceFile(project)
	if err != nil {
		return fmt.Errorf("failed to generate service file: %w", err)
	}

	// Ensure working directory exists
	workingDir := project.WorkingDir
	if workingDir != "" && workingDir != "/" {
		if _, err := os.Stat(workingDir); os.IsNotExist(err) {
			return fmt.Errorf("working directory '%s' does not exist", workingDir)
		}
	}

	servicePath := filepath.Join(serviceDir, project.ServiceName())

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
