package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Store defines the persistence interface.
type Store interface {
	// Projects
	CreateProject(ctx context.Context, req *CreateProjectRequest) (*Project, error)
	GetProject(ctx context.Context, id int64) (*Project, error)
	ListProjects(ctx context.Context) ([]*Project, error)
	UpdateProject(ctx context.Context, id int64, req *UpdateProjectRequest) (*Project, error)
	DeleteProject(ctx context.Context, id int64) error

	// Services
	CreateService(ctx context.Context, req *CreateServiceRequest) (*Service, error)
	GetService(ctx context.Context, id int64) (*Service, error)
	ListServicesByProject(ctx context.Context, projectID int64) ([]*Service, error)
	UpdateService(ctx context.Context, id int64, req *UpdateServiceRequest) (*Service, error)
	DeleteService(ctx context.Context, id int64) error

	// Deployments
	CreateDeployment(ctx context.Context, serviceID int64) (*Deployment, error)
	GetDeployment(ctx context.Context, id int64) (*Deployment, error)
	ListDeploymentsByService(ctx context.Context, serviceID int64) ([]*Deployment, error)
	UpdateDeploymentStatus(ctx context.Context, id int64, status, log string, finishedAt *time.Time) error

	Close() error
}

// dbData is the on-disk JSON structure.
type dbData struct {
	Projects    []*Project    `json:"projects"`
	Services    []*Service    `json:"services"`
	Deployments []*Deployment `json:"deployments"`
	NextID      int64         `json:"next_id"`
}

// Storage is the JSON-file backed implementation of Store.
type Storage struct {
	mu   sync.RWMutex
	path string
	data dbData
}

// New opens (or creates) the JSON storage file.
func New(path string) (*Storage, error) {
	s := &Storage{path: path}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Storage) Close() error { return nil }

func (s *Storage) load() error {
	raw, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		s.data = dbData{NextID: 1}
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read storage file: %w", err)
	}
	return json.Unmarshal(raw, &s.data)
}

// save atomically writes data to disk (write temp → rename).
func (s *Storage) save() error {
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	dir := filepath.Dir(s.path)
	if dir == "" {
		dir = "."
	}
	tmp, err := os.CreateTemp(dir, "servio-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}
	return nil
}

func (s *Storage) nextID() int64 {
	id := s.data.NextID
	s.data.NextID++
	return id
}
