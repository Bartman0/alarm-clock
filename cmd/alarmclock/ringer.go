package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"alarmclock/internal/alarm"
	"alarmclock/internal/spotify"
)

// Volume has two layers. The PipeWire sink holds the intended level: the
// slider on the home screen sets it, and it applies to everything. The Connect
// volume of our librespot device is the transient layer on top of it, for
// Spotify alone: 0 while an alarm warms up or is snoozed (and at rest, so a
// librespot that comes back after a crash is silent), ramped to 100 — the
// slider's level — when it rings.

// These are variables only so tests can run the ringer at speed.
var (
	// playbackConfirm is how long we wait for the Connect device to really
	// start rendering audio, and playbackPoll how often we check. Spotify
	// accepting a Play command proves nothing: librespot still has to take
	// over playback, and player commands issued before it does come back
	// "Restriction violated".
	playbackConfirm = 20 * time.Second
	playbackPoll    = time.Second

	// revealConfirm is how long a warmed-up playlist gets, after skipping to
	// a fresh track at ringing time, to prove it is still playing before the
	// alarm tone takes over. It plays silently either way at that point.
	revealConfirm = 10 * time.Second

	// retryInterval is the pause between attempts to get the playlist going.
	retryInterval = 3 * time.Second

	// librespotRecheck is how long we give librespot to come back and play
	// after a restart before restarting it again.
	librespotRecheck = 45 * time.Second

	// silentMax is how long a session may play at volume 0 without ringing.
	// The scheduler cancels warm-ups that will not ring; this is the backstop
	// against streaming silently all day if that ever goes wrong.
	silentMax = 10 * time.Minute

	// rampUp is how long the music takes to rise from silence to the
	// slider's level, in rampSteps steps (each one a Web API call).
	rampUp    = 20 * time.Second
	rampSteps = 10
)

// tone is the mpv alarm tone (audio.Controller); Stop also ends the radio.
type tone interface {
	Start(a alarm.Alarm)
	Stop()
}

// restarter forces a fresh librespot (librespot.Supervisor).
type restarter interface {
	Restart()
}

// alarmRinger sounds a firing alarm. A tone alarm is the mpv tone. A Spotify
// alarm is a session that starts its playlist at volume 0 ahead of time (see
// Prepare), so when the alarm is due all that is left is to skip to a fresh
// track and raise the volume. Whenever Spotify is not confirmed playing at
// that moment, the mpv tone rings while the session keeps trying, and is
// silenced once music is. It satisfies ui.Ringer.
//
// One goroutine (loop) owns everything below reqs and is the only one to touch
// the tone. The public methods just queue a request for it, so they never
// block their caller. Spotify calls run as jobs in their own goroutine, one at
// a time: a job never stalls the loop, and the pause that ends one alarm
// always lands before the next alarm's Play.
type alarmRinger struct {
	audio  tone
	spot   *spotify.Client
	lib    restarter
	device string
	reqs   chan func()

	sess        *session // the Spotify alarm in progress, if any
	job         *job     // the Spotify job in flight, if any
	dirty       bool     // our device may be playing: silence it once free
	lastRestart time.Time
}

// session is one Spotify alarm occurrence, from its warm-up through ringing
// and snoozes until it is stopped.
type session struct {
	alarm       alarm.Alarm
	ringing     bool      // the alarm should be audible now
	rung        bool      // it has rung at least once (so it is no mere warm-up)
	ready       bool      // the playlist is confirmed playing on our device
	id          string    // our device id, once ready
	audible     bool      // our volume is up (or ramping up)
	failed      bool      // the last attempt to start it failed
	silentSince time.Time // when it last stopped ringing (or was created)
}

// jobKind is a step towards what the session wants; see nextJob.
type jobKind int

const (
	jobNone    jobKind = iota
	jobStart           // get the playlist playing at volume 0
	jobReveal          // skip to a fresh track and ramp the volume up
	jobLower           // back to volume 0 for a snooze
	jobSilence         // pause our device and leave it at volume 0
)

type job struct {
	kind   jobKind
	sess   *session
	cancel context.CancelFunc
}

func newAlarmRinger(audio tone, spot *spotify.Client, lib restarter, device string) *alarmRinger {
	r := &alarmRinger{audio: audio, spot: spot, lib: lib, device: device, reqs: make(chan func(), 16)}
	go r.loop()
	return r
}

// do runs f on the loop goroutine.
func (r *alarmRinger) do(f func()) { r.reqs <- f }

