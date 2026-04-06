package dlna

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	cases := []struct {
		input string
		want  time.Duration
	}{
		{"0:02:30", 2*time.Minute + 30*time.Second},
		{"00:02:30", 2*time.Minute + 30*time.Second},
		{"1:00:00", time.Hour},
		{"0:00:30.500", 30*time.Second + 500*time.Millisecond},
		{"", 0},
		{"NOT_IMPLEMENTED", 0},
	}

	for _, tc := range cases {
		got := parseDuration(tc.input)
		if got != tc.want {
			t.Errorf("parseDuration(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		input time.Duration
		want  string
	}{
		{2*time.Minute + 30*time.Second, "0:02:30"},
		{time.Hour, "1:00:00"},
		{0, "0:00:00"},
	}

	for _, tc := range cases {
		got := formatDuration(tc.input)
		if got != tc.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
