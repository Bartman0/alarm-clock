// Command alarmclock is a touchscreen alarm clock for the Raspberry Pi 5 with
// the official Touch Display 2 (1280x720 landscape).
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/op"
	"gioui.org/unit"

	"alarmclock/internal/alarm"
	"alarmclock/internal/audio"
	"alarmclock/internal/config"
	"alarmclock/internal/librespot"
	"alarmclock/internal/spotify"
	"alarmclock/internal/ui"
)

// deviceName is how the Pi advertises itself as a Spotify Connect device.
const deviceName = "Wekker"

const (
	// playbackConfirm is how long we wait for the Connect device to really
	// start rendering audio, and playbackPoll how often we check. Spotify
	// accepting a Play command proves nothing: librespot still has to take
	// over playback, and player commands issued before it does come back
	// "Restriction violated".
	playbackConfirm = 20 * time.Second
	playbackPoll    = time.Second

	// playbackBudget is how long we keep trying to get music playing before
	// giving up. The alarm tone rings throughout, so there is no reason to
	// stop early: librespot may need a restart and a re-registration with
	// Spotify, which can take far longer than one attempt.
	playbackBudget = 5 * time.Minute
	retryInterval  = 3 * time.Second

	// librespotRecheck is how long we give librespot to reappear on Spotify's
	// device list after a restart before concluding the restart did not take.
	librespotRecheck = 45 * time.Second
)

func main() {
	go func() {
		w := new(app.Window)
		w.Option(
			app.Title("Alarm Clock"),
			app.Size(unit.Dp(1280), unit.Dp(720)),
		)
		// Default to a normal window: under the sway kiosk the borderless
		// tiled window already fills the screen, and Gio's Wayland fullscreen
		// path fails to size the GL surface in time (wl_egl_window_create with
		// a 0x0 size). Opt into Gio fullscreen with ALARMCLOCK_FULLSCREEN=1.
		if os.Getenv("ALARMCLOCK_FULLSCREEN") != "" {
			w.Option(app.Fullscreen.Option())
		}
		if err := run(w); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

func run(w *app.Window) error {
	store, err := config.Load()
	if err != nil {
		log.Printf("loading config: %v (using defaults)", err)
	}

	controller := audio.NewController()
	defer controller.Close()

	// Spotify: client ID from config, overridable by env for convenience.
	clientID := store.Spotify.ClientID
	if env := os.Getenv("ALARMCLOCK_SPOTIFY_CLIENT_ID"); env != "" {
		clientID = env
	}
	spot := spotify.New(spotify.Config{ClientID: clientID}, store.Spotify.Tokens, func(t spotify.Tokens) {
		store.Spotify.Tokens = t
		if err := store.Save(); err != nil {
			log.Printf("saving spotify tokens: %v", err)
		}
	})

	// librespot makes the Pi a Connect device we can target via the Web API.
	lib := librespot.New(deviceName)
	lib.Start()
	defer lib.Stop()

	ringer := &alarmRinger{audio: controller, spot: spot, lib: lib, device: deviceName}

	application := ui.NewApp(ui.NewTheme(), store, ringer)
	application.SetRadio(controller)
	application.SetSpotify(spot, deviceName)
	application.SetInvalidate(w.Invalidate)
	application.StartScheduler() // evaluate alarms independently of rendering

	// Redraw once a second so the clock stays current and alarms are evaluated.
	go func() {
		for range time.Tick(time.Second) {
			w.Invalidate()
		}
	}()

	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			application.Layout(gtx, time.Now())
			e.Frame(gtx.Ops)
		}
	}
}

// alarmRinger sounds a firing alarm. It always starts the mpv alarm tone
// immediately so the alarm reliably wakes you; for a Spotify alarm it then
// tries, in the background, to play the chosen playlist on the librespot
// device — restarting librespot if the device has dropped off Spotify's list
// while idle — and silences the tone once music is playing. It satisfies
// ui.Ringer.
type alarmRinger struct {
	audio  *audio.Controller
	spot   *spotify.Client
	lib    *librespot.Supervisor
	device string

	mu     sync.Mutex
	cancel context.CancelFunc
}

