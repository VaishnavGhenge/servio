package http

import (
	"context"
	"net/http"
	"time"

	"servio/internal/blueprints"
	"servio/internal/nginx"
	"servio/internal/storage"
	"servio/internal/systemd"
)

// Server represents the HTTP server
type Server struct {
	addr         string
	httpServer   *http.Server
	store        storage.Store
	svcManager   systemd.ServiceManager
	blueprints   *blueprints.Registry
	nginxManager *nginx.Manager
}

// NewServer creates a new HTTP server
func NewServer(addr string, store storage.Store, svcManager systemd.ServiceManager) *Server {
	s := &Server{
		addr:         addr,
		store:        store,
		svcManager:   svcManager,
		blueprints:   blueprints.NewRegistry(),
		nginxManager: nginx.NewManager(),
	}

	// Initial distro configuration from settings
	if distro, err := store.GetSetting(context.Background(), "distro"); err == nil && distro != "" {
		s.nginxManager.Configure(distro)
	}

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      BasicAuth(Logger(CORS(mux))),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

// registerRoutes sets up all routes
func (s *Server) registerRoutes(mux *http.ServeMux) {
	// UI routes
	mux.HandleFunc("/", s.handleDashboard)
	mux.HandleFunc("/projects/new", s.handleNewProject)
	mux.HandleFunc("/projects/", s.handleProjectDetail)
	mux.HandleFunc("/services/new", s.handleNewService)
	mux.HandleFunc("/services/", s.handleServiceDetail)

	// API routes
	mux.HandleFunc("/api/projects", s.handleAPIProjects)
	mux.HandleFunc("/api/projects/", s.handleAPIProject)
	mux.HandleFunc("/api/services", s.handleAPIServices)
	mux.HandleFunc("/api/services/", s.handleAPIService)
	mux.HandleFunc("/api/stats", s.handleAPIStats)
	mux.HandleFunc("/api/blueprints", s.handleAPIBlueprints)
	mux.HandleFunc("/api/nginx/", s.handleAPINginx)
	mux.HandleFunc("/api/settings/", s.handleAPISettings)

	// Static assets with no-cache headers
	staticHandler := http.StripPrefix("/static/", http.FileServer(http.FS(getStaticFS())))
	mux.Handle("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		staticHandler.ServeHTTP(w, r)
	}))
}

// Start starts the HTTP server
func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
