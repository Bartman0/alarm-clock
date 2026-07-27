package audio

import (
	"log"
	"time"

	"alarmclock/internal/alarm"
)

// fadeIn is how long the alarm sound ramps from silence to full volume.
const fadeIn = 20 * time.Second

// Controller is the single owner of the mpv player. It serves both the alarm
// ringer (Start/Stop, satisfying ui.Ringer) and internet-radio playback
// (PlayStream/StopStream, satisfying ui.RadioPlayer). Starting an alarm or a
// stream replaces whatever was playing before.
type Controller struct {
	player     *Player
	alarmSound string
}

// NewController prepares the alarm sound and an mpv player. It never returns
// nil; if audio can't be initialised it logs instead of playing.
func NewController() *Controller {
	snd, err := EnsureAlarmSound()
	if err != nil {
		log.Printf("audio: preparing alarm sound: %v", err)
	}
	p, err := NewPlayer()
	if err != nil {
		log.Printf("audio: %v (playback disabled)", err)
	}
	return &Controller{player: p, alarmSound: snd}
}

// Start rings the alarm tone: the alarm sound looped with a gentle fade-in.
// (Spotify alarms play the tone first, then switch to music; see alarmRinger.)
func (c *Controller) Start(a alarm.Alarm) {
	if !c.player.Enabled() || c.alarmSound == "" {
		log.Printf("audio: alarm %s ringing (no audio backend)", a.TimeString())
		return
	}
	c.player.PlayLoopFadeIn(c.alarmSound, fadeIn, 100)
}

// Stop halts the alarm sound.
func (c *Controller) Stop() {
	c.player.Stop()
}

// PlayStream plays an internet-radio stream URL.
func (c *Controller) PlayStream(url string) {
	if !c.player.Enabled() {
		log.Printf("audio: would play stream %s (no audio backend)", url)
		return
	}
	c.player.Play(url)
}

// StopStream halts radio playback.
func (c *Controller) StopStream() {
	c.player.Stop()
}

// Close releases the mpv process; call on shutdown.
func (c *Controller) Close() {
	c.player.Close()
}
