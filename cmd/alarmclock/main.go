// Command alarmclock is a touchscreen alarm clock for the Raspberry Pi 5 with
// the official Touch Display 2 (1280x720 landscape).
package main

import (
	"context"
	"log"
	"os"
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
		if os.Getenv("ALARMCLOCK_WINDOWED") == "" {
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

	ringer := &alarmRinger{audio: controller, spot: spot, device: deviceName}

	application := ui.NewApp(ui.NewTheme(), store, ringer)
	application.SetRadio(controller)
	application.SetSpotify(spot, deviceName)
	application.SetInvalidate(w.Invalidate)

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

// alarmRinger plays the right sound for a firing alarm: a Spotify context when
// the alarm is configured for Spotify (falling back to the alarm tone on any
// failure), otherwise the mpv-backed alarm tone. It satisfies ui.Ringer.
type alarmRinger struct {
	audio  *audio.Controller
	spot   *spotify.Client
	device string
}

func (r *alarmRinger) Start(a alarm.Alarm) {
	if a.Sound.Kind == alarm.SoundSpotify && a.Sound.Ref != "" && r.spot.Authorized() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := r.spot.PlayOnDevice(ctx, r.device, a.Sound.Ref, nil); err != nil {
				log.Printf("spotify alarm failed (%v); falling back to alarm sound", err)
				r.audio.Start(a)
			}
		}()
		return
	}
	r.audio.Start(a)
}

func (r *alarmRinger) Stop() {
	r.audio.Stop()
	if r.spot.Authorized() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			_ = r.spot.Pause(ctx)
		}()
	}
}