func (r *alarmRinger) Start(a alarm.Alarm) {
	// Tone first — this always wakes you, even if Spotify is unreachable.
	r.audio.Start(a)

	if a.Sound.Kind != alarm.SoundSpotify || a.Sound.Ref == "" || r.spot == nil || !r.spot.Authorized() {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	if r.cancel != nil {
		r.cancel() // cancel any previous attempt
	}
	r.cancel = cancel
	r.mu.Unlock()

	go func() {
		defer cancel()
		ctx, tcancel := context.WithTimeout(ctx, playbackBudget)
		defer tcancel()

		// If the alarm is dismissed while this goroutine is starting playback,
		// pause here on the way out. This runs after our own Play, so it wins
		// the race against Stop's pause (which may land before Play does).
		defer func() {
			if ctx.Err() == context.Canceled {
				pctx, pcancel := context.WithTimeout(context.Background(), 8*time.Second)
				defer pcancel()
				_ = r.spot.Pause(pctx)
			}
		}()

		r.logDevices(ctx)

		play := func() bool {
			id, ok, err := r.spot.DeviceIDByName(ctx, r.device)
			if err != nil || !ok {
				return false
			}
			if err := r.spot.Play(ctx, id, a.Sound.Ref, nil); err != nil {
				log.Printf("spotify alarm: play failed: %v", err)
				return false
			}
			// Accepting Play is not the same as making a sound. Wait for the
			// device to actually be playing before issuing player commands or
			// touching the tone.
			if !r.playbackConfirmed(ctx, id, playbackConfirm) {
				log.Printf("spotify alarm: %q accepted Play but never started playing", r.device)
				return false
			}
			r.randomizeStart(ctx, id)
			return true
		}

		if play() {
			return
		}
		// Either the device dropped off while idle overnight, or it is a
		// stale entry on Spotify's list that accepts commands without playing
		// anything. Restart librespot and keep trying: the tone is ringing
		// throughout, so the only thing giving up early would achieve is a
		// silent bedroom once you hit Stop.
		log.Printf("spotify alarm: restarting librespot and retrying for up to %s", playbackBudget)
		r.lib.Restart()
		lastRestart := time.Now()

		for attempt := 1; ; attempt++ {
			select {
			case <-ctx.Done():
				log.Printf("spotify alarm: gave up after %s without confirmed playback; the alarm tone is still ringing", playbackBudget)
				return
			case <-time.After(retryInterval):
			}

			_, ok, err := r.spot.DeviceIDByName(ctx, r.device)
			switch {
			case err != nil:
				log.Printf("spotify alarm: device lookup failed: %v", err)
			case !ok:
				// librespot has not come back yet. If it has had long enough,
				// the restart did not take — kill it again.
				if time.Since(lastRestart) >= librespotRecheck {
					log.Printf("spotify alarm: %q still not registered %s after the restart, restarting librespot again", r.device, librespotRecheck)
					r.lib.Restart()
					lastRestart = time.Now()
				}
			default:
				log.Printf("spotify alarm: %q is back on the device list, retrying playback (attempt %d)", r.device, attempt)
				if play() {
					return
				}
			}
		}
	}()
}

// randomizeStart enables shuffle and skips off the deterministic first track of
// a just-started playlist, so the alarm doesn't always start on the same song.
// The alarm tone keeps playing to mask the brief first-track blip, and is
// silenced only once music is confirmed to be playing again after the skip.
// Bails out early if the alarm is dismissed (ctx cancelled).
func (r *alarmRinger) randomizeStart(ctx context.Context, deviceID string) {
	if err := r.spot.Shuffle(ctx, deviceID, true); err != nil {
		log.Printf("spotify alarm: shuffle failed: %v", err)
	}
	if err := r.spot.Next(ctx, deviceID); err != nil {
		log.Printf("spotify alarm: next failed: %v", err)
	}
	// Hand over only against evidence that music is actually playing. The
	// tone is the thing that wakes you; silencing it on an assumption is how
	// an alarm ends up showing its firing screen in total silence.
	if !r.playbackConfirmed(ctx, deviceID, playbackConfirm) {
		log.Printf("spotify alarm: playback not confirmed after the skip; keeping the alarm tone")
		return
	}
	log.Printf("spotify alarm: playback confirmed on %q; silencing the alarm tone", r.device)
	r.audio.Stop()
}

// playbackConfirmed reports whether deviceID is genuinely rendering audio. The
// player must say it is playing on that device AND its track position must
// advance between two reads: is_playing alone can be true while the device
// produces nothing at all. Returns false if the alarm is dismissed or the
// deadline passes first.
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

// logDevices records what Spotify can see when an alarm tries to play, so a
// failure says whether the Pi was even on the device list and what else was
// holding playback.
func (r *alarmRinger) logDevices(ctx context.Context) {
	devs, err := r.spot.Devices(ctx)
	if err != nil {
		log.Printf("spotify alarm: device list unavailable: %v", err)
		return
	}
	if len(devs) == 0 {
		log.Printf("spotify alarm: Spotify sees no Connect devices at all")
		return
	}
	seen := make([]string, 0, len(devs))
	for _, d := range devs {
		seen = append(seen, fmt.Sprintf("%q(active=%t)", d.Name, d.IsActive))
	}
	log.Printf("spotify alarm: devices visible: %s", strings.Join(seen, " "))
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

func (r *alarmRinger) Stop() {
	r.mu.Lock()
	if r.cancel != nil {
		r.cancel() // stop any in-flight Spotify attempt so music can't start after Stop
		r.cancel = nil
	}
	r.mu.Unlock()

	r.audio.Stop()
	if r.spot != nil && r.spot.Authorized() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			_ = r.spot.Pause(ctx)
		}()
	}
}
