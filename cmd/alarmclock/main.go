// Command alarmclock is a touchscreen alarm clock for the Raspberry Pi 5 with
// the official Touch Display 2 (1280x720 landscape).
package main

import (
	"log"
	"os"
	"time"

	"gioui.org/app"
	"gioui.org/op"
	"gioui.org/unit"

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
