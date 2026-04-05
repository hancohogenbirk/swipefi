package library

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dhowden/tag"
)

type TrackMeta struct {
	Path       string
	Title      string
	Artist     string
	Album      string
	DurationMs int64
	Format     string
	AddedAt    int64
}

var audioExtensions = map[string]string{
	".flac": "flac",
	".mp3":  "mp3",
	".wav":  "wav",
	".aiff": "aiff",
	".aif":  "aiff",
	".ogg":  "ogg",
	".m4a":  "aac",
	".aac":  "aac",
	".wma":  "wma",
	".ape":  "ape",
	".dsf":  "dsf",
	".dff":  "dff",
}

func IsAudioFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	_, ok := audioExtensions[ext]
	return ok
}

func FormatFromExt(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if f, ok := audioExtensions[ext]; ok {
		return f
	}
	return ""
}

func ReadMetadata(fullPath, relPath string) (*TrackMeta, error) {
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", relPath, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", relPath, err)
	}

	meta := &TrackMeta{
		Path:    relPath,
		Format:  FormatFromExt(relPath),
		AddedAt: info.ModTime().Unix(),
	}

	// Try to read tags; not all files have them
	m, err := tag.ReadFrom(f)
	if err == nil {
		meta.Title = m.Title()
		meta.Artist = m.Artist()
		meta.Album = m.Album()
	}

	// Fall back to filename if no title tag
	if meta.Title == "" {
		base := filepath.Base(relPath)
		meta.Title = strings.TrimSuffix(base, filepath.Ext(base))
	}

	// dhowden/tag doesn't provide duration; we'd need a format-specific parser.
	// For now, duration stays 0 and can be populated later or from the renderer.
	_ = time.Now()

	return meta, nil
}
