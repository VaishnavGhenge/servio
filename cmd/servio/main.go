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
	"servio/internal/deploy"
	httpserver "servio/internal/http"
	"servio/internal/storage"
	"servio/internal/systemd"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	setupLogger(cfg.LogLevel)
	slog.Info("Starting Servio", "version", "2.0.0")

	store, err := storage.New(cfg.DBPath)
	if err != nil {
		slog.Error("Failed to initialize storage", "error", err, "path", cfg.DBPath)
		os.Exit(1)
	}
	defer store.Close()

	svcManager := systemd.NewManager()
	deployMgr := deploy.NewManager(store, svcManager)
	server := httpserver.NewServer(cfg.Addr, store, svcManager, deployMgr)

	go func() {
		slog.Info("Server starting", "addr", cfg.Addr)
		if err := server.Start(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down...")
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
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slogLevel})
	slog.SetDefault(slog.New(handler))
}
