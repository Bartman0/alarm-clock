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

// Preparer is an optional Ringer capability. The scheduler calls Prepare once
// per alarm occurrence, prepareLead before it is due, so a ringer that depends
// on something fragile (a Spotify Connect session that goes stale overnight)
// can get it working again while there is still time to spare. Implementations
// must return promptly and do their work in the background.
type Preparer interface {
	Prepare(a alarm.Alarm)
}

// RadioPlayer plays and stops an internet-radio stream URL. audio.Controller
// implements both this and Ringer over one shared mpv player.
type RadioPlayer interface {
	PlayStream(url string)
	StopStream()
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
