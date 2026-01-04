package blueprints

import (
	"context"

	"servio/internal/storage"
)

// BlueprintMetadata contains static information about a blueprint
// for UI rendering and discovery. This makes it easy to add new
// blueprints without modifying frontend code.
type BlueprintMetadata struct {
	Type        string   `json:"type"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
	Icon        string   `json:"icon"` // emoji or icon class
	Versions    []string `json:"versions"`
	Default     string   `json:"default_version"`
}

// BlueprintDefaults contains the default field values when a blueprint
// type is selected in the UI.
type BlueprintDefaults struct {
	Command    string `json:"command"`
	User       string `json:"user"`
	WorkingDir string `json:"working_dir"`
	Hint       string `json:"hint"`
}

// Blueprint defines the interface for managed service types.
// To add a new blueprint:
// 1. Create a new file (e.g., mongodb.go)
// 2. Implement this interface
// 3. Register it in NewRegistry()
type Blueprint interface {
	// Type returns the unique identifier (e.g., "postgres", "redis")
	Type() string

	// Metadata returns static info for UI display
	Metadata() BlueprintMetadata

	// Defaults returns field defaults for the service form
	Defaults(version string) BlueprintDefaults

	// GenerateCommand returns the ExecStart command for systemd
	GenerateCommand(service *storage.Service) string

	// GenerateEnvironment returns environment variables as KEY=VALUE\n format
	GenerateEnvironment(service *storage.Service) string

	// GenerateSystemdOverrides returns additional systemd unit directives
	GenerateSystemdOverrides(service *storage.Service) string

	// InstallDependencies installs required packages on the system
	InstallDependencies(ctx context.Context, version string) error
}

// Registry holds all registered blueprints and provides discovery
type Registry struct {
	blueprints map[string]Blueprint
}

// NewRegistry creates a new blueprint registry with all built-in blueprints.
// To add a new blueprint, simply add a Register() call here.
func NewRegistry() *Registry {
	r := &Registry{
		blueprints: make(map[string]Blueprint),
	}

	// Register built-in blueprints
	// Add new blueprints here:
	r.Register(&DjangoBlueprint{})
	// r.Register(&MongoDBBlueprint{})
	// r.Register(&NodeBlueprint{})

	return r
}

// Register adds a blueprint to the registry
func (r *Registry) Register(bp Blueprint) {
	r.blueprints[bp.Type()] = bp
}

// Get retrieves a blueprint by type
func (r *Registry) Get(serviceType string) (Blueprint, bool) {
	bp, ok := r.blueprints[serviceType]
	return bp, ok
}

// Types returns all registered blueprint type identifiers
func (r *Registry) Types() []string {
	types := make([]string, 0, len(r.blueprints))
	for t := range r.blueprints {
		types = append(types, t)
	}
	return types
}

// AllMetadata returns metadata for all registered blueprints.
// Used by the API to populate the service form dynamically.
func (r *Registry) AllMetadata() []BlueprintMetadata {
	metas := make([]BlueprintMetadata, 0, len(r.blueprints))
	for _, bp := range r.blueprints {
		metas = append(metas, bp.Metadata())
	}
	return metas
}

// GetDefaults returns the defaults for a specific blueprint and version
func (r *Registry) GetDefaults(serviceType, version string) (BlueprintDefaults, bool) {
	bp, ok := r.blueprints[serviceType]
	if !ok {
		return BlueprintDefaults{}, false
	}
	return bp.Defaults(version), true
}

// IsManaged returns true if the service type has a blueprint
func (r *Registry) IsManaged(serviceType string) bool {
	_, ok := r.blueprints[serviceType]
	return ok
}
