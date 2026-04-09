package library

import (
	"os"
	"testing"
)

func TestReadFLACStreamInfo(t *testing.T) {
	// Build a minimal valid FLAC file in memory:
	// 4 bytes magic + 4 bytes block header + 34 bytes STREAMINFO = 42 bytes
	data := make([]byte, 42)

	// Magic: "fLaC"
	copy(data[0:4], "fLaC")

	// Block header: last-metadata-block=1 (0x80), type=0 (STREAMINFO), length=34
	data[4] = 0x80 // is_last=1, type=0
	data[5] = 0
	data[6] = 0
	data[7] = 34

	// STREAMINFO (34 bytes):
	// min_block=4096 (0x1000), max_block=4096 (0x1000)
	data[8] = 0x10
	data[9] = 0x00
	data[10] = 0x10
	data[11] = 0x00
	// min_frame=0 (3 bytes), max_frame=0 (3 bytes) — bytes 12-17 are zero
	//
	// Bytes 18-25: 8 bytes (64 bits) packed as:
	//   sample_rate (20 bits) | channels-1 (3 bits) | bps-1 (5 bits) | total_samples (36 bits)
	//
	// sample_rate = 44100 = 0xAC44 = 0b0000_1010_1100_0100_0100 (20 bits)
	// channels-1  = 1 (stereo) = 0b001 (3 bits)
	// bps-1       = 15 (16-bit) = 0b0_1111 (5 bits)
	// total_samples = 441000 = 0x6BAA8 (36 bits)
	//
	// Packed 64-bit value = 0x0AC442F00006BAA8:
	//   byte[0] = 0x0A, byte[1] = 0xC4, byte[2] = 0x42, byte[3] = 0xF0
	//   byte[4] = 0x00, byte[5] = 0x06, byte[6] = 0xBA, byte[7] = 0xA8
	data[18] = 0x0A
	data[19] = 0xC4
	data[20] = 0x42
	data[21] = 0xF0
	data[22] = 0x00
	data[23] = 0x06
	data[24] = 0xBA
	data[25] = 0xA8

	// Write to temp file
	f, err := os.CreateTemp(t.TempDir(), "test-*.flac")
	if err != nil {
		t.Fatal(err)
	}
	f.Write(data)
	f.Close()

	info, err := ReadFLACStreamInfo(f.Name())
	if err != nil {
		t.Fatalf("ReadFLACStreamInfo: %v", err)
	}

	if info.SampleRate != 44100 {
		t.Errorf("SampleRate = %d, want 44100", info.SampleRate)
	}
	if info.BitDepth != 16 {
		t.Errorf("BitDepth = %d, want 16", info.BitDepth)
	}
	if info.TotalSamples != 441000 {
		t.Errorf("TotalSamples = %d, want 441000", info.TotalSamples)
	}
}

func TestReadFLACStreamInfo_NotFLAC(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "test-*.mp3")
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte("not a flac file at all"))
	f.Close()

	_, err = ReadFLACStreamInfo(f.Name())
	if err == nil {
		t.Error("expected error for non-FLAC file")
	}
}
