package audio

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateAlarmWav(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alarm.wav")
	if err := generateAlarmWav(path); err != nil {
		t.Fatalf("generateAlarmWav: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 44 {
		t.Fatalf("file too small: %d bytes", len(data))
	}
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		t.Fatalf("bad WAV header: %q %q", data[0:4], data[8:12])
	}

	// data chunk size in the header must match the bytes that follow it.
	dataSize := binary.LittleEndian.Uint32(data[40:44])
	if int(dataSize) != len(data)-44 {
		t.Fatalf("data chunk size %d, want %d", dataSize, len(data)-44)
	}
	// 2 seconds of 16-bit mono at 44100 Hz.
	if want := sampleRate * 2 * 2; int(dataSize) != want {
		t.Fatalf("data size %d, want %d (2s mono 16-bit)", dataSize, want)
	}
}
