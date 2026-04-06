package library

import "testing"

func TestIsAudioFile(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"song.flac", true},
		{"song.mp3", true},
		{"song.wav", true},
		{"song.ogg", true},
		{"song.m4a", true},
		{"song.txt", false},
		{"song.jpg", false},
		{"", false},
		{"song.FLAC", true},  // uppercase
		{"song.Mp3", true},   // mixed case
	}

	for _, c := range cases {
		got := IsAudioFile(c.name)
		if got != c.want {
			t.Errorf("IsAudioFile(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestFormatFromExt(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"song.flac", "flac"},
		{"song.mp3", "mp3"},
		{"song.wav", "wav"},
		{"song.ogg", "ogg"},
		{"song.m4a", "aac"},
		{"song.aac", "aac"},
		{"song.aiff", "aiff"},
		{"song.aif", "aiff"},
		{"song.wma", "wma"},
		{"song.ape", "ape"},
		{"song.dsf", "dsf"},
		{"song.dff", "dff"},
		{"song.txt", ""},
		{"song.jpg", ""},
		{"noext", ""},
		{"", ""},
		{"song.FLAC", "flac"}, // uppercase normalised
	}

	for _, c := range cases {
		got := FormatFromExt(c.name)
		if got != c.want {
			t.Errorf("FormatFromExt(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestCleanTrackTitle(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"01 - So What", "So What"},
		{"01. So What", "So What"},
		{"1-Track", "Track"},
		{"12 Title", "Title"},
		{"NoNumber", "NoNumber"},
		{"01", "01"}, // all digits, no suffix — returned as-is
	}

	for _, c := range cases {
		got := cleanTrackTitle(c.input)
		if got != c.want {
			t.Errorf("cleanTrackTitle(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}