// Prepare starts a Spotify alarm's playlist at volume 0, ahead of the alarm.
// librespot's long-lived session to Spotify can be dead after a night idle
// without librespot knowing; playing for real, silently, finds that out — and
// gives time to restart it — before anyone needs to wake up.
func (r *alarmRinger) Prepare(a alarm.Alarm, at time.Time) {
	r.do(func() {
		if !r.spotifyAlarm(a) {
			return
		}
		if r.sess != nil {
			log.Printf("spotify alarm: not warming up %s: another alarm holds the device", a.TimeString())
			return
		}
		log.Printf("spotify alarm: warming up the %s alarm at volume 0, %s ahead", a.TimeString(), time.Until(at).Round(time.Second))
		r.sess = newSession(a)
	})
}

// CancelPrepare ends a warm-up that will not ring after all. A session that
// has already rung is left alone: that is Stop's to end.
func (r *alarmRinger) CancelPrepare() {
	r.do(func() {
		if r.sess == nil || r.sess.rung {
			return
		}
		log.Printf("spotify alarm: warm-up for %s cancelled", r.sess.alarm.TimeString())
		r.endSession()
	})
}

// Start makes the alarm audible: the warmed-up session if there is one for
// this alarm, a fresh one otherwise, and the tone whenever music is not yet
// confirmed.
func (r *alarmRinger) Start(a alarm.Alarm) {
	r.do(func() {
		if r.sess != nil && (!r.spotifyAlarm(a) || r.sess.alarm.Sound.Ref != a.Sound.Ref) {
			r.endSession() // a different alarm takes over the device
		}
		if !r.spotifyAlarm(a) {
			r.audio.Start(a)
			return
		}
		if r.sess == nil {
			r.sess = newSession(a)
		}
		s := r.sess
		s.ringing, s.rung = true, true
		if s.ready {
			log.Printf("spotify alarm: %s is due; bringing up the warmed-up playlist", a.TimeString())
			return
		}
		log.Printf("spotify alarm: %s is due but Spotify is not playing yet; ringing the tone meanwhile", a.TimeString())
		r.audio.Start(a)
	})
}

// Snooze silences the alarm but keeps a Spotify session playing at volume 0,
// so ringing again is once more just a skip and a volume ramp.
func (r *alarmRinger) Snooze() {
	r.do(func() {
		r.audio.Stop()
		if r.sess != nil {
			r.sess.ringing = false
			r.sess.silentSince = time.Now()
		}
	})
}

// Stop silences everything: the tone, the radio, the alarm's session, and
// any music playing on our device.
func (r *alarmRinger) Stop() {
	r.do(func() {
		r.audio.Stop()
		r.endSession()
	})
}

func (r *alarmRinger) spotifyAlarm(a alarm.Alarm) bool {
	return a.Sound.Kind == alarm.SoundSpotify && a.Sound.Ref != "" && r.spot != nil && r.spot.Authorized()
}

func newSession(a alarm.Alarm) *session {
	return &session{alarm: a, silentSince: time.Now()}
}

// endSession drops the current session, if any, and marks our device to be
// paused and returned to volume 0 once the job in flight is done. That happens
// even without a session, so Stop also ends music started from the Spotify
// screen.
func (r *alarmRinger) endSession() {
	r.sess = nil
	if r.spot != nil && r.spot.Authorized() {
		r.dirty = true
	}
}

// loop serves requests and job results, and after each one moves the device
// a step closer to what the session wants. It also ends a session that has
// played silently for silentMax.
func (r *alarmRinger) loop() {
	for {
		r.schedule()
		var expire <-chan time.Time
		if s := r.sess; s != nil && !s.ringing {
			expire = time.After(silentMax - time.Since(s.silentSince))
		}
		select {
		case f := <-r.reqs:
			f()
		case <-expire:
			log.Printf("spotify alarm: %s has played silently for %s without ringing; ending it", r.sess.alarm.TimeString(), silentMax)
			r.endSession()
		}
	}
}

// nextJob is the step that brings the device closer to what the session wants.
func (r *alarmRinger) nextJob() jobKind {
	s := r.sess
	switch {
	case s == nil && r.dirty:
		return jobSilence
	case s == nil:
		return jobNone
	case !s.ready:
		return jobStart
	case s.ringing && !s.audible:
		return jobReveal
	case !s.ringing && s.audible:
		return jobLower
	}
	return jobNone
}

// stale reports whether the job in flight no longer serves the session: it
// belongs to one that has ended, or it ramps up an alarm that was snoozed.
// Silencing is never stale: the next Play must not overtake its Pause.
func (r *alarmRinger) stale(j *job) bool {
	switch {
	case j.kind == jobSilence:
		return false
	case j.sess != r.sess:
		return true
	}
	return j.kind == jobReveal && !j.sess.ringing
}

