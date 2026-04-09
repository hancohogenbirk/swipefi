package library

import (
	"net/url"
	"strings"
	"testing"
)

func TestBuildMusicBrainzQuery(t *testing.T) {
	tests := []struct {
		name        string
		artist      string
		album       string
		wantArtist  string // expected encoded artist in URL
		wantAlbum   string // expected encoded album in URL
	}{
		{
			name:       "simple names no special chars",
			artist:     "Ry Cooder",
			album:      "Paris Texas",
			wantArtist: "Ry+Cooder",
			wantAlbum:  "Paris+Texas",
		},
		{
			name:       "apostrophe in artist",
			artist:     "Guns N' Roses",
			album:      "Appetite for Destruction",
			wantArtist: "Guns+N%27+Roses",
			wantAlbum:  "Appetite+for+Destruction",
		},
		{
			name:       "ampersand in artist",
			artist:     "Simon & Garfunkel",
			album:      "Bridge Over Troubled Water",
			wantArtist: "Simon+%26+Garfunkel",
			wantAlbum:  "Bridge+Over+Troubled+Water",
		},
		{
			name:       "comma in album",
			artist:     "Miles Davis",
			album:      "Kind of Blue",
			wantArtist: "Miles+Davis",
			wantAlbum:  "Kind+of+Blue",
		},
		{
			name:       "special chars colon in album",
			artist:     "Various Artists",
			album:      "Best Of: 2000s",
			wantArtist: "Various+Artists",
			wantAlbum:  "Best+Of%3A+2000s",
		},
		{
			name:       "single word names",
			artist:     "Prince",
			album:      "Purple Rain",
			wantArtist: "Prince",
			wantAlbum:  "Purple+Rain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildMusicBrainzURL(tt.artist, tt.album)

			// URL must be parseable
			parsed, err := url.Parse(got)
			if err != nil {
				t.Fatalf("buildMusicBrainzURL returned invalid URL: %v", err)
			}

			// Must target musicbrainz.org
			if parsed.Host != "musicbrainz.org" {
				t.Errorf("unexpected host: got %q, want musicbrainz.org", parsed.Host)
			}

			// Must include limit=5
			if q := parsed.Query().Get("limit"); q != "5" {
				t.Errorf("expected limit=5, got %q", q)
			}

			// Must include fmt=json
			if q := parsed.Query().Get("fmt"); q != "json" {
				t.Errorf("expected fmt=json, got %q", q)
			}

			// Query must contain properly encoded artist and album
			rawQuery := parsed.RawQuery
			if !strings.Contains(rawQuery, tt.wantArtist) {
				t.Errorf("URL %q does not contain encoded artist %q", rawQuery, tt.wantArtist)
			}
			if !strings.Contains(rawQuery, tt.wantAlbum) {
				t.Errorf("URL %q does not contain encoded album %q", rawQuery, tt.wantAlbum)
			}
		})
	}
}
