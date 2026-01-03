package systemd

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"servio/internal/storage"
)

// ServiceManager defines the interface for managing system services
type ServiceManager interface {
	Start(serviceName string) error
	Stop(serviceName string) error
	Restart(serviceName string) error
	Enable(serviceName string) error
	Disable(serviceName string) error
	Status(serviceName string) (ServiceStatus, error)
	Reload() error
	GetStartTime(serviceName string) (string, error)
	GetLogsWithTimeRange(serviceName, since, until string) (string, error)
	StreamLogs(ctx context.Context, serviceName string) (<-chan string, error)
	InstallService(project *storage.Project) error
	UninstallService(serviceName string) error
	ServiceExists(serviceName string) bool
}

// Manager provides systemd service management and implements ServiceManager
type Manager struct{}

// NewManager creates a new systemd Manager
func NewManager() *Manager {
	return &Manager{}
}

// Start starts a systemd service
func (m *Manager) Start(serviceName string) error {
	return m.runSystemctl("start", serviceName)
}

// Stop stops a systemd service
func (m *Manager) Stop(serviceName string) error {
	return m.runSystemctl("stop", serviceName)
}

// Restart restarts a systemd service
func (m *Manager) Restart(serviceName string) error {
	return m.runSystemctl("restart", serviceName)
}

// Enable enables a systemd service to start on boot
func (m *Manager) Enable(serviceName string) error {
	return m.runSystemctl("enable", serviceName)
}

// Disable disables a systemd service from starting on boot
func (m *Manager) Disable(serviceName string) error {
	return m.runSystemctl("disable", serviceName)
}

// Status returns the status of a systemd service
func (m *Manager) Status(serviceName string) (ServiceStatus, error) {
	status := ServiceStatus{
		Name: serviceName,
	}

	// Check if active
	activeCmd := exec.Command("systemctl", "is-active", serviceName)
	activeOut, _ := activeCmd.Output()
	status.Active = strings.TrimSpace(string(activeOut)) == "active"

	// Check if enabled
	enabledCmd := exec.Command("systemctl", "is-enabled", serviceName)
	enabledOut, _ := enabledCmd.Output()
	status.Enabled = strings.TrimSpace(string(enabledOut)) == "enabled"

	// Get full status
	statusCmd := exec.Command("systemctl", "status", serviceName, "--no-pager")
	var stdout, stderr bytes.Buffer
	statusCmd.Stdout = &stdout
	statusCmd.Stderr = &stderr
	statusCmd.Run() // Ignore error, status returns non-zero for inactive services

	status.Output = stdout.String()

	return status, nil
}

// Reload reloads the systemd daemon
func (m *Manager) Reload() error {
	cmd := exec.Command("systemctl", "daemon-reload")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("daemon-reload failed: %s - %w", string(output), err)
	}
	return nil
}

// runSystemctl executes a systemctl command
func (m *Manager) runSystemctl(action, serviceName string) error {
	cmd := exec.Command("systemctl", action, serviceName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl %s %s failed: %s - %w", action, serviceName, string(output), err)
	}
	return nil
}

// GetStartTime returns the ActiveEnterTimestamp of the service
func (m *Manager) GetStartTime(serviceName string) (string, error) {
	cmd := exec.Command("systemctl", "show", "-p", "ActiveEnterTimestamp", "--value", serviceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get start time: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// ServiceStatus represents the status of a systemd service
type ServiceStatus struct {
	Name    string `json:"name"`
	Active  bool   `json:"active"`
	Enabled bool   `json:"enabled"`
	Output  string `json:"output,omitempty"`
}
