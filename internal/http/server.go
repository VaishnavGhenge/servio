package http

import (
	"context"
	"net/http"
	"time"

	"servio/internal/deploy"
	"servio/internal/storage"
	"servio/internal/systemd"
)

// Server is the HTTP server.
type Server struct {
	addr         string
	httpServer   *http.Server
	store        storage.Store
	svcManager   systemd.ServiceManager
	deployMgr    *deploy.Manager
}

// NewServer creates and configures the HTTP server.
func NewServer(addr string, store storage.Store, svcManager systemd.ServiceManager, deployMgr *deploy.Manager) *Server {
	s := &Server{
		addr:       addr,
		store:      store,
		svcManager: svcManager,
		deployMgr:  deployMgr,
	}

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: BasicAuth(Logger(CORS(mux))),
		// Note: WriteTimeout is intentionally omitted for SSE streaming endpoints.
		// Individual handlers use request context cancellation instead.
		ReadTimeout: 15 * time.Second,
		IdleTimeout: 60 * time.Second,
	}

	return s
}

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
	mux.HandleFunc("/api/deployments/", s.handleAPIDeployment)

	// Static assets
	staticHandler := http.StripPrefix("/static/", http.FileServer(http.FS(getStaticFS())))
	mux.Handle("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		staticHandler.ServeHTTP(w, r)
	}))
}

func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
