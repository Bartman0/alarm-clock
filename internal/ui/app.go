package ui

import (
	"fmt"
	"log"
	"strings"
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

	// fireGrace is how late a scheduled alarm may still ring. Firing is
	// level-triggered within this window, so a stalled tick or a clock step
	// across the alarm minute no longer loses the alarm. Past the window we
	// log the miss rather than wake you at an hour you did not ask for.
	fireGrace = 2 * time.Minute

	// prepareLead is how far ahead of an alarm the ringer is asked to make
	// sure it can actually make a sound. Long enough to restart a Spotify
	// Connect session and have it register again, with slack.
	prepareLead = 5 * time.Minute

	// heartbeatInterval is how often the scheduler proves it is alive in the
	// log; stallThreshold and stepThreshold are when a tick gap or a
	// wall-clock jump is worth recording.
	heartbeatInterval = time.Minute
	stallThreshold    = 5 * time.Second
	stepThreshold     = 2 * time.Second
)

// ringOp is a change to the ringer decided while holding a.mu and applied after
// releasing it. The ringer talks to mpv and the Spotify Web API, and a slow
// call there must never wedge the scheduler or the UI (Layout takes the same
// mutex, so a blocked ringer would freeze the display on its last frame).
type ringOp int

const (
	opNone ringOp = iota
	opStart
	opStop
	opPrepare
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
	btnStopAll widget.Clickable

	// Master volume slider (drives the PipeWire default sink).
	volume     widget.Float
	volInit    bool
	volApplied int

	// Alarms-list widgets.
	rows          [alarm.Count]alarmRow
	alarmsList    widget.List
	btnAlarmsBack widget.Clickable

	// Edit-screen widgets.
	editIdx    int
	draft      alarm.Alarm
	editEnable widget.Bool
	hourUp     widget.Clickable
	hourDown   widget.Clickable
	minUp      widget.Clickable
	minDown    widget.Clickable
	rhythmBtns [4]widget.Clickable
	soundBtns  [2]widget.Clickable
	editSave   widget.Clickable
	editCancel widget.Clickable

	// Firing state, evaluated by a background scheduler goroutine as well as
	// the UI; guarded by mu. ringingIdx >= 0 means an alarm is ringing (the UI
	// shows the firing screen regardless of cur). disableOnce is the index of a
	// fired Eenmalig alarm the UI goroutine should disable, or -1.
	mu           sync.Mutex
	ringingIdx   int
	ringStart    time.Time
	snoozeUntil  time.Time
	snoozeIdx    int
	lastFired    [alarm.Count]time.Time // the occurrence fired, per alarm
	lastPrepared [alarm.Count]time.Time // the occurrence warmed up, per alarm
	seeded       bool                   // past occurrences marked on the first tick
	disableOnce  int
	btnSnooze    widget.Clickable
	btnStop      widget.Clickable

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

	// Shared on-screen keyboard for the search fields.
	kbd *keyboard
}

type alarmRow struct {
	tap    widget.Clickable
	toggle widget.Bool
}

