package main

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"swipefi/internal/api"
	"swipefi/internal/dlna"
	"swipefi/internal/library"
	"swipefi/internal/player"
	"swipefi/internal/store"
	"swipefi/web"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

const (
	defaultPort        = "8080"
	defaultDataDir     = "./data"
	dlnaDiscoveryDelay = 2 * time.Second
	shutdownTimeout    = 5 * time.Second
)

func run() error {
	port := envOr("SWIPEFI_PORT", defaultPort)
	dataDir := envOr("SWIPEFI_DATA_DIR", defaultDataDir)

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	// Open database
	dbPath := filepath.Join(dataDir, "swipefi.db")
	s, err := store.New(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer s.Close()

	// Determine music dir: env var overrides DB config
	musicDir := os.Getenv("SWIPEFI_MUSIC_DIR")
	if musicDir == "" {
		musicDir, _ = s.GetConfig(store.ConfigKeyMusicDir)
	}
	var deleteDir string
	if musicDir != "" {
		deleteDir = library.DeleteDir(musicDir)
	}

	slog.Info("starting swipefi",
		"port", port,
		"music_dir", musicDir,
		"data_dir", dataDir,
		"delete_dir", deleteDir,
	)

	// Library scanner (music dir may be empty on first run)
	scanner := library.NewScanner(musicDir, s)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Player
	p, err := player.New(ctx, s, musicDir, deleteDir, port)
	if err != nil {
		return fmt.Errorf("create player: %w", err)
	}

	// DLNA discovery
	discovery := dlna.NewDiscovery()

	// WebSocket hub
	hub := api.NewHub()

	p.SetOnChange(func(state player.PlayerState) {
		hub.Broadcast(state)
	})

	// API and router
	a := api.NewAPI(s, scanner, p, discovery, hub, dataDir)

	// Handle music dir changes from the settings UI
	a.SetOnMusicDirChanged(func(newMusicDir, newDeleteDir string) {
		slog.Info("music directory changed", "path", newMusicDir)
		scanner.SetMusicDir(newMusicDir)
		p.SetDirs(newMusicDir, newDeleteDir)
		os.MkdirAll(newDeleteDir, 0755)

		// Trigger a rescan in background (play counts preserved via UpsertTrack)
		// purgeOrphans=true: hard-delete old dir tracks instead of soft-deleting
		scanner.MarkScanning()
		go func() {
			count, err := scanner.Scan(ctx, false, true)
			if err != nil {
				slog.Error("rescan after config change", "err", err)
				return
			}
			slog.Info("rescan complete", "tracks", count)
		}()
	})

	// Embedded frontend (built into the binary)
	var frontendFS fs.FS
	distFS, err := fs.Sub(web.DistFS, "dist")
	if err == nil {
		// Check if the dist directory has content
		if entries, err := fs.ReadDir(distFS, "."); err == nil && len(entries) > 0 {
			frontendFS = distFS
			slog.Info("serving embedded frontend")
		}
	}

	router := api.NewRouter(a, frontendFS)

	// If we already have a music dir, scan on startup
	if musicDir != "" {
		os.MkdirAll(deleteDir, 0755)
		go func() {
			count, err := scanner.Scan(ctx, false)
			if err != nil {
				slog.Error("initial scan failed", "err", err)
				return
			}
			slog.Info("initial scan done", "tracks", count)
		}()
	}

	// DLNA discovery + auto-reconnect to last device
	go func() {
		time.Sleep(dlnaDiscoveryDelay)
		if err := discovery.Scan(ctx); err != nil {
			slog.Error("initial discovery failed", "err", err)
			return
		}

		// Try auto-reconnect to last selected device
		savedUDN, _ := s.GetConfig(store.ConfigKeyDeviceUDN)
		if savedUDN != "" {
			if renderer, ok := discovery.GetRenderer(savedUDN); ok {
				transport := dlna.NewTransport(renderer.Transport)
				p.SetTransport(transport)
				slog.Info("auto-reconnected to device", "name", renderer.Name, "udn", savedUDN)
			} else {
				slog.Info("saved device not found, manual selection required", "udn", savedUDN)
			}
		}
	}()

	// HTTP server
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		slog.Info("shutting down", "signal", sig)
		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
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
