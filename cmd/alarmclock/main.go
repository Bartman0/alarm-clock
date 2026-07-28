// Command alarmclock is a touchscreen alarm clock for the Raspberry Pi 5 with
// the official Touch Display 2 (1280x720 landscape).
package main

import (
	"context"
	"log"
	"os"
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
		ctx, tcancel := context.WithTimeout(ctx, 90*time.Second)
		defer tcancel()

		play := func() bool {
			if err := r.spot.PlayOnDevice(ctx, r.device, a.Sound.Ref, nil); err != nil {
				log.Printf("spotify alarm: play failed: %v", err)
				return false
			}
			r.audio.Stop() // music is playing; silence the tone
			return true
		}

		if play() {
			return
		}
		// Device likely dropped off while idle overnight; restart librespot to
		// re-register it, then poll until it reappears.
		log.Printf("spotify alarm: device %q unavailable, restarting librespot", r.device)
		r.lib.Restart()
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
			if _, ok, _ := r.spot.DeviceIDByName(ctx, r.device); ok && play() {
				return
			}
		}
	}()
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
