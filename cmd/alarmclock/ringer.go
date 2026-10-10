package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"alarmclock/internal/alarm"
	"alarmclock/internal/spotify"
	"alarmclock/internal/ui"
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

// The scheduler finds Prepare by type assertion; keep a signature drift from
// silently switching the warm-up off.
var _ ui.Preparer = (*alarmRinger)(nil)

// alarmRinger sounds a firing alarm. A tone alarm is the mpv tone. A Spotify
// alarm is a session that starts its playlist at volume 0 ahead of time (see
// Prepare), so when the alarm is due all that is left is to skip to a fresh
// track and raise the volume. Whenever Spotify is not confirmed playing at
// that moment, the mpv tone rings while the session keeps trying, and is
// silenced once music is. It satisfies ui.Ringer and ui.Preparer.
type alarmRinger struct {
	audio  tone
	spot   *spotify.Client
	lib    restarter
	device string

	mu    sync.Mutex
	sess  *session      // the Spotify alarm in progress, if any
	quiet chan struct{} // closed once the last ended session is silenced
}

// session is one Spotify alarm occurrence, from its warm-up through ringing
// and snoozes until it is stopped. Its goroutine (run) owns all Spotify calls;
// the fields below are guarded by alarmRinger.mu.
type session struct {
	alarm  alarm.Alarm
	cancel context.CancelFunc
	wake   chan struct{} // nudges the goroutine when ringing changes
	done   chan struct{} // closed when the goroutine has returned

	ringing bool // the alarm should be audible now
	rung    bool // it has rung at least once (so it is no mere warm-up)
	ready   bool // the playlist is confirmed playing on our device
}

func (s *session) poke() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (r *alarmRinger) spotifyAlarm(a alarm.Alarm) bool {
	return a.Sound.Kind == alarm.SoundSpotify && a.Sound.Ref != "" && r.spot != nil && r.spot.Authorized()
}

// Prepare starts a Spotify alarm's playlist at volume 0, ahead of the alarm.
// librespot's long-lived session to Spotify can be dead after a night idle
// without librespot knowing; playing for real, silently, finds that out — and
// gives time to restart it — before anyone needs to wake up.
func (r *alarmRinger) Prepare(a alarm.Alarm, at time.Time) {
	if !r.spotifyAlarm(a) {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sess != nil {
		log.Printf("spotify alarm: not warming up %s: another alarm holds the device", a.TimeString())
		return
	}
	log.Printf("spotify alarm: warming up the %s alarm at volume 0, %s ahead", a.TimeString(), time.Until(at).Round(time.Second))
	r.sess = r.startSessionLocked(a)
}

// CancelPrepare ends a warm-up that will not ring after all. A session that
// has already rung is left alone: that is Stop's to end.
func (r *alarmRinger) CancelPrepare() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sess == nil || r.sess.rung {
		return
	}
	log.Printf("spotify alarm: warm-up for %s cancelled", r.sess.alarm.TimeString())
	r.endSessionLocked()
}

// Start makes the alarm audible: the warmed-up session if there is one for
// this alarm, a fresh one otherwise, and the tone whenever music is not yet
// confirmed. The tone is switched under r.mu, as the session does, so the two
// can never leave it ringing over the music.
func (r *alarmRinger) Start(a alarm.Alarm) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.sess != nil && (!r.spotifyAlarm(a) || r.sess.alarm.Sound.Ref != a.Sound.Ref) {
		r.endSessionLocked() // a different alarm takes over the device
	}
	if !r.spotifyAlarm(a) {
		r.audio.Start(a)
		return
	}
	if r.sess == nil {
		r.sess = r.startSessionLocked(a)
	}
	s := r.sess
	s.ringing, s.rung = true, true
	s.poke()
	if s.ready {
		log.Printf("spotify alarm: %s is due; bringing up the warmed-up playlist", a.TimeString())
		return
	}
	log.Printf("spotify alarm: %s is due but Spotify is not playing yet; ringing the tone meanwhile", a.TimeString())
	r.audio.Start(a)
}

// Snooze silences the alarm but keeps a Spotify session playing at volume 0,
// so ringing again is once more just a skip and a volume ramp.
func (r *alarmRinger) Snooze() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.audio.Stop()
	if r.sess != nil {
		r.sess.ringing = false
		r.sess.poke()
	}
}

// Stop silences everything: the tone, the radio, the alarm's session, and
// any music playing on our device.
func (r *alarmRinger) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.audio.Stop()
	r.endSessionLocked()
}

// startSessionLocked launches a session for a. Its first Play waits for the
// previous session to be silenced, or that session's Pause could land after
// it and leave the new alarm paused. The caller must hold r.mu.
func (r *alarmRinger) startSessionLocked(a alarm.Alarm) *session {
	ctx, cancel := context.WithCancel(context.Background())
	s := &session{
		alarm:  a,
		cancel: cancel,
		wake:   make(chan struct{}, 1),
		done:   make(chan struct{}),
	}
	go r.run(ctx, s, r.quiet)
	return s
}

