package ui

import (
	"os/exec"
	"strconv"
	"strings"
)

// systemVolume reads the default PipeWire sink volume (0..1) via wpctl.
func systemVolume() (float32, bool) {
	out, err := exec.Command("wpctl", "get-volume", "@DEFAULT_AUDIO_SINK@").Output()
	if err != nil {
		return 0, false
	}
	// Output looks like: "Volume: 0.90" (optionally " [MUTED]").
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return 0, false
	}
	v, err := strconv.ParseFloat(fields[1], 32)
	if err != nil {
		return 0, false
	}
	if v > 1 {
		v = 1
	}
	return float32(v), true
}

// setSystemVolume sets the default PipeWire sink volume (0..1) via wpctl.
func setSystemVolume(v float32) {
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	_ = exec.Command("wpctl", "set-volume", "@DEFAULT_AUDIO_SINK@",
		strconv.FormatFloat(float64(v), 'f', 2, 32)).Run()
}
