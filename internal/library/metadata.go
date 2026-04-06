package library

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dhowden/tag"
)

// ArtData holds extracted album art.
type ArtData struct {
	Data     []byte
	MimeType string // "image/jpeg", "image/png", etc.
}

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

	// Fall back to path structure for missing metadata
	// e.g. "Artist/Album/01 - Title.flac" → derive artist, album, title
	parts := strings.Split(filepath.ToSlash(filepath.Dir(relPath)), "/")

	if meta.Title == "" {
		base := filepath.Base(relPath)
		meta.Title = cleanTrackTitle(strings.TrimSuffix(base, filepath.Ext(base)))
	}

	if meta.Album == "" && len(parts) >= 1 && parts[len(parts)-1] != "." {
		meta.Album = parts[len(parts)-1]
	}

	if meta.Artist == "" && len(parts) >= 2 {
		meta.Artist = parts[len(parts)-2]
	}

	return meta, nil
}

// cleanTrackTitle strips common prefixes like "01 - ", "01. ", "1-" from filenames.
func cleanTrackTitle(name string) string {
	// Strip leading track numbers: "01 - Title", "01. Title", "1-Title", "01 Title"
	i := 0
	for i < len(name) && name[i] >= '0' && name[i] <= '9' {
		i++
	}
	if i > 0 && i < len(name) {
		rest := name[i:]
		rest = strings.TrimLeft(rest, " .-_")
		if rest != "" {
			return rest
		}
	}
	return name
}

// ExtractArt reads embedded album art from an audio file.
// Returns nil if no art is embedded.
func ExtractArt(fullPath string) (*ArtData, error) {
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil {
		return nil, nil // no tags = no art
	}

	pic := m.Picture()
	if pic == nil || len(pic.Data) == 0 {
		return nil, nil
	}

	mime := pic.MIMEType
	if mime == "" {
		// Detect from data
		if len(pic.Data) > 2 && pic.Data[0] == 0xFF && pic.Data[1] == 0xD8 {
			mime = "image/jpeg"
		} else if len(pic.Data) > 4 && string(pic.Data[:4]) == "\x89PNG" {
			mime = "image/png"
		} else {
			mime = "image/jpeg" // default assumption
		}
	}

	return &ArtData{Data: pic.Data, MimeType: mime}, nil
}
