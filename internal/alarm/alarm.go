// Package alarm defines the alarm domain model: the three configurable alarms,
// their weekly rhythm, the chosen sound, and next-fire computation.
package alarm

import (
	"fmt"
	"time"
)

// Rhythm decides on which weekdays an alarm is active.
type Rhythm int

const (
	FullWeek Rhythm = iota // every day
	Workweek               // Monday–Friday
	Weekend                // Saturday & Sunday
)

// Active reports whether the alarm should fire on the given weekday.
func (r Rhythm) Active(wd time.Weekday) bool {
	switch r {
	case Workweek:
		return wd >= time.Monday && wd <= time.Friday
	case Weekend:
		return wd == time.Saturday || wd == time.Sunday
	default: // FullWeek
		return true
	}
}

// String returns the Dutch label for the rhythm.
func (r Rhythm) String() string {
	switch r {
	case Workweek:
		return "Werkweek"
	case Weekend:
		return "Weekend"
	default:
		return "Hele week"
	}
}

// SoundKind selects what plays when the alarm fires.
type SoundKind int

const (
	SoundAlarm   SoundKind = iota // a built-in alarm sound
	SoundSpotify                  // a Spotify track/playlist
)

// String returns the Dutch label for the sound kind.
func (k SoundKind) String() string {
	if k == SoundSpotify {
		return "Spotify"
	}
	return "Alarmgeluid"
}

// Sound is the audio played when an alarm fires. Ref identifies a bundled
// sound file (for SoundAlarm) or a Spotify context URI (for SoundSpotify);
// Label is a human-readable name for the Spotify choice.
type Sound struct {
	Kind  SoundKind `json:"kind"`
	Ref   string    `json:"ref"`
	Label string    `json:"label,omitempty"`
}

// Alarm is a single configurable alarm.
type Alarm struct {
	Enabled bool   `json:"enabled"`
	Hour    int    `json:"hour"`
	Minute  int    `json:"minute"`
	Rhythm  Rhythm `json:"rhythm"`
	Sound   Sound  `json:"sound"`
}

// TimeString formats the alarm time as HH:MM.
func (a Alarm) TimeString() string {
	return fmt.Sprintf("%02d:%02d", a.Hour, a.Minute)
}

// Next returns the next moment this alarm will fire strictly after now, or
// ok=false when the alarm is disabled. It scans up to a week ahead so it
// honours the rhythm across weekends.
func (a Alarm) Next(now time.Time) (t time.Time, ok bool) {
	if !a.Enabled {
		return time.Time{}, false
	}
	for d := 0; d < 8; d++ {
		day := now.AddDate(0, 0, d)
		cand := time.Date(day.Year(), day.Month(), day.Day(), a.Hour, a.Minute, 0, 0, now.Location())
		if a.Rhythm.Active(cand.Weekday()) && cand.After(now) {
			return cand, true
		}
	}
	return time.Time{}, false
}
