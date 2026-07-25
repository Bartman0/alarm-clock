package ui

import (
	"log"
	"sync"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"alarmclock/internal/alarm"
	"alarmclock/internal/clock"
	"alarmclock/internal/config"
	"alarmclock/internal/radio"
	"alarmclock/internal/spotify"
)

// maxRadioResults / maxSpotItems bound how many rows we fetch/display and size
// the pre-allocated per-row clickables.
const (
	maxRadioResults = 60
	maxSpotItems    = 50
)

type screen int

const (
	screenHome screen = iota
	screenAlarms
	screenEdit
	screenFiring
	screenRadio
	screenSpotify
)

const (
	snoozeDuration  = 5 * time.Minute
	maxRingDuration = 15 * time.Minute
)

// App owns all screen state and drives navigation, alarm timing and layout.
// Every screen lives as a method on App (across the ui package's files) and
// mutates this shared state; the Gio main loop only calls Layout each frame.
type App struct {
	th     *material.Theme
	store  *config.Store
	ringer Ringer
	radio  RadioPlayer

	// invalidate asks the window to redraw; set by main so async fetches can
	// refresh the UI promptly (nil in tests).
	invalidate func()

	cur screen
	now time.Time // set each frame for use by click handlers

	// Home widgets.
	btnAlarms  widget.Clickable
	btnRadio   widget.Clickable
	btnSpotify widget.Clickable

	// Master volume slider (drives the PipeWire default sink).
	volume     widget.Float
	volInit    bool
	volApplied int

	// Alarms-list widgets.
	rows          [3]alarmRow
	btnAlarmsBack widget.Clickable

	// Edit-screen widgets.
	editIdx    int
	draft      alarm.Alarm
	editEnable widget.Bool
	hourUp     widget.Clickable
	hourDown   widget.Clickable
	minUp      widget.Clickable
	minDown    widget.Clickable
	rhythmBtns [3]widget.Clickable
	soundBtns  [2]widget.Clickable
	editSave   widget.Clickable
	editCancel widget.Clickable

	// Firing state.
	ringingIdx  int
	ringStart   time.Time
	snoozeUntil time.Time
	snoozeIdx   int
	lastFired   [3]time.Time
	btnSnooze   widget.Clickable
	btnStop     widget.Clickable

	// Radio screen.
	radioBack    widget.Clickable
	radioSearch  widget.Clickable
	radioStop    widget.Clickable
	radioQuery   widget.Editor
	radioList    widget.List
	radioRows    []widget.Clickable
	radioClient  *radio.Client
	radioMu      sync.Mutex
	radioResults []radio.Station
	radioLoading bool
	radioErr     string
	nowPlaying   string

	// Spotify screen.
	spot            *spotify.Client
	spotDevice      string
	spotBack        widget.Clickable
	spotConnect     widget.Clickable
	spotPause       widget.Clickable
	spotTabSearch   widget.Clickable
	spotTabLibrary  widget.Clickable
	spotSearchBtn   widget.Clickable
	spotQuery       widget.Editor
	spotList        widget.List
	spotRows        []widget.Clickable
	spotTab         int  // 0 = search artists, 1 = library playlists
	spotPick        bool // selecting a playlist for an alarm instead of playing
	spotMu          sync.Mutex
	spotArtists     []spotify.Artist
	spotPlaylists   []spotify.Playlist
	spotLoading     bool
	spotAuthorizing bool
	spotErr         string
	spotStatus      string

	// Editor: pick-a-Spotify-playlist button.
	editPick widget.Clickable
}

type alarmRow struct {
	tap    widget.Clickable
	toggle widget.Bool
}

// NewApp builds the app from a loaded store and a ringer.
func NewApp(th *material.Theme, store *config.Store, ringer Ringer) *App {
	a := &App{
		th:         th,
		store:      store,
		ringer:     ringer,
		cur:        screenHome,
		editIdx:    -1,
		ringingIdx: -1,
		radioRows:  make([]widget.Clickable, maxRadioResults),
		spotRows:   make([]widget.Clickable, maxSpotItems),
	}
	a.radioList.Axis = layout.Vertical
	a.radioQuery.SingleLine = true
	a.radioQuery.Submit = true
	a.spotList.Axis = layout.Vertical
	a.spotQuery.SingleLine = true
	a.spotQuery.Submit = true
	for i := range a.rows {
		a.rows[i].toggle.Value = store.Alarms[i].Enabled
	}
	return a
}

