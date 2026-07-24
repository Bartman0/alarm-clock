package ui

import (
	"log"

	"alarmclock/internal/alarm"
)

// Ringer plays and stops the sound for a firing alarm. The real, mpv/Spotify
// backed implementation arrives in Milestone 4; until then LogRinger stands in
// so the firing/snooze flow is fully exercisable.
type Ringer interface {
	Start(a alarm.Alarm)
	Stop()
}

// LogRinger just logs start/stop, so alarm timing and the firing screen can be
// verified without an audio backend.
type LogRinger struct{}

func (LogRinger) Start(a alarm.Alarm) {
	log.Printf("alarm ringing: %s — %s / %s", a.TimeString(), a.Rhythm, a.Sound.Kind)
}

func (LogRinger) Stop() {
	log.Printf("alarm stopped")
}