// NewApp builds the app from a loaded store and a ringer.
func NewApp(th *material.Theme, store *config.Store, ringer Ringer) *App {
	a := &App{
		th:          th,
		store:       store,
		ringer:      ringer,
		cur:         screenHome,
		editIdx:     -1,
		ringingIdx:  -1,
		disableOnce: -1,
		radioRows:   make([]widget.Clickable, maxRadioResults),
		spotRows:    make([]widget.Clickable, maxSpotItems),
		kbd:         newKeyboard(),
	}
	a.alarmsList.Axis = layout.Vertical
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

// Layout renders the current screen for the given wall-clock time. Alarm timing
// is handled by the background scheduler (see StartScheduler), not here.
func (a *App) Layout(gtx layout.Context, now time.Time) layout.Dimensions {
	a.now = now

	a.mu.Lock()
	ringing := a.ringingIdx
	once := a.disableOnce
	a.disableOnce = -1
	a.mu.Unlock()

	// Apply a fired Eenmalig alarm's self-disable here, on the UI goroutine,
	// which owns writes to the alarm store.
	if once >= 0 {
		log.Printf("alarm %d: disabling itself after firing (Eenmalig)", once)
		a.setAlarmEnabled(once, false)
		a.rows[once].toggle.Value = false
		a.save()
	}

	Fill(gtx, Mocha.Base)
	if ringing >= 0 { // an alarm is ringing: firing screen wins over cur
		return a.layoutFiring(gtx)
	}
	switch a.cur {
	case screenAlarms:
		return a.layoutAlarms(gtx)
	case screenEdit:
		return a.layoutEdit(gtx)
	case screenRadio:
		return a.layoutRadio(gtx)
	case screenSpotify:
		return a.layoutSpotify(gtx)
	default:
		return a.layoutHome(gtx)
	}
}

// StartScheduler runs alarm evaluation in its own goroutine, once a second, so
// alarms fire even when the UI isn't rendering (e.g. the display has blanked
// overnight, which stalls the Gio frame loop). Call once after SetInvalidate.
func (a *App) StartScheduler() {
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		prev := time.Now()
		var lastBeat time.Time
		for range t.C {
			now := time.Now()
			logTimeAnomaly(clockGaps(prev, now))
			prev = now
			a.evaluate(now)
			if now.Sub(lastBeat) >= heartbeatInterval {
				a.logHeartbeat(now)
				lastBeat = now
			}
		}
	}()
}

// clockGaps measures how much time passed between two ticks by each clock.
// Go's time.Time carries both readings: Sub uses the monotonic one, and
// Round(0) strips it so Sub falls back to the wall clock.
func clockGaps(prev, now time.Time) (mono, wall time.Duration) {
	return now.Sub(prev), now.Round(0).Sub(prev.Round(0))
}

// logTimeAnomaly records the two ways an alarm's firing window can pass
// unobserved: the scheduler stalling, and the wall clock jumping over it.
// The two gaps tell them apart — a stall shows in both, a clock step only in
// the wall clock. (The Pi 5's RTC needs a battery, so after a reboot without
// one the clock is wrong until systemd-timesyncd steps it.)
func logTimeAnomaly(mono, wall time.Duration) {
	if skew := wall - mono; skew > stepThreshold || skew < -stepThreshold {
		log.Printf("scheduler: WALL CLOCK STEPPED by %s (only %s really elapsed); any alarm in the skipped span was not observed",
			skew.Round(time.Millisecond), mono.Round(time.Millisecond))
	}
	if mono >= stallThreshold {
		log.Printf("scheduler: STALLED for %s between ticks", mono.Round(time.Millisecond))
	}
}

// logHeartbeat writes one line a minute: proof the scheduler is alive, the time
// it believes it is, and what it is waiting for. A gap in these lines is a
// stalled or dead process; a jump in them is a clock step.
func (a *App) logHeartbeat(now time.Time) {
	a.mu.Lock()
	ringing, snooze := a.ringingIdx, a.snoozeUntil
	pending := make([]string, 0, alarm.Count)
	for i := range a.store.Alarms {
		al := a.store.Alarms[i]
		if !al.Enabled {
			continue
		}
		if next, ok := al.Next(now); ok {
			pending = append(pending, fmt.Sprintf("%d=%s", i, next.Format("Mon 15:04")))
		}
	}
	a.mu.Unlock()

	next := "none enabled"
	if len(pending) > 0 {
		next = strings.Join(pending, " ")
	}
	state := "idle"
	if ringing >= 0 {
		state = fmt.Sprintf("ringing=%d", ringing)
	} else if !snooze.IsZero() {
		state = "snoozed until " + snooze.Format("15:04:05")
	}
	log.Printf("heartbeat: %s %s next[%s]", now.Format("2006-01-02 15:04:05 MST"), state, next)
}

// evaluate runs one scheduler step, applies the resulting ringer change outside
// the lock, and refreshes the UI if firing state changed.
func (a *App) evaluate(now time.Time) {
	a.mu.Lock()
	op, al := a.tickLocked(now)
	a.mu.Unlock()

	a.applyRingOp(op, al)
	if op != opNone && op != opPrepare && a.invalidate != nil {
		a.invalidate()
	}
}