// schedule cancels a stale job and, once no job is in flight, starts the next
// one.
func (r *alarmRinger) schedule() {
	if j := r.job; j != nil {
		if r.stale(j) {
			j.cancel()
		}
		return
	}
	next := r.nextJob()
	if next == jobNone {
		return
	}

	s := r.sess
	ctx, cancel := context.WithCancel(context.Background())
	if next == jobSilence {
		ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	}
	r.job = &job{kind: next, sess: s, cancel: cancel}
	switch next {
	case jobSilence:
		r.dirty = false
		go func() {
			defer cancel()
			r.silence(ctx)
			r.do(func() { r.job = nil })
		}()

	case jobStart:
		a := s.alarm
		// Either the device dropped off while idle, or it is a stale entry
		// on Spotify's list that accepts commands without playing. A fresh
		// librespot fixes both.
		restart := s.failed && time.Since(r.lastRestart) >= librespotRecheck
		if restart {
			r.lastRestart = time.Now()
		}
		go func() {
			defer cancel()
			if restart {
				log.Printf("spotify alarm: restarting librespot and trying again")
				r.lib.Restart()
			}
			id, ok := "", false
			if !s.failed || sleepCtx(ctx, retryInterval) {
				id, ok = r.startSilently(ctx, a)
			}
			stopped := ctx.Err() != nil
			r.do(func() {
				r.job = nil
				switch {
				case ok:
					s.ready, s.id, s.failed, s.audible = true, id, false, false
				case !stopped:
					s.failed = true
				}
			})
		}()

	case jobReveal:
		id := s.id
		go func() {
			defer cancel()
			ok := r.reveal(ctx, s, id)
			stopped := ctx.Err() != nil
			r.do(func() {
				r.job = nil
				if ok || stopped || r.sess != s {
					return
				}
				s.ready = false
				if s.ringing {
					log.Printf("spotify alarm: lost the playlist while ringing; the tone rings until it is back")
					r.audio.Start(s.alarm)
				}
			})
		}()

	case jobLower:
		id := s.id
		go func() {
			defer cancel()
			if err := r.spot.SetVolume(ctx, id, 0); err != nil {
				log.Printf("spotify alarm: lowering the volume for the snooze failed: %v", err)
			}
			r.do(func() {
				r.job = nil
				s.audible = false
			})
		}()
	}
}

// silence pauses our device and returns its Connect volume to rest (0). Other
// devices are left alone: the account may be playing on a phone.
func (r *alarmRinger) silence(ctx context.Context) {
	st, ok, err := r.spot.PlaybackState(ctx)
	if err != nil {
		log.Printf("spotify alarm: cannot check the player to silence it: %v", err)
		return
	}
	if !ok || st.Device.Name != r.device {
		return
	}
	if st.IsPlaying {
		if err := r.spot.Pause(ctx); err != nil {
			log.Printf("spotify alarm: pause failed: %v", err)
		}
	}
	if err := r.spot.SetVolume(ctx, st.Device.ID, 0); err != nil {
		log.Printf("spotify alarm: returning the volume to 0 failed: %v", err)
	}
}

// startSilently starts the alarm's playlist on our device at volume 0 and
// waits for proof that it is really playing. Returns the device id.
func (r *alarmRinger) startSilently(ctx context.Context, a alarm.Alarm) (string, bool) {
	devs, err := r.spot.Devices(ctx)
	if err != nil {
		log.Printf("spotify alarm: device lookup failed: %v", err)
		return "", false
	}
	var dev *spotify.Device
	for i := range devs {
		if devs[i].Name == r.device {
			dev = &devs[i]
			break
		}
	}
	if dev == nil {
		r.logDevices(devs)
		return "", false
	}
	id := dev.ID
	// The device rests at 0, but someone may have left it up after playing
	// to it from a phone. Playing like that is an alarm going off early, and
	// an idle device can refuse a volume change — then a librespot restart
	// (before the next attempt) brings it back at 0.
	if v := dev.VolumePercent; v == nil || *v != 0 {
		if err := r.spot.SetVolume(ctx, id, 0); err != nil {
			log.Printf("spotify alarm: %q is idle at a volume above 0 and refuses to go to 0 (%v)", r.device, err)
			return "", false
		}
	}
	if err := r.spot.Play(ctx, id, a.Sound.Ref, nil); err != nil {
		log.Printf("spotify alarm: play failed: %v", err)
		return "", false
	}
	if err := r.spot.SetVolume(ctx, id, 0); err != nil {
		// Playing at whatever volume the device had is an alarm going off
		// early. Don't.
		log.Printf("spotify alarm: cannot silence the device (%v); pausing and trying again", err)
		_ = r.spot.Pause(ctx)
		return "", false
	}
	if !r.playbackConfirmed(ctx, id, playbackConfirm) {
		if ctx.Err() == nil {
			log.Printf("spotify alarm: %q accepted Play but never started playing", r.device)
		}
		return "", false
	}
	if err := r.spot.Shuffle(ctx, id, true); err != nil {
		log.Printf("spotify alarm: shuffle failed: %v", err)
	}
	log.Printf("spotify alarm: %s is playing silently on %q", a.TimeString(), r.device)
	return id, true
}

