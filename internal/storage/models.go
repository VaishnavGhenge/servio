package storage

import "time"

// Project is a logical grouping of related services (e.g., a full web app stack).
type Project struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	Services    []*Service `json:"services,omitempty"` // populated at read time, not stored here
}

// Service is an individual managed component backed by a systemd unit.
type Service struct {
	ID           int64     `json:"id"`
	ProjectID    int64     `json:"project_id"`
	Name         string    `json:"name"`
	GitRepoURL   string    `json:"git_repo_url,omitempty"`
	BuildCommand string    `json:"build_command,omitempty"` // run before starting (e.g. npm ci, pip install -r requirements.txt)
	Command      string    `json:"command"`
	WorkingDir   string    `json:"working_dir"`
	User         string    `json:"user"`
	Environment  string    `json:"environment"` // KEY=VALUE pairs, newline-separated
	AutoRestart  bool      `json:"auto_restart"`
	SystemdRaw   string    `json:"systemd_raw,omitempty"` // expert override: raw unit file
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// Runtime fields — not stored
	Status string `json:"status,omitempty"`
}

// ServiceName returns the systemd unit name for this service.
func (s *Service) ServiceName() string {
	return "servio-" + s.Name + ".service"
}

// Deployment records a single deploy attempt for a service.
type Deployment struct {
	ID         int64      `json:"id"`
	ServiceID  int64      `json:"service_id"`
	Status     string     `json:"status"` // pending | running | success | failed
	Log        string     `json:"log"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// --- Request types ---

type CreateProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type UpdateProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type CreateServiceRequest struct {
	ProjectID    int64  `json:"project_id"`
	Name         string `json:"name"`
	GitRepoURL   string `json:"git_repo_url"`
	BuildCommand string `json:"build_command"`
	Command      string `json:"command"`
	WorkingDir   string `json:"working_dir"`
	User         string `json:"user"`
	Environment  string `json:"environment"`
	AutoRestart  bool   `json:"auto_restart"`
	SystemdRaw   string `json:"systemd_raw"`
}

type UpdateServiceRequest struct {
	Name         string `json:"name"`
	GitRepoURL   string `json:"git_repo_url"`
	BuildCommand string `json:"build_command"`
	Command      string `json:"command"`
	WorkingDir   string `json:"working_dir"`
	User         string `json:"user"`
	Environment  string `json:"environment"`
	AutoRestart  bool   `json:"auto_restart"`
	SystemdRaw   string `json:"systemd_raw"`
}
