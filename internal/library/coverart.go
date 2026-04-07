package library

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	coverArtFetchTimeout = 10 * time.Second
	mbMinConfidenceScore = 80
	userAgent            = "SwipeFi/1.0 (https://github.com/hancohogenbirk/swipefi)"
)

var httpClient = &http.Client{
	Timeout: coverArtFetchTimeout,
}

// FetchCoverArt searches MusicBrainz for the artist+album and fetches cover art
// from the Cover Art Archive. Returns nil if not found.
func FetchCoverArt(artist, album string) (*ArtData, error) {
	if artist == "" || album == "" {
		return nil, nil
	}

	// Search MusicBrainz for a release matching artist + album
	mbid, err := searchMusicBrainz(artist, album)
	if err != nil {
		return nil, fmt.Errorf("musicbrainz search: %w", err)
	}
	if mbid == "" {
		return nil, nil
	}

	// Fetch cover from Cover Art Archive (250px thumbnail)
	data, err := fetchFromCoverArtArchive(mbid)
	if err != nil {
		return nil, fmt.Errorf("cover art archive: %w", err)
	}
	if data == nil {
		return nil, nil
	}

	return &ArtData{Data: data, MimeType: "image/jpeg"}, nil
}

type mbSearchResult struct {
	Releases []struct {
		ID    string `json:"id"`
		Score int    `json:"score"`
	} `json:"releases"`
}

func searchMusicBrainz(artist, album string) (string, error) {
	// Build URL manually — MusicBrainz Lucene query needs + for spaces, no encoding of : or AND
	safeArtist := strings.ReplaceAll(artist, " ", "+")
	safeAlbum := strings.ReplaceAll(album, " ", "+")
	reqURL := fmt.Sprintf("https://musicbrainz.org/ws/2/release/?query=artist:%s+AND+release:%s&fmt=json&limit=1",
		safeArtist, safeAlbum)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}

	var result mbSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Releases) == 0 || result.Releases[0].Score < mbMinConfidenceScore {
		return "", nil // no good match
	}

	slog.Debug("musicbrainz match", "artist", artist, "album", album, "mbid", result.Releases[0].ID, "score", result.Releases[0].Score)
	return result.Releases[0].ID, nil
}

func fetchFromCoverArtArchive(mbid string) ([]byte, error) {
	// Request 250px thumbnail — small and fast
	reqURL := fmt.Sprintf("https://coverartarchive.org/release/%s/front-250", mbid)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil // no cover art for this release
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Sanity check — should be an image
	if len(data) < 100 {
		return nil, nil
	}

	return data, nil
}