// SetRadio wires the radio player (called by main after construction).
func (a *App) SetRadio(rp RadioPlayer) { a.radio = rp }

// SetSpotify wires the Spotify client and the Connect device name.
func (a *App) SetSpotify(c *spotify.Client, deviceName string) {
	a.spot = c
	a.spotDevice = deviceName
}

// SetInvalidate sets the redraw callback used to refresh after async fetches.
func (a *App) SetInvalidate(fn func()) { a.invalidate = fn }

// Layout renders the current screen for the given wall-clock time.
func (a *App) Layout(gtx layout.Context, now time.Time) layout.Dimensions {
	a.now = now
	a.tick(now)

	Fill(gtx, Mocha.Base)
	switch a.cur {
	case screenAlarms:
		return a.layoutAlarms(gtx)
	case screenEdit:
		return a.layoutEdit(gtx)
	case screenFiring:
		return a.layoutFiring(gtx)
	case screenRadio:
		return a.layoutRadio(gtx)
	case screenSpotify:
		return a.layoutSpotify(gtx)
	default:
		return a.layoutHome(gtx)
	}
}

// tick drives alarm firing, snooze wake-up and the ring time limit. It runs
// every frame (the app invalidates once a second) at one-minute granularity.
func (a *App) tick(now time.Time) {
	if a.ringingIdx >= 0 {
		if now.Sub(a.ringStart) >= maxRingDuration {
			a.stopRinging()
		}
		return
	}

	// Wake from snooze.
	if !a.snoozeUntil.IsZero() && !now.Before(a.snoozeUntil) {
		a.snoozeUntil = time.Time{}
		a.startRinging(a.snoozeIdx, now)
		return
	}

	// Fire a scheduled alarm.
	for i := range a.store.Alarms {
		al := a.store.Alarms[i]
		if !al.Enabled || !al.Rhythm.Active(now.Weekday()) {
			continue
		}
		// Fire at most once per clock-minute: skip if we already fired during
		// the current minute (guards against snooze being overridden by a
		// second same-minute trigger).
		if al.Hour == now.Hour() && al.Minute == now.Minute() && !sameMinute(a.lastFired[i], now) {
			a.lastFired[i] = now
			a.startRinging(i, now)
			return
		}
	}
}

// sameMinute reports whether two times fall in the same minute-of-day.
func sameMinute(a, b time.Time) bool {
	return a.Truncate(time.Minute).Equal(b.Truncate(time.Minute))
}

func (a *App) startRinging(i int, now time.Time) {
	a.ringingIdx = i
	a.ringStart = now
	a.cur = screenFiring
	a.ringer.Start(a.store.Alarms[i])
}

func (a *App) stopRinging() {
	if a.ringingIdx >= 0 {
		a.ringer.Stop()
	}
	a.ringingIdx = -1
	a.snoozeUntil = time.Time{}
	a.cur = screenHome
}

func (a *App) snooze() {
	if a.ringingIdx < 0 {
		return
	}
	a.ringer.Stop()
	a.snoozeIdx = a.ringingIdx
	a.snoozeUntil = a.now.Add(snoozeDuration)
	a.ringingIdx = -1
	a.cur = screenHome
}

func (a *App) save() {
	if err := a.store.Save(); err != nil {
		log.Printf("save config: %v", err)
	}
}

// nextAlarmText summarises the soonest upcoming alarm for the home screen.
func (a *App) nextAlarmText(now time.Time) string {
	var soonest time.Time
	found := false
	for i := range a.store.Alarms {
		if t, ok := a.store.Alarms[i].Next(now); ok && (!found || t.Before(soonest)) {
			soonest, found = t, true
		}
	}
	if !found {
		return "Geen alarm ingesteld"
	}
	day := "vandaag"
	switch soonest.YearDay() - now.YearDay() {
	case 0:
		day = "vandaag"
	case 1:
		day = "morgen"
	default:
		day = clock.Weekday(soonest)
	}
	return "Volgend alarm: " + clock.Time(soonest) + " " + day
}
