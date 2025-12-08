package systemd

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strconv"
)

// GetLogs retrieves recent logs for a service
func (m *Manager) GetLogs(serviceName string, lines int) (string, error) {
	if lines <= 0 {
		lines = 100
	}

	cmd := exec.Command("journalctl",
		"-u", serviceName,
		"-n", strconv.Itoa(lines),
		"--no-pager",
		"-o", "short-iso",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get logs: %w", err)
	}

	return string(output), nil
}

// StreamLogs streams logs for a service in real-time
// The returned channel will receive log lines until the context is cancelled
func (m *Manager) StreamLogs(ctx context.Context, serviceName string) (<-chan string, error) {
	cmd := exec.CommandContext(ctx, "journalctl",
		"-u", serviceName,
		"-f", // Follow mode
		"--no-pager",
		"-o", "short-iso",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start journalctl: %w", err)
	}

	logChan := make(chan string, 100)

	go func() {
		defer close(logChan)
		defer cmd.Wait()

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			case logChan <- scanner.Text():
			}
		}
	}()

	return logChan, nil
}

// GetLogsWithTimeRange retrieves logs for a service within a time range
func (m *Manager) GetLogsWithTimeRange(serviceName, since, until string) (string, error) {
	args := []string{
		"-u", serviceName,
		"--no-pager",
		"-o", "short-iso",
	}

	if since != "" {
		args = append(args, "--since", since)
	}
	if until != "" {
		args = append(args, "--until", until)
	}

	cmd := exec.Command("journalctl", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get logs: %w", err)
	}

	return string(output), nil
}
