// Package config loads and saves the alarm clock's persistent state (currently
// the three alarms) as JSON under the user's config directory.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"alarmclock/internal/alarm"
)

// Store holds all persisted state and knows where to write it.
type Store struct {
	Alarms [3]alarm.Alarm `json:"alarms"`

	path string
}

// defaultPath returns ~/.config/alarmclock/config.json (or the platform
// equivalent), falling back to the working directory if it can't be resolved.
func defaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "config.json"
	}
	return filepath.Join(dir, "alarmclock", "config.json")
}

// defaults returns the initial three alarms, all disabled.
func defaults() [3]alarm.Alarm {
	return [3]alarm.Alarm{
		{Hour: 7, Minute: 0, Rhythm: alarm.Workweek},
		{Hour: 9, Minute: 0, Rhythm: alarm.Weekend},
		{Hour: 8, Minute: 0, Rhythm: alarm.FullWeek},
	}
}

// Load reads the config file. A missing file is not an error: it returns a
// store seeded with sensible defaults, ready to Save.
func Load() (*Store, error) {
	path := defaultPath()
	s := &Store{Alarms: defaults(), path: path}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return s, err
	}
	return s, nil
}

// Save writes the config file, creating the directory if needed.
func (s *Store) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}
