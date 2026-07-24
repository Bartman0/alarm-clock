package ui

import (
	"log"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"alarmclock/internal/alarm"
	"alarmclock/internal/clock"
	"alarmclock/internal/config"
)

type screen int

const (
	screenHome screen = iota
	screenAlarms
	screenEdit
	screenFiring
)

const (
	snoozeDuration = 5 * time.Minute
	maxRingDuration = 15 * time.Minute
	minuteGuard     = 30 * time.Second // avoid re-firing within the same minute
)

// App owns all screen state and drives navigation, alarm timing and layout.
// Every screen lives as a method on App (across the ui package's files) and
// mutates this shared state; the Gio main loop only calls Layout each frame.
type App struct {
	th     *material.Theme
	store  *config.Store
	ringer Ringer

	cur screen
	now time.Time // set each frame for use by click handlers

	// Home widgets.
	btnAlarms  widget.Clickable
	btnRadio   widget.Clickable
	btnSpotify widget.Clickable

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
	}
	for i := range a.rows {
		a.rows[i].toggle.Value = store.Alarms[i].Enabled
	}
	return a
}

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
		if al.Hour == now.Hour() && al.Minute == now.Minute() && now.Sub(a.lastFired[i]) > minuteGuard {
			a.lastFired[i] = now
			a.startRinging(i, now)
			return
		}
	}
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
