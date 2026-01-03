package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"servio/internal/config"
	httpserver "servio/internal/http"
	"servio/internal/storage"
	"servio/internal/systemd"
)

func main() {
	// Initialize configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Initialize structured logger
	setupLogger(cfg.LogLevel)

	slog.Info("Starting Servio", "version", "1.0.0")

	// Initialize storage
	store, err := storage.New(cfg.DBPath)
	if err != nil {
		slog.Error("Failed to initialize storage", "error", err, "path", cfg.DBPath)
		os.Exit(1)
	}
	defer store.Close()

	// Initialize systemd service manager
	svcManager := systemd.NewManager()

	// Initialize HTTP server
	server := httpserver.NewServer(cfg.Addr, store, svcManager)

	// Start server in goroutine
	go func() {
		slog.Info("🚀 Server starting", "addr", cfg.Addr)
		if err := server.Start(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("Server shutdown error", "error", err)
	}

	slog.Info("Server stopped")
}

func setupLogger(level string) {
	var slogLevel slog.Level
	switch level {
	case "debug":
		slogLevel = slog.LevelDebug
	case "info":
		slogLevel = slog.LevelInfo
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slogLevel})
	logger := slog.New(handler)
	slog.SetDefault(logger)
}