// applyRingOp drives the ringer for a state change decided under a.mu. It must
// be called with the lock released.
func (a *App) applyRingOp(op ringOp, al alarm.Alarm) {
	switch op {
	case opStart:
		a.ringer.Start(al)
	case opStop:
		a.ringer.Stop()
	case opPrepare:
		if p, ok := a.ringer.(Preparer); ok {
			p.Prepare(al)
		}
	}
}

// tickLocked drives alarm firing, snooze wake-up and the ring time limit. The
// caller must hold a.mu; the returned operation must be applied after
// unlocking.
func (a *App) tickLocked(now time.Time) (ringOp, alarm.Alarm) {
	a.seedLastFiredLocked(now)

	if a.ringingIdx >= 0 {
		if now.Sub(a.ringStart) >= maxRingDuration {
			log.Printf("alarm %d: auto-stopping after %s", a.ringingIdx, maxRingDuration)
			return a.stopRingingLocked(), alarm.Alarm{}
		}
		return opNone, alarm.Alarm{}
	}

	// Wake from snooze.
	if !a.snoozeUntil.IsZero() && !now.Before(a.snoozeUntil) {
		a.snoozeUntil = time.Time{}
		log.Printf("alarm %d: snooze elapsed, ringing again", a.snoozeIdx)
		return a.startRingingLocked(a.snoozeIdx, now)
	}

	// Fire a scheduled alarm. Firing is level-triggered: any tick inside
	// fireGrace of the scheduled moment fires it, and what we record as fired
	// is the occurrence itself, not the wall-clock minute. Matching an exact
	// minute instead would need a tick to land in one specific 60-second
	// window, so a stall or a clock step across it lost the alarm silently.
	for i := range a.store.Alarms {
		al := a.store.Alarms[i]
		if !al.Enabled {
			continue
		}
		sched := occurrence(al, now)
		if !al.Rhythm.Active(sched.Weekday()) || a.lastFired[i].Equal(sched) {
			continue
		}
		late := now.Sub(sched)
		if late < 0 {
			continue // not due yet today
		}
		a.lastFired[i] = sched // handled either way: fired, or recorded as missed
		if late >= fireGrace {
			log.Printf("alarm %d: MISSED occurrence %s — noticed %s late (grace %s), not ringing now",
				i, sched.Format("2006-01-02 15:04"), late.Round(time.Second), fireGrace)
			continue
		}
		log.Printf("alarm %d: firing occurrence %s (%s late) rhythm=%s sound=%s",
			i, sched.Format("2006-01-02 15:04"), late.Round(time.Second), al.Rhythm, al.Sound.Kind)
		// A one-time (Eenmalig) alarm disables itself after firing; the UI
		// goroutine applies the store write (see Layout).
		if al.Rhythm == alarm.Once {
			a.disableOnce = i
		}
		return a.startRingingLocked(i, now)
	}

	return a.prepareLocked(now)
}

// prepareLocked asks the ringer, once per occurrence, to make sure it will be
// able to sound an alarm that is prepareLead away. The caller must hold a.mu.
func (a *App) prepareLocked(now time.Time) (ringOp, alarm.Alarm) {
	for i := range a.store.Alarms {
		al := a.store.Alarms[i]
		if !al.Enabled {
			continue
		}
		sched := occurrence(al, now)
		if !sched.After(now) { // today's has passed; look at tomorrow's
			sched = sched.AddDate(0, 0, 1)
		}
		if !al.Rhythm.Active(sched.Weekday()) || a.lastPrepared[i].Equal(sched) {
			continue
		}
		if sched.Sub(now) > prepareLead {
			continue
		}
		a.lastPrepared[i] = sched
		return opPrepare, al
	}
	return opNone, alarm.Alarm{}
}

// occurrence is the alarm's scheduled moment on now's calendar day.
func occurrence(al alarm.Alarm, now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day(), al.Hour, al.Minute, 0, 0, now.Location())
}

