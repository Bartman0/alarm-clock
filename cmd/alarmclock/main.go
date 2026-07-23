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

	"alarmclock/internal/ui"
)

func main() {
	go func() {
		w := new(app.Window)
		w.Option(
			app.Title("Alarm Clock"),
			app.Size(unit.Dp(1280), unit.Dp(720)),
		)
		// Kiosk fullscreen is enabled on the device; keep it windowed when a
		// dev flag is set so it is comfortable to iterate on a desktop.
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
	th := ui.NewTheme()
	home := &ui.Home{}

	// Redraw once a second so the clock stays current.
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
			home.Layout(gtx, th, time.Now())
			e.Frame(gtx.Ops)
		}
	}
}
