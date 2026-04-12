package analyzer

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"swipefi/internal/store"
)

type Analyzer struct {
	binPath string
	store   *store.Store
	mu      sync.Mutex
	cancel  context.CancelFunc
}

type flacResult struct {
	Type        string  `json:"type"`
	Path        string  `json:"path"`
	Verdict     string  `json:"verdict"`
	Confidence  float64 `json:"confidence"`
	SourceCodec *string `json:"source_codec"`
}

func New(s *store.Store) *Analyzer {
	binPath, err := exec.LookPath("flacalyzer")
	if err != nil {
		slog.Info("flacalyzer not found, transcode detection disabled")
		return &Analyzer{store: s}
	}
	slog.Info("flacalyzer found", "path", binPath)
	return &Analyzer{binPath: binPath, store: s}
}

func (a *Analyzer) Available() bool {
	return a.binPath != ""
}

func (a *Analyzer) Cancel() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
}

func (a *Analyzer) Run(ctx context.Context, musicDir string) error {
	if !a.Available() {
		return nil
	}
	if musicDir == "" {
		return nil
	}

	a.Cancel()

	a.mu.Lock()
	runCtx, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	a.mu.Unlock()
	defer cancel()

	tracks, err := a.store.ListTracksNeedingAnalysis(runCtx, musicDir)
	if err != nil {
		if runCtx.Err() != nil {
			return nil
		}
		return fmt.Errorf("list tracks needing analysis: %w", err)
	}
	if len(tracks) == 0 {
		return nil
	}

	slog.Info("starting transcode analysis", "tracks", len(tracks))

	pathToID := make(map[string]int64, len(tracks))
	for _, t := range tracks {
		absPath := filepath.Join(musicDir, filepath.FromSlash(t.Path))
		pathToID[absPath] = t.ID
	}

	cmd := exec.CommandContext(runCtx, a.binPath, "--format", "json", musicDir)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start flacalyzer: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var analyzed int
	for scanner.Scan() {
		if runCtx.Err() != nil {
			break
		}

		line := scanner.Bytes()
		var result flacResult
		if err := json.Unmarshal(line, &result); err != nil {
			continue
		}

		if result.Type != "file" {
			continue
		}

		trackID, ok := pathToID[result.Path]
		if !ok {
			cleanPath := filepath.Clean(result.Path)
			trackID, ok = pathToID[cleanPath]
			if !ok {
				continue
			}
		}

		source := ""
		if result.SourceCodec != nil {
			source = *result.SourceCodec
		}

		score := result.Confidence
		if strings.Contains(result.Verdict, "lossless") {
			score = 0
		}

		if err := a.store.UpdateTranscodeAnalysis(runCtx, trackID, score, source); err != nil {
			slog.Warn("update transcode analysis", "id", trackID, "err", err)
			continue
		}
		analyzed++
	}

	if err := cmd.Wait(); err != nil && runCtx.Err() == nil {
		slog.Warn("flacalyzer exited with error", "err", err)
	}

	slog.Info("transcode analysis complete", "analyzed", analyzed, "total", len(tracks))
	return nil
}