// seedLastFiredLocked marks today's long-past occurrences as handled on the
// first tick, so starting the app in the afternoon doesn't report every morning
// alarm as missed. Occurrences still inside fireGrace are left alone: an alarm
// due a minute ago should still ring.
func (a *App) seedLastFiredLocked(now time.Time) {
	if a.seeded {
		return
	}
	a.seeded = true
	for i := range a.store.Alarms {
		if sched := occurrence(a.store.Alarms[i], now); now.Sub(sched) >= fireGrace {
			a.lastFired[i] = sched
		}
	}
}

// startRingingLocked, stopRingingLocked and snoozeLocked mutate firing state
// only. The caller must hold a.mu and apply the returned operation after
// unlocking — see applyRingOp.
func (a *App) startRingingLocked(i int, now time.Time) (ringOp, alarm.Alarm) {
	a.ringingIdx = i
	a.ringStart = now
	return opStart, a.store.Alarms[i]
}

func (a *App) stopRingingLocked() ringOp {
	op := opNone
	if a.ringingIdx >= 0 {
		op = opStop
	}
	a.ringingIdx = -1
	a.snoozeUntil = time.Time{}
	return op
}

func (a *App) snoozeLocked(now time.Time) ringOp {
	if a.ringingIdx < 0 {
		return opNone
	}
	log.Printf("alarm %d: snoozing for %s", a.ringingIdx, snoozeDuration)
	a.snoozeIdx = a.ringingIdx
	a.snoozeUntil = now.Add(snoozeDuration)
	a.ringingIdx = -1
	return opStop
}

// stopRinging and snooze are the UI-facing entry points for the firing screen:
// they take the lock, change state, release it, and only then touch the ringer.
func (a *App) stopRinging() {
	a.mu.Lock()
	op := a.stopRingingLocked()
	a.mu.Unlock()
	if op == opStop {
		log.Printf("alarm stopped from the firing screen")
	}
	a.applyRingOp(op, alarm.Alarm{})
}

func (a *App) snooze(now time.Time) {
	a.mu.Lock()
	op := a.snoozeLocked(now)
	a.mu.Unlock()
	a.applyRingOp(op, alarm.Alarm{})
}

// stopAll silences everything from the home screen: it clears any ringing /
// pending-snooze state and stops the alarm tone + Spotify (via the ringer),
// the radio, and any interactive Spotify playback. Safe to call when nothing
// is playing.
func (a *App) stopAll() {
	a.mu.Lock()
	a.ringingIdx = -1
	a.snoozeUntil = time.Time{}
	a.mu.Unlock()

	a.ringer.Stop() // alarm tone + Spotify pause + cancel any in-flight attempt
	if a.radio != nil {
		a.radio.StopStream()
	}
	a.nowPlaying = ""
}

// alarmAt returns a copy of alarm i. The scheduler goroutine reads and writes
// the store under a.mu, so every UI-side read goes through here.
func (a *App) alarmAt(i int) alarm.Alarm {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.store.Alarms[i]
}

// setAlarm replaces alarm i, and setAlarmEnabled flips its on/off state.
func (a *App) setAlarm(i int, al alarm.Alarm) {
	a.mu.Lock()
	a.store.Alarms[i] = al
	a.mu.Unlock()
}

func (a *App) setAlarmEnabled(i int, on bool) {
	a.mu.Lock()
	a.store.Alarms[i].Enabled = on
	a.mu.Unlock()
}

func (a *App) save() {
	a.mu.Lock()
	err := a.store.Save()
	a.mu.Unlock()
	if err != nil {
		log.Printf("save config: %v", err)
	}
}

// nextAlarmText summarises the soonest upcoming alarm for the home screen.
func (a *App) nextAlarmText(now time.Time) string {
	alarms := a.alarmsSnapshot()
	var soonest time.Time
	found := false
	for i := range alarms {
		if t, ok := alarms[i].Next(now); ok && (!found || t.Before(soonest)) {
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

// alarmsSnapshot copies the alarms under the lock for read-only UI use.
func (a *App) alarmsSnapshot() [alarm.Count]alarm.Alarm {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.store.Alarms
}
