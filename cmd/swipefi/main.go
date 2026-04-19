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
	"sync/atomic"
	"syscall"
	"time"

	"swipefi/internal/analyzer"
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
	s.SetMusicDir(musicDir)
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

	// Transcode analyzer (gracefully disabled if flacalyzer binary not found)
	az := analyzer.New(s)

	if musicDir != "" {
		if err := s.BackfillMusicDir(musicDir); err != nil {
			slog.Warn("backfill music_dir failed", "err", err)
		}
	}

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
	hub := api.NewHub(nil)

	p.SetOnChange(func(state player.PlayerState) {
		hub.Broadcast(state)
	})

	var reconnectRunning atomic.Bool

	p.SetOnDisconnect(func() {
		if !reconnectRunning.CompareAndSwap(false, true) {
			slog.Info("reconnect loop already running, skipping")
			return
		}
		defer reconnectRunning.Store(false)

		savedUDN, _ := s.GetConfig(store.ConfigKeyDeviceUDN)
		if savedUDN == "" {
			p.ClearReconnecting()
			return
		}
		backoffs := []time.Duration{
			2 * time.Second, 5 * time.Second, 10 * time.Second,
			20 * time.Second, 30 * time.Second, 60 * time.Second,
		}
		for _, delay := range backoffs {
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			slog.Info("attempting auto-reconnect", "delay", delay)
			if err := discovery.Scan(ctx); err != nil {
				slog.Debug("reconnect scan failed", "err", err)
				continue
			}
			renderer, ok := discovery.GetRenderer(savedUDN)
			if !ok {
				slog.Debug("reconnect: device not found in scan")
				continue
			}
			transport := dlna.NewTransport(renderer.Transport)
			if _, err := transport.GetState(ctx); err != nil {
				slog.Debug("reconnect: device not responding", "err", err)
				continue
			}
			p.SetTransport(transport)
			slog.Info("auto-reconnected to device", "name", renderer.Name)
			return
		}
		slog.Warn("auto-reconnect failed after all retries")
		p.ClearReconnecting()
	})

	// When the analyzer finishes a track, refresh the player's in-memory state
	// so the FAKE stamp appears immediately without restarting playback.
	az.OnTrackAnalyzed = func(trackID int64) {
		p.RefreshTrack(ctx, trackID)
	}

	// API and router
	a := api.NewAPI(s, scanner, p, discovery, az, hub, dataDir)

	// Handle music dir changes from the settings UI
	a.SetOnMusicDirChanged(func(newMusicDir, newDeleteDir string) {
		slog.Info("music directory changed", "path", newMusicDir)
		s.SetMusicDir(newMusicDir)
		scanner.SetMusicDir(newMusicDir)
		p.SetDirs(newMusicDir, newDeleteDir)
		os.MkdirAll(newDeleteDir, 0755)
		az.Cancel()

		// Trigger a rescan in background (music_dir scoping handles old tracks)
		scanner.MarkScanning()
		go func() {
			count, err := scanner.Scan(ctx, false)
			if err != nil {
				slog.Error("rescan after config change", "err", err)
				return
			}
			slog.Info("rescan complete", "tracks", count)

			enabled, _ := s.GetConfig(store.ConfigKeyFlacalyzerEnabled)
			if enabled == "true" && az.Available() {
				if err := az.Run(ctx, newMusicDir); err != nil {
					slog.Error("transcode analysis after dir change", "err", err)
				}
			}
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

			enabled, _ := s.GetConfig(store.ConfigKeyFlacalyzerEnabled)
			if enabled == "true" && az.Available() {
				if err := az.Run(ctx, musicDir); err != nil {
					slog.Error("transcode analysis failed", "err", err)
				}
			}
		}()

		// Backfill audio format info for existing FLAC tracks that don't have it yet
		go func() {
			// Wait for initial scan to complete
			time.Sleep(5 * time.Second)
			tracks, err := s.ListTracksNeedingAudioInfo(ctx, musicDir)
			if err != nil {
				slog.Warn("list tracks for audio info backfill", "err", err)
				return
			}
			if len(tracks) == 0 {
				return
			}
			slog.Info("backfilling audio info", "tracks", len(tracks))
			for _, t := range tracks {
				if ctx.Err() != nil {
					return
				}
				fullPath := filepath.Join(musicDir, filepath.FromSlash(t.Path))
				info, err := library.ReadFLACStreamInfo(fullPath)
				if err != nil {
					continue
				}
				bitrateKbps := 0
				if fi, statErr := os.Stat(fullPath); statErr == nil && info.TotalSamples > 0 && info.SampleRate > 0 {
					durationSec := float64(info.TotalSamples) / float64(info.SampleRate)
					bitrateKbps = int(float64(fi.Size()) * 8 / durationSec / 1000)
				}
				if err := s.UpdateTrackAudioInfo(ctx, t.ID, info.SampleRate, info.BitDepth, bitrateKbps); err != nil {
					slog.Warn("update audio info", "id", t.ID, "err", err)
				}
			}
			slog.Info("audio info backfill complete")
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
				if _, err := transport.GetState(ctx); err != nil {
					slog.Warn("saved device not responding, skipping auto-connect", "name", renderer.Name, "err", err)
				} else {
					p.SetTransport(transport)
					slog.Info("auto-reconnected to device", "name", renderer.Name, "udn", savedUDN)
				}
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