// reveal makes a silently playing session audible: skip to a fresh track of
// the shuffled playlist, confirm it plays, silence the tone (and the radio),
// and ramp the volume up. A snooze or stop cancels ctx, cutting the ramp
// short. Returns false if the music did not come back after the skip.
func (r *alarmRinger) reveal(ctx context.Context, s *session, id string) bool {
	if err := r.spot.Next(ctx, id); err != nil {
		log.Printf("spotify alarm: next failed: %v", err)
	}
	if !r.playbackConfirmed(ctx, id, revealConfirm) {
		if ctx.Err() == nil {
			log.Printf("spotify alarm: the playlist stopped playing before it could ring")
		}
		return false
	}

	r.do(func() {
		if r.sess == s {
			s.audible = true // from here on a snooze must lower it again
			if s.ringing {
				r.audio.Stop()
			}
		}
	})
	log.Printf("spotify alarm: ringing with music on %q, ramping up over %s", r.device, rampUp)

	for i := 1; i <= rampSteps; i++ {
		if !sleepCtx(ctx, rampUp/time.Duration(rampSteps)) {
			return true
		}
		if err := r.spot.SetVolume(ctx, id, 100*i/rampSteps); err != nil {
			log.Printf("spotify alarm: volume step failed: %v", err)
		}
	}
	return true
}

// playbackConfirmed reports whether deviceID is genuinely rendering audio. The
// player must say it is playing on that device AND its track position must
// advance between two reads: is_playing alone can be true while the device
// produces nothing at all. Returns false if ctx is cancelled or the deadline
// passes first.
func (r *alarmRinger) playbackConfirmed(ctx context.Context, deviceID string, within time.Duration) bool {
	deadline := time.Now().Add(within)
	prev := -1
	seen := "no reading taken"
	for time.Now().Before(deadline) {
		st, ok, err := r.spot.PlaybackState(ctx)
		switch {
		case err != nil:
			seen = fmt.Sprintf("player state unavailable: %v", err)
			prev = -1
		case !ok:
			seen = "Spotify reports nothing playing anywhere"
			prev = -1
		case st.Device.ID != deviceID:
			seen = fmt.Sprintf("playback is on %q, not us", st.Device.Name)
			prev = -1
		case !st.IsPlaying:
			seen = "our device holds playback but is paused"
			prev = -1
		case prev >= 0 && st.ProgressMS > prev:
			return true // the track position moved: audio is really running
		case prev < 0:
			seen = fmt.Sprintf("playing, first position reading %dms", st.ProgressMS)
			prev = st.ProgressMS
		default:
			// The usual shape of a silent alarm: Spotify believes the track
			// is playing, but the position never moves, so librespot is
			// connected without rendering anything.
			seen = fmt.Sprintf("is_playing is true but the position is frozen at %dms", st.ProgressMS)
			prev = st.ProgressMS
		}
		if !sleepCtx(ctx, playbackPoll) {
			return false
		}
	}
	log.Printf("spotify alarm: playback unconfirmed after %s — last reading: %s", within, seen)
	return false
}

// logDevices records what Spotify can see when our device is missing, so a
// failure says what else was on the list.
func (r *alarmRinger) logDevices(devs []spotify.Device) {
	if len(devs) == 0 {
		log.Printf("spotify alarm: Spotify sees no Connect devices at all")
		return
	}
	seen := make([]string, 0, len(devs))
	for _, d := range devs {
		seen = append(seen, fmt.Sprintf("%q(active=%t)", d.Name, d.IsActive))
	}
	log.Printf("spotify alarm: %q is not on the device list: %s", r.device, strings.Join(seen, " "))
}

// sleepCtx sleeps for d, returning false if ctx is cancelled first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
