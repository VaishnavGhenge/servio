package http

import (
	"context"
	"net/http"
	"time"

	"servio/internal/storage"
	"servio/internal/systemd"
)

// Server represents the HTTP server
type Server struct {
	addr       string
	httpServer *http.Server
	store      *storage.Storage
	svcManager *systemd.Manager
}

// NewServer creates a new HTTP server
func NewServer(addr string, store *storage.Storage, svcManager *systemd.Manager) *Server {
	s := &Server{
		addr:       addr,
		store:      store,
		svcManager: svcManager,
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

	// API routes
	mux.HandleFunc("/api/projects", s.handleAPIProjects)
	mux.HandleFunc("/api/projects/", s.handleAPIProject)

	// Static assets
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(getStaticFS()))))
}

// Start starts the HTTP server
func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
