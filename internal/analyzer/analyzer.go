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
	binPath  string
	store    *store.Store
	mu       sync.Mutex
	cancel   context.CancelFunc
	running  bool
	analyzed int
	total    int
	lastErr  string
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

type Status struct {
	Running  bool   `json:"running"`
	Analyzed int    `json:"analyzed"`
	Total    int    `json:"total"`
	Error    string `json:"error,omitempty"`
}

func (a *Analyzer) GetStatus() Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	return Status{Running: a.running, Analyzed: a.analyzed, Total: a.total, Error: a.lastErr}
}

// MarkPending sets the analyzer as running before the actual analysis starts,
// so the frontend poll doesn't stop in the gap between scan and analysis.
func (a *Analyzer) MarkPending() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.running = true
	a.analyzed = 0
	a.total = 0
	a.lastErr = ""
}

func (a *Analyzer) Cancel() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.running = false
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
}

func (a *Analyzer) setDone(errMsg string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.running = false
	a.lastErr = errMsg
}

func (a *Analyzer) Run(ctx context.Context, musicDir string) error {
	if !a.Available() {
		a.setDone("")
		return nil
	}
	if musicDir == "" {
		a.setDone("")
		return nil
	}

	// Cancel any previous run
	a.mu.Lock()
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	a.mu.Unlock()
	defer cancel()

	tracks, err := a.store.ListTracksNeedingAnalysis(runCtx, musicDir)
	if err != nil {
		if runCtx.Err() != nil {
			a.setDone("cancelled")
			return nil
		}
		a.setDone(err.Error())
		return fmt.Errorf("list tracks needing analysis: %w", err)
	}
	if len(tracks) == 0 {
		slog.Info("no tracks need transcode analysis")
		a.setDone("")
		return nil
	}

	a.mu.Lock()
	a.running = true
	a.analyzed = 0
	a.total = len(tracks)
	a.lastErr = ""
	a.mu.Unlock()

	slog.Info("starting transcode analysis", "tracks", len(tracks))

	pathToID := make(map[string]int64, len(tracks))
	for _, t := range tracks {
		absPath := filepath.Join(musicDir, filepath.FromSlash(t.Path))
		pathToID[absPath] = t.ID
	}

	cmd := exec.CommandContext(runCtx, a.binPath, "--format", "json", musicDir)
	stderr, _ := cmd.StderrPipe()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		a.setDone(fmt.Sprintf("stdout pipe: %v", err))
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		a.setDone(fmt.Sprintf("start: %v", err))
		return fmt.Errorf("start flacalyzer: %w", err)
	}

	// Drain stderr in background for diagnostics
	go func() {
		s := bufio.NewScanner(stderr)
		for s.Scan() {
			slog.Debug("flacalyzer stderr", "line", s.Text())
		}
	}()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var analyzed, skipped int
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
				skipped++
				if skipped <= 3 {
					slog.Debug("flacalyzer result path not in DB", "path", result.Path)
				}
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
		a.mu.Lock()
		a.analyzed = analyzed
		a.mu.Unlock()
	}

	waitErr := cmd.Wait()
	if waitErr != nil && runCtx.Err() == nil {
		slog.Warn("flacalyzer exited with error", "err", waitErr)
		a.setDone(fmt.Sprintf("flacalyzer: %v", waitErr))
	} else if runCtx.Err() != nil {
		a.setDone("cancelled")
	} else {
		a.setDone("")
	}

	slog.Info("transcode analysis complete", "analyzed", analyzed, "skipped", skipped, "total", len(tracks))
	return nil
}
