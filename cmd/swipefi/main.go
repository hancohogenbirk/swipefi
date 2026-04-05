package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"swipefi/internal/api"
	"swipefi/internal/library"
	"swipefi/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	// Config from environment with defaults
	port := envOr("SWIPEFI_PORT", "8080")
	musicDir := envOr("SWIPEFI_MUSIC_DIR", "./music")
	dataDir := envOr("SWIPEFI_DATA_DIR", "./data")
	deleteDir := envOr("SWIPEFI_DELETE_DIR", filepath.Join(musicDir, "to_delete"))

	// Setup logging
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	slog.Info("starting swipefi",
		"port", port,
		"music_dir", musicDir,
		"data_dir", dataDir,
		"delete_dir", deleteDir,
	)

	// Ensure directories exist
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	if err := os.MkdirAll(deleteDir, 0755); err != nil {
		return fmt.Errorf("create delete dir: %w", err)
	}

	// Verify music directory exists
	if _, err := os.Stat(musicDir); os.IsNotExist(err) {
		return fmt.Errorf("music directory does not exist: %s", musicDir)
	}

	// Open database
	dbPath := filepath.Join(dataDir, "swipefi.db")
	s, err := store.New(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer s.Close()

	// Library scanner
	scanner := library.NewScanner(musicDir, s)

	// Run initial scan in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		count, err := scanner.Scan(ctx)
		if err != nil {
			slog.Error("initial scan failed", "err", err)
			return
		}
		slog.Info("initial scan done", "tracks", count)
	}()

	// API and router
	a := api.NewAPI(s, scanner)
	router := api.NewRouter(a, musicDir)

	// HTTP server
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		slog.Info("shutting down", "signal", sig)
		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		srv.Shutdown(shutdownCtx)
	}()

	slog.Info("server listening", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("server: %w", err)
	}

	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
