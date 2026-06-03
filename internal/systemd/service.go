package systemd

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"servio/internal/storage"
)

// ServiceManager defines the interface for managing systemd services.
type ServiceManager interface {
	Start(ctx context.Context, serviceName string) error
	Stop(ctx context.Context, serviceName string) error
	Restart(ctx context.Context, serviceName string) error
	Enable(ctx context.Context, serviceName string) error
	Disable(ctx context.Context, serviceName string) error
	Status(ctx context.Context, serviceName string) (ServiceStatus, error)
	Reload(ctx context.Context) error
	GetStartTime(ctx context.Context, serviceName string) (string, error)
	GetLogsWithTimeRange(ctx context.Context, serviceName, since, until string) (string, error)
	StreamLogs(ctx context.Context, serviceName string) (<-chan string, error)
	GenerateServiceFile(service *storage.Service) (string, error)
	InstallService(ctx context.Context, service *storage.Service) error
	UninstallService(ctx context.Context, serviceName string) error
	ServiceExists(serviceName string) bool
	DiscoverServices(managedNames map[string]struct{}) ([]*DiscoveredService, error)
}

// ServiceStatus holds the runtime status of a systemd unit.
type ServiceStatus struct {
	Name    string `json:"name"`
	Active  bool   `json:"active"`
	Enabled bool   `json:"enabled"`
	Output  string `json:"output,omitempty"`
}

// Manager provides systemd service management.
type Manager struct{}

// NewManager creates a new systemd Manager.
func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) Start(ctx context.Context, serviceName string) error {
	return m.runSystemctl(ctx, "start", serviceName)
}

func (m *Manager) Stop(ctx context.Context, serviceName string) error {
	return m.runSystemctl(ctx, "stop", serviceName)
}

func (m *Manager) Restart(ctx context.Context, serviceName string) error {
	return m.runSystemctl(ctx, "restart", serviceName)
}

func (m *Manager) Enable(ctx context.Context, serviceName string) error {
	return m.runSystemctl(ctx, "enable", serviceName)
}

func (m *Manager) Disable(ctx context.Context, serviceName string) error {
	return m.runSystemctl(ctx, "disable", serviceName)
}

func (m *Manager) Status(ctx context.Context, serviceName string) (ServiceStatus, error) {
	status := ServiceStatus{Name: serviceName}

	activeCmd := exec.CommandContext(ctx, "systemctl", "is-active", serviceName)
	activeOut, _ := activeCmd.Output()
	status.Active = strings.TrimSpace(string(activeOut)) == "active"

	enabledCmd := exec.CommandContext(ctx, "systemctl", "is-enabled", serviceName)
	enabledOut, _ := enabledCmd.Output()
	status.Enabled = strings.TrimSpace(string(enabledOut)) == "enabled"

	statusCmd := exec.CommandContext(ctx, "systemctl", "status", serviceName, "--no-pager")
	var stdout bytes.Buffer
	statusCmd.Stdout = &stdout
	statusCmd.Run()
	status.Output = stdout.String()

	return status, nil
}

func (m *Manager) Reload(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "systemctl", "daemon-reload")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("daemon-reload failed: %s - %w", string(output), err)
	}
	return nil
}

func (m *Manager) GetStartTime(ctx context.Context, serviceName string) (string, error) {
	cmd := exec.CommandContext(ctx, "systemctl", "show", "-p", "ActiveEnterTimestamp", "--value", serviceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get start time: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func (m *Manager) DiscoverServices(managedNames map[string]struct{}) ([]*DiscoveredService, error) {
	svcs, err := DiscoverExistingServices(managedNames)
	if err != nil {
		return nil, err
	}
	// Annotate active/enabled status
	for _, svc := range svcs {
		status, _ := m.Status(context.Background(), svc.UnitName)
		svc.Active = status.Active
		svc.Enabled = status.Enabled
	}
	return svcs, nil
}

func (m *Manager) runSystemctl(ctx context.Context, action, serviceName string) error {
	cmd := exec.CommandContext(ctx, "systemctl", action, serviceName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl %s %s failed: %s - %w", action, serviceName, string(output), err)
	}
	return nil
}
