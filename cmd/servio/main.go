package main

import (
"context"
"flag"
"log"
"net/http"
"os"
"os/signal"
"syscall"
"time"

httpserver "servio/internal/http"
"servio/internal/storage"
"servio/internal/systemd"

"github.com/joho/godotenv"
)

func main() {
	// Load environment variables from .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	// Parse flags
	addr := flag.String("addr", ":8080", "HTTP server address")
	dbPath := flag.String("db", "servio.db", "SQLite database path")
	flag.Parse()

	// Initialize storage
	store, err := storage.New(*dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()

	// Initialize systemd service manager
	svcManager := systemd.NewManager()

	// Initialize HTTP server
	server := httpserver.NewServer(*addr, store, svcManager)

	// Start server in goroutine
	go func() {
		log.Printf("🚀 Servio starting on http://localhost%s", *addr)
		if err := server.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}
