package audio

import (
	"log"
	"time"

	"alarmclock/internal/alarm"
)

// fadeIn is how long the alarm sound ramps from silence to full volume.
const fadeIn = 20 * time.Second

// Ringer plays the alarm sound through mpv with a gentle fade-in. It satisfies
// the ui.Ringer interface. Spotify-backed alarms fall back to the alarm sound
// until Milestone 6 wires up playback.
type Ringer struct {
	player     *Player
	alarmSound string
}

// NewRinger prepares the alarm sound and an mpv player. It never returns nil;
// if audio can't be initialised the ringer logs instead of playing.
func NewRinger() *Ringer {
	snd, err := EnsureAlarmSound()
	if err != nil {
		log.Printf("audio: preparing alarm sound: %v", err)
	}
	p, err := NewPlayer()
	if err != nil {
		log.Printf("audio: %v (alarms will be silent)", err)
	}
	return &Ringer{player: p, alarmSound: snd}
}

func (r *Ringer) Start(a alarm.Alarm) {
	if a.Sound.Kind == alarm.SoundSpotify {
		log.Printf("audio: Spotify alarms arrive in M6; playing the alarm sound instead")
	}
	if !r.player.Enabled() || r.alarmSound == "" {
		log.Printf("audio: alarm %s ringing (no audio backend)", a.TimeString())
		return
	}
	r.player.PlayLoopFadeIn(r.alarmSound, fadeIn, 100)
}

func (r *Ringer) Stop() {
	r.player.Stop()
}

// Close releases the mpv process; call on shutdown.
func (r *Ringer) Close() {
	r.player.Close()
}
