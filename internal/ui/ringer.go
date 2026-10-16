package ui

import (
	"time"

	"alarmclock/internal/alarm"
)

// Ringer plays and stops the sound for a firing alarm. Snooze silences it for
// now; the scheduler calls Start again when the snooze has passed.
//
// The scheduler calls Prepare once per alarm occurrence, prepareLead before it
// is due, so a ringer that depends on something fragile (a Spotify Connect
// session that goes stale overnight) can get it working — silently — while
// there is still time to spare. CancelPrepare abandons that when the
// occurrence will not ring after all (the alarm was edited, disabled or
// missed).
//
// The scheduler drives the ringer while holding its lock, which the UI also
// takes every frame, so every method must return promptly and do any slow
// work (mpv, the Spotify Web API) in the background.
type Ringer interface {
	Start(a alarm.Alarm)
	Snooze()
	Stop()
	Prepare(a alarm.Alarm, at time.Time)
	CancelPrepare()
}

// RadioPlayer plays and stops an internet-radio stream URL. audio.Controller
// implements both this and Ringer over one shared mpv player.
type RadioPlayer interface {
	PlayStream(url string)
	StopStream()
}
