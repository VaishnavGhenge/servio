package storage

import "time"

// Project represents a group of related services (e.g., an entire web application stack)
type Project struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Domain      string    `json:"domain,omitempty"`    // e.g., "myapp.com" for Nginx site config
	NginxRaw    string    `json:"nginx_raw,omitempty"` // Raw Nginx site config override
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Services belonging to this project
	Services []*Service `json:"services,omitempty"`
}

// Service represents an individual managed component (e.g., a database or a backend)
type Service struct {
	ID          int64     `json:"id"`
	ProjectID   int64     `json:"project_id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"` // e.g., django, postgres, redis, custom
	Version     string    `json:"version,omitempty"`
	Port        int       `json:"port,omitempty"`         // Port the service listens on (for Nginx proxy)
	GitRepoURL  string    `json:"git_repo_url,omitempty"` // Git repository URL for cloning
	Command     string    `json:"command"`
	WorkingDir  string    `json:"working_dir"`
	User        string    `json:"user"`
	Environment string    `json:"environment"` // KEY=VALUE pairs, newline separated
	AutoRestart bool      `json:"auto_restart"`
	Config      string    `json:"config,omitempty"` // JSON configuration overrides
	SystemdRaw  string    `json:"systemd_raw,omitempty"`
	NginxRaw    string    `json:"nginx_raw,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Runtime status (not stored in DB)
	Status string `json:"status,omitempty"`
}

// ServiceName returns the systemd service name for this service
func (s *Service) ServiceName() string {
	return "servio-" + s.Name + ".service"
}

// CreateProjectRequest represents the request body for creating a project
type CreateProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Domain      string `json:"domain"`
}

// CreateServiceRequest represents the request body for adding a service to a project
type CreateServiceRequest struct {
	ProjectID   int64  `json:"project_id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Version     string `json:"version"`
	Port        int    `json:"port"`
	GitRepoURL  string `json:"git_repo_url"`
	Command     string `json:"command"`
	WorkingDir  string `json:"working_dir"`
	User        string `json:"user"`
	Environment string `json:"environment"`
	AutoRestart bool   `json:"auto_restart"`
	Config      string `json:"config"`
	SystemdRaw  string `json:"systemd_raw"`
	NginxRaw    string `json:"nginx_raw"`
}

// UpdateProjectRequest represents the request body for updating a project
type UpdateProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Domain      string `json:"domain"`
}

// UpdateServiceRequest represents the request body for updating a service
type UpdateServiceRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Port        int    `json:"port"`
	GitRepoURL  string `json:"git_repo_url"`
	Command     string `json:"command"`
	WorkingDir  string `json:"working_dir"`
	User        string `json:"user"`
	Environment string `json:"environment"`
	AutoRestart bool   `json:"auto_restart"`
	Config      string `json:"config"`
	SystemdRaw  string `json:"systemd_raw"`
	NginxRaw    string `json:"nginx_raw"`
}
