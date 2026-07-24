package audio

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
)

const sampleRate = 44100

// EnsureAlarmSound returns the path to the bundled alarm tone, generating it in
// the user cache dir on first use so we don't ship a binary asset.
func EnsureAlarmSound() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "alarmclock")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "alarm.wav")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	if err := generateAlarmWav(path); err != nil {
		return "", err
	}
	return path, nil
}

// generateAlarmWav writes a 2-second, seamlessly loopable beep pattern: four
// 880 Hz tones separated by silence, each tone edge-faded to avoid clicks.
func generateAlarmWav(path string) error {
	const (
		freq    = 880.0
		amp     = 0.6
		toneLen = sampleRate / 4 // 0.25s
		silLen  = sampleRate / 4 // 0.25s
		fade    = sampleRate / 200
	)
	var samples []int16
	for rep := 0; rep < 4; rep++ {
		for i := 0; i < toneLen; i++ {
			t := float64(i) / sampleRate
			env := 1.0
			if i < fade {
				env = float64(i) / fade
			} else if i > toneLen-fade {
				env = float64(toneLen-i) / fade
			}
			v := amp * env * math.Sin(2*math.Pi*freq*t)
			samples = append(samples, int16(v*math.MaxInt16))
		}
		for i := 0; i < silLen; i++ {
			samples = append(samples, 0)
		}
	}
	return writeWav(path, samples)
}

// writeWav writes 16-bit mono PCM samples as a canonical WAV file.
func writeWav(path string, samples []int16) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	dataSize := len(samples) * 2
	le := binary.LittleEndian
	put := func(order any) error { return binary.Write(f, le, order) }

	if _, err := f.WriteString("RIFF"); err != nil {
		return err
	}
	if err := put(uint32(36 + dataSize)); err != nil {
		return err
	}
	if _, err := f.WriteString("WAVEfmt "); err != nil {
		return err
	}
	for _, v := range []any{
		uint32(16),             // fmt chunk size
		uint16(1),              // PCM
		uint16(1),              // channels (mono)
		uint32(sampleRate),     // sample rate
		uint32(sampleRate * 2), // byte rate = rate * channels * bytesPerSample
		uint16(2),              // block align
		uint16(16),             // bits per sample
	} {
		if err := put(v); err != nil {
			return err
		}
	}
	if _, err := f.WriteString("data"); err != nil {
		return err
	}
	if err := put(uint32(dataSize)); err != nil {
		return err
	}
	return put(samples)
}
