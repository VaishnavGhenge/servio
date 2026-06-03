package systemd

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// DiscoveredService represents a systemd unit found on disk that is not yet
// managed by Servio.
type DiscoveredService struct {
	UnitName    string `json:"unit_name"`    // e.g. "nginx.service"
	Description string `json:"description"`
	ExecStart   string `json:"exec_start"`
	WorkingDir  string `json:"working_dir"`
	User        string `json:"user"`
	Environment string `json:"environment"` // KEY=VALUE newline-separated
	AutoRestart bool   `json:"auto_restart"`
	Active      bool   `json:"active"`
	Enabled     bool   `json:"enabled"`
}

// DiscoverExistingServices scans /etc/systemd/system for .service files that
// are not already tracked by Servio (servio-*.service units are excluded).
func DiscoverExistingServices(managedNames map[string]struct{}) ([]*DiscoveredService, error) {
	const unitDir = "/etc/systemd/system"

	entries, err := os.ReadDir(unitDir)
	if err != nil {
		return nil, err
	}

	var result []*DiscoveredService
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".service") {
			continue
		}
		// Skip units already managed by Servio
		if strings.HasPrefix(name, "servio-") {
			continue
		}
		// Skip units whose base name is already in Servio store
		if _, ok := managedNames[name]; ok {
			continue
		}

		svc, err := parseUnitFile(filepath.Join(unitDir, name), name)
		if err != nil {
			continue
		}
		result = append(result, svc)
	}
	return result, nil
}

func parseUnitFile(path, unitName string) (*DiscoveredService, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	svc := &DiscoveredService{UnitName: unitName}
	var envLines []string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "Description":
			svc.Description = strings.TrimSpace(val)
		case "ExecStart":
			svc.ExecStart = strings.TrimSpace(val)
		case "WorkingDirectory":
			svc.WorkingDir = strings.TrimSpace(val)
		case "User":
			svc.User = strings.TrimSpace(val)
		case "Environment":
			// Environment=KEY=VALUE (the value itself may contain =)
			env := strings.TrimSpace(val)
			env = strings.Trim(env, `"`)
			if env != "" {
				envLines = append(envLines, env)
			}
		case "Restart":
			v := strings.TrimSpace(val)
			svc.AutoRestart = v != "" && v != "no"
		}
	}
	svc.Environment = strings.Join(envLines, "\n")
	return svc, scanner.Err()
}
