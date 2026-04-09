package library

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	coverArtFetchTimeout    = 10 * time.Second
	mbMinConfidenceScore    = 80
	mbFuzzyConfidenceScore  = 70
	userAgent               = "SwipeFi/1.0 (https://github.com/hancohogenbirk/swipefi)"
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

// buildMusicBrainzURL constructs a structured Lucene query URL for the MusicBrainz API.
// It uses url.QueryEscape for proper encoding, then converts %20 to + for readability.
func buildMusicBrainzURL(artist, album string) string {
	encArtist := strings.ReplaceAll(url.QueryEscape(artist), "%20", "+")
	encAlbum := strings.ReplaceAll(url.QueryEscape(album), "%20", "+")
	query := fmt.Sprintf("artist%%3A%s+AND+release%%3A%s", encArtist, encAlbum)
	return fmt.Sprintf("https://musicbrainz.org/ws/2/release/?query=%s&fmt=json&limit=5", query)
}

// doMusicBrainzSearch performs a single search request and returns the best matching MBID
// if the top result meets the minScore threshold.
func doMusicBrainzSearch(reqURL string, minScore int) (string, error) {
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

	if len(result.Releases) == 0 || result.Releases[0].Score < minScore {
		return "", nil
	}

	return result.Releases[0].ID, nil
}

func searchMusicBrainz(artist, album string) (string, error) {
	// Try structured query first with high confidence threshold
	structuredURL := buildMusicBrainzURL(artist, album)
	mbid, err := doMusicBrainzSearch(structuredURL, mbMinConfidenceScore)
	if err != nil {
		return "", err
	}
	if mbid != "" {
		slog.Debug("musicbrainz structured match", "artist", artist, "album", album, "mbid", mbid)
		return mbid, nil
	}

	// Fuzzy fallback: simple combined query with lower confidence threshold
	fuzzyQuery := strings.ReplaceAll(url.QueryEscape(artist+" "+album), "%20", "+")
	fuzzyURL := fmt.Sprintf("https://musicbrainz.org/ws/2/release/?query=%s&fmt=json&limit=5", fuzzyQuery)
	mbid, err = doMusicBrainzSearch(fuzzyURL, mbFuzzyConfidenceScore)
	if err != nil {
		return "", err
	}
	if mbid != "" {
		slog.Debug("musicbrainz fuzzy match", "artist", artist, "album", album, "mbid", mbid)
	}
	return mbid, nil
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
