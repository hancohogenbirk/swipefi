package library

import (
	"encoding/binary"
	"fmt"
	"os"
)

// FLACStreamInfo holds audio format info from a FLAC file's STREAMINFO block.
type FLACStreamInfo struct {
	SampleRate   int   // Hz (e.g. 44100, 96000)
	BitDepth     int   // bits per sample (e.g. 16, 24)
	TotalSamples int64 // total samples in stream
}

// ReadFLACStreamInfo reads the STREAMINFO metadata block from a FLAC file.
// Only reads the first 42 bytes of the file.
func ReadFLACStreamInfo(path string) (*FLACStreamInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Read: 4 (magic) + 4 (block header) + 34 (STREAMINFO) = 42 bytes
	buf := make([]byte, 42)
	if _, err := f.Read(buf); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	// Verify FLAC magic
	if string(buf[0:4]) != "fLaC" {
		return nil, fmt.Errorf("not a FLAC file")
	}

	// Block header: byte 4 has type in lower 7 bits (should be 0 for STREAMINFO)
	blockType := buf[4] & 0x7F
	if blockType != 0 {
		return nil, fmt.Errorf("first block is not STREAMINFO (type %d)", blockType)
	}

	// STREAMINFO starts at byte 8, bytes 10-17 relative to STREAMINFO start
	// contain sample rate, channels, bps, total samples packed as 64 bits.
	// Offset from file start: 8 + 10 = 18
	packed := binary.BigEndian.Uint64(buf[18:26])

	sampleRate := int((packed >> 44) & 0xFFFFF)
	bitDepth := int((packed>>36)&0x1F) + 1
	totalSamples := int64(packed & 0xFFFFFFFFF)

	if sampleRate == 0 {
		return nil, fmt.Errorf("invalid sample rate 0")
	}

	return &FLACStreamInfo{
		SampleRate:   sampleRate,
		BitDepth:     bitDepth,
		TotalSamples: totalSamples,
	}, nil
}