// endSessionLocked cancels the current session, if any, and then — in the
// background, once it has stopped issuing commands — pauses our device and
// returns its volume to 0. That happens even without a session, so Stop also
// ends music started from the Spotify screen. The caller must hold r.mu.
func (r *alarmRinger) endSessionLocked() {
	s := r.sess
	r.sess = nil
	if s != nil {
		s.cancel()
	}
	if r.spot == nil || !r.spot.Authorized() {
		return
	}
	prev, quiet := r.quiet, make(chan struct{})
	r.quiet = quiet
	go func() {
		defer close(quiet)
		if prev != nil {
			<-prev
		}
		if s != nil {
			<-s.done
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		r.silence(ctx)
	}()
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

// run drives one session until it is cancelled: get the playlist playing
// silently (restarting librespot as often as that takes), then follow ringing
// — up when the alarm rings, back to 0 when it is snoozed.
func (r *alarmRinger) run(ctx context.Context, s *session, quiet <-chan struct{}) {
	defer close(s.done)
	if quiet != nil {
		select {
		case <-quiet:
		case <-ctx.Done():
			return
		}
	}

	var (
		id          string
		audible     bool // our volume is up (or ramping up)
		lastRestart time.Time
		silentSince = time.Now()
	)
	for ctx.Err() == nil {
		ringing, ready := r.state(s)

		if ringing {
			silentSince = time.Now()
		} else if time.Since(silentSince) >= silentMax {
			log.Printf("spotify alarm: %s has played silently for %s without ringing; ending it", s.alarm.TimeString(), silentMax)
			r.mu.Lock()
			if r.sess == s {
				r.endSessionLocked()
			}
			r.mu.Unlock()
			return
		}

		if !ready {
			var ok bool
			if id, ok = r.startSilently(ctx, s); !ok {
				if ctx.Err() != nil {
					return // stopped mid-attempt: leave librespot alone
				}
				// Either the device dropped off while idle, or it is a stale
				// entry on Spotify's list that accepts commands without
				// playing. A fresh librespot fixes both.
				if time.Since(lastRestart) >= librespotRecheck {
					log.Printf("spotify alarm: restarting librespot and trying again")
					r.lib.Restart()
					lastRestart = time.Now()
				}
				sleepCtx(ctx, retryInterval)
				continue
			}
			audible = false
			ringing = r.setReady(s, true)
		}

		switch {
		case ringing && !audible:
			if !r.reveal(ctx, s, id) {
				if ctx.Err() == nil {
					r.setReady(s, false) // rings the tone while we recover
				}
				continue
			}
			audible = true
		case !ringing && audible:
			if err := r.spot.SetVolume(ctx, id, 0); err != nil {
				log.Printf("spotify alarm: lowering the volume for the snooze failed: %v", err)
			}
			audible = false
		}

		var expire <-chan time.Time
		if !ringing {
			expire = time.After(silentMax - time.Since(silentSince))
		}
		select {
		case <-ctx.Done():
		case <-s.wake:
		case <-expire:
		}
		if ringing { // it rang right up to this wake-up
			silentSince = time.Now()
		}
	}
}

// state reads whether s should be ringing and whether its playlist is ready.
func (r *alarmRinger) state(s *session) (ringing, ready bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return s.ringing, s.ready
}

// setReady records whether s's playlist is playing. Losing it while the alarm
// rings brings the tone back. Returns whether s should be ringing.
func (r *alarmRinger) setReady(s *session, ready bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	s.ready = ready
	if !ready && s.ringing && r.sess == s {
		log.Printf("spotify alarm: lost the playlist while ringing; the tone rings until it is back")
		r.audio.Start(s.alarm)
	}
	return s.ringing
}

// startSilently starts the alarm's playlist on our device at volume 0 and
// waits for proof that it is really playing. Returns the device id.
func (r *alarmRinger) startSilently(ctx context.Context, s *session) (string, bool) {
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
	// (the caller's next step) brings it back at 0.
	if v := dev.VolumePercent; v == nil || *v != 0 {
		if err := r.spot.SetVolume(ctx, id, 0); err != nil {
			log.Printf("spotify alarm: %q is idle at a volume above 0 and refuses to go to 0 (%v)", r.device, err)
			return "", false
		}
	}
	if err := r.spot.Play(ctx, id, s.alarm.Sound.Ref, nil); err != nil {
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
	log.Printf("spotify alarm: %s is playing silently on %q", s.alarm.TimeString(), r.device)
	return id, true
}

// reveal makes a silently playing session audible: skip to a fresh track of
// the shuffled playlist, confirm it plays, silence the tone (and the radio),
// and ramp the volume up. A snooze or stop cuts the ramp short. Returns false
// if the music did not come back after the skip.
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

	r.mu.Lock()
	if s.ringing && r.sess == s {
		r.audio.Stop()
	}
	r.mu.Unlock()
	log.Printf("spotify alarm: ringing with music on %q, ramping up over %s", r.device, rampUp)

	for i := 1; i <= rampSteps; i++ {
		if !sleepCtx(ctx, rampUp/time.Duration(rampSteps)) {
			return true
		}
		if ringing, _ := r.state(s); !ringing {
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
// produces nothing at all. Returns false if the session ends or the deadline
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
