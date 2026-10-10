package ui

import (
	"log"
	"time"

	"alarmclock/internal/alarm"
)

// Ringer plays and stops the sound for a firing alarm. Snooze silences it for
// now; the scheduler calls Start again when the snooze has passed.
type Ringer interface {
	Start(a alarm.Alarm)
	Snooze()
	Stop()
}

// Preparer is an optional Ringer capability. The scheduler calls Prepare once
// per alarm occurrence, prepareLead before it is due, so a ringer that depends
// on something fragile (a Spotify Connect session that goes stale overnight)
// can get it working — silently — while there is still time to spare.
// CancelPrepare abandons that when the occurrence will not ring after all (the
// alarm was edited, disabled or missed). Implementations must return promptly
// and do their work in the background.
type Preparer interface {
	Prepare(a alarm.Alarm, at time.Time)
	CancelPrepare()
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

func (LogRinger) Snooze() {
	log.Printf("alarm snoozed")
}

func (LogRinger) Stop() {
	log.Printf("alarm stopped")
}
