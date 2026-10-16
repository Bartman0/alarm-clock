package ui

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"alarmclock/internal/alarm"
	"alarmclock/internal/config"
)

type fakeRinger struct {
	started, snoozed, stopped, cancelled int
	last                                 alarm.Alarm
	prepared                             []alarm.Alarm
	preparedAt                           []time.Time
}

func (f *fakeRinger) Start(a alarm.Alarm) { f.started++; f.last = a }
func (f *fakeRinger) Snooze()             { f.snoozed++ }
func (f *fakeRinger) Stop()               { f.stopped++ }
func (f *fakeRinger) CancelPrepare()      { f.cancelled++ }
func (f *fakeRinger) Prepare(a alarm.Alarm, at time.Time) {
	f.prepared = append(f.prepared, a)
	f.preparedAt = append(f.preparedAt, at)
}

func newTestApp(al alarm.Alarm) (*App, *fakeRinger) {
	store := &config.Store{}
	store.Alarms[0] = al
	r := &fakeRinger{}
	return NewApp(NewTheme(), store, r), r
}

func (a *App) ringing() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.ringingIdx >= 0
}

func TestFiresAtMatchingMinute(t *testing.T) {
	now := time.Date(2026, 7, 22, 7, 0, 5, 0, time.UTC)
	app, r := newTestApp(alarm.Alarm{Enabled: true, Hour: 7, Minute: 0, Rhythm: alarm.FullWeek})

	app.evaluate(now)

	if !app.ringing() {
		t.Fatal("expected the alarm to be ringing")
	}
	if r.started != 1 {
		t.Fatalf("ringer started %d times, want 1", r.started)
	}
}

func TestDoesNotRefireSameMinute(t *testing.T) {
	now := time.Date(2026, 7, 22, 7, 0, 0, 0, time.UTC)
	app, r := newTestApp(alarm.Alarm{Enabled: true, Hour: 7, Minute: 0, Rhythm: alarm.FullWeek})

	app.evaluate(now)                       // fires
	app.stopRinging()                       // user stops
	app.evaluate(now.Add(10 * time.Second)) // still same minute

	if r.started != 1 {
		t.Fatalf("ringer started %d times, want 1 (guarded within the minute)", r.started)
	}
}

func TestSnoozeReArmsAfterFiveMinutes(t *testing.T) {
	now := time.Date(2026, 7, 22, 7, 0, 0, 0, time.UTC)
	app, r := newTestApp(alarm.Alarm{Enabled: true, Hour: 7, Minute: 0, Rhythm: alarm.FullWeek})

	app.evaluate(now) // fires
	app.snooze(now)   // snooze from firing screen

	if app.ringing() {
		t.Fatal("after snooze the alarm should not be ringing")
	}

	// Not yet: 4 minutes later nothing happens.
	app.evaluate(now.Add(4 * time.Minute))
	if app.ringing() {
		t.Fatal("alarm re-fired before the 5-minute snooze elapsed")
	}

	// After 5 minutes it rings again.
	app.evaluate(now.Add(5 * time.Minute))
	if !app.ringing() || r.started != 2 {
		t.Fatalf("snooze did not re-fire: ringing=%v started=%d", app.ringing(), r.started)
	}
}

// Regression: snoozing partway through the firing minute must not be
// overridden by a second trigger later in that same minute.
func TestSnoozeNotOverriddenLaterInSameMinute(t *testing.T) {
	fire := time.Date(2026, 7, 22, 7, 0, 3, 0, time.UTC)
	app, r := newTestApp(alarm.Alarm{Enabled: true, Hour: 7, Minute: 0, Rhythm: alarm.FullWeek})

	app.evaluate(fire) // fires at 07:00:03
	app.snooze(time.Date(2026, 7, 22, 7, 0, 10, 0, time.UTC))

	// Later in the SAME minute (07:00:40) the alarm must stay silent.
	app.evaluate(time.Date(2026, 7, 22, 7, 0, 40, 0, time.UTC))

	if app.ringing() || r.started != 1 {
		t.Fatalf("alarm re-fired within the same minute after snooze: ringing=%v started=%d", app.ringing(), r.started)
	}
}

func TestOnceAlarmQueuesSelfDisableAfterFiring(t *testing.T) {
	now := time.Date(2026, 7, 22, 7, 0, 0, 0, time.UTC)
	app, r := newTestApp(alarm.Alarm{Enabled: true, Hour: 7, Minute: 0, Rhythm: alarm.Once})

	app.evaluate(now)

	if r.started != 1 || !app.ringing() {
		t.Fatalf("Once alarm did not fire: started=%d ringing=%v", r.started, app.ringing())
	}
	app.mu.Lock()
	queued := app.disableOnce
	app.mu.Unlock()
	if queued != 0 {
		t.Fatalf("Once alarm should be queued for self-disable (disableOnce=%d, want 0)", queued)
	}
}

type fakeRadio struct{ stopped int }

func (f *fakeRadio) PlayStream(string) {}
func (f *fakeRadio) StopStream()       { f.stopped++ }

func TestStopAllSilencesEverything(t *testing.T) {
	now := time.Date(2026, 7, 22, 7, 0, 0, 0, time.UTC)
	app, r := newTestApp(alarm.Alarm{Enabled: true, Hour: 7, Minute: 0, Rhythm: alarm.FullWeek})
	radio := &fakeRadio{}
	app.SetRadio(radio)

	app.evaluate(now) // ringing
	app.stopAll()

	if app.ringing() {
		t.Fatal("stopAll should clear ringing state")
	}
	if r.stopped == 0 {
		t.Fatal("stopAll should stop the alarm ringer")
	}
	if radio.stopped == 0 {
		t.Fatal("stopAll should stop the radio")
	}
}

func TestDisabledAlarmDoesNotFire(t *testing.T) {
	now := time.Date(2026, 7, 22, 7, 0, 0, 0, time.UTC)
	app, r := newTestApp(alarm.Alarm{Enabled: false, Hour: 7, Minute: 0, Rhythm: alarm.FullWeek})

	app.evaluate(now)

	if r.started != 0 || app.ringing() {
		t.Fatalf("disabled alarm fired: started=%d ringing=%v", r.started, app.ringing())
	}
}

func TestAutoStopAfterMaxDuration(t *testing.T) {
	now := time.Date(2026, 7, 22, 7, 0, 0, 0, time.UTC)
	app, r := newTestApp(alarm.Alarm{Enabled: true, Hour: 7, Minute: 0, Rhythm: alarm.FullWeek})

	app.evaluate(now) // fires
	app.evaluate(now.Add(maxRingDuration + time.Second))

	if app.ringing() || r.stopped == 0 {
		t.Fatalf("alarm did not auto-stop: ringing=%v stopped=%d", app.ringing(), r.stopped)
	}
}

// Regression for the missed-alarm bug: firing used to require a tick to land
// inside the alarm's exact clock-minute. A scheduler stall or an NTP step over
// that minute lost the alarm silently. Firing is now level-triggered within
// fireGrace, so a tick arriving late still rings it.
func TestFiresWhenTheExactMinuteIsSkipped(t *testing.T) {
	app, r := newTestApp(alarm.Alarm{Enabled: true, Hour: 7, Minute: 0, Rhythm: alarm.FullWeek})

	// First tick well before the alarm, then nothing until after 07:00 has
	// passed entirely — as happens when the clock steps forward.
	app.evaluate(time.Date(2026, 7, 22, 6, 58, 0, 0, time.UTC))
	app.evaluate(time.Date(2026, 7, 22, 7, 1, 30, 0, time.UTC))

	if !app.ringing() || r.started != 1 {
		t.Fatalf("alarm lost when its exact minute was skipped: ringing=%v started=%d", app.ringing(), r.started)
	}
}

// Past the grace window the alarm is not rung hours late; it is recorded as
// missed and does not fire when a later tick arrives.
func TestDoesNotFireLongAfterTheGraceWindow(t *testing.T) {
	app, r := newTestApp(alarm.Alarm{Enabled: true, Hour: 7, Minute: 0, Rhythm: alarm.FullWeek})

	app.evaluate(time.Date(2026, 7, 22, 6, 58, 0, 0, time.UTC))
	app.evaluate(time.Date(2026, 7, 22, 9, 30, 0, 0, time.UTC)) // 2.5h late

	if app.ringing() || r.started != 0 {
		t.Fatalf("alarm rang far outside its window: ringing=%v started=%d", app.ringing(), r.started)
	}
}

// Starting the app long after an alarm's time must not fire it retroactively.
func TestStartupDoesNotFirePastOccurrences(t *testing.T) {
	app, r := newTestApp(alarm.Alarm{Enabled: true, Hour: 7, Minute: 0, Rhythm: alarm.FullWeek})

	app.evaluate(time.Date(2026, 7, 22, 15, 0, 0, 0, time.UTC)) // first ever tick

	if app.ringing() || r.started != 0 {
		t.Fatalf("a long-past alarm fired at startup: ringing=%v started=%d", app.ringing(), r.started)
	}
}

// An alarm due moments before startup should still ring: the grace window
// applies to the seed too.
func TestStartupFiresAnAlarmDueWithinGrace(t *testing.T) {
	app, r := newTestApp(alarm.Alarm{Enabled: true, Hour: 7, Minute: 0, Rhythm: alarm.FullWeek})

	app.evaluate(time.Date(2026, 7, 22, 7, 0, 30, 0, time.UTC)) // first ever tick

	if !app.ringing() || r.started != 1 {
		t.Fatalf("alarm due 30s ago did not ring at startup: ringing=%v started=%d", app.ringing(), r.started)
	}
}

// The next day's occurrence is a distinct one, so a daily alarm keeps firing.
func TestFiresAgainTheNextDay(t *testing.T) {
	app, r := newTestApp(alarm.Alarm{Enabled: true, Hour: 7, Minute: 0, Rhythm: alarm.FullWeek})

	app.evaluate(time.Date(2026, 7, 22, 7, 0, 0, 0, time.UTC))
	app.stopRinging()
	app.evaluate(time.Date(2026, 7, 23, 7, 0, 0, 0, time.UTC))

	if !app.ringing() || r.started != 2 {
		t.Fatalf("daily alarm did not fire the next day: ringing=%v started=%d", app.ringing(), r.started)
	}
}

// A Workweek alarm must stay silent on the weekend even inside its window.
func TestRhythmStillGatesFiring(t *testing.T) {
	app, r := newTestApp(alarm.Alarm{Enabled: true, Hour: 7, Minute: 0, Rhythm: alarm.Workweek})

	app.evaluate(time.Date(2026, 7, 25, 7, 0, 10, 0, time.UTC)) // Saturday

	if app.ringing() || r.started != 0 {
		t.Fatalf("Workweek alarm fired on a Saturday: ringing=%v started=%d", app.ringing(), r.started)
	}
}

// captureLog collects what fn writes to the standard logger.
func captureLog(fn func()) string {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	fn()
	return buf.String()
}

// A forward NTP step is the failure we cannot see in the monotonic clock, so
// it must be reported from the divergence between the two gaps. (The Pi has no
// battery-backed RTC, so timesyncd steps the clock after a reboot.)
func TestLogsWallClockStep(t *testing.T) {
	out := captureLog(func() {
		logTimeAnomaly(time.Second, time.Hour) // 1s really elapsed, clock jumped an hour
	})
	if !strings.Contains(out, "WALL CLOCK STEPPED") {
		t.Fatalf("forward clock step not reported, got: %q", out)
	}
}

func TestLogsBackwardWallClockStep(t *testing.T) {
	out := captureLog(func() {
		logTimeAnomaly(time.Second, -30*time.Minute)
	})
	if !strings.Contains(out, "WALL CLOCK STEPPED") {
		t.Fatalf("backward clock step not reported, got: %q", out)
	}
}

func TestLogsSchedulerStall(t *testing.T) {
	out := captureLog(func() {
		logTimeAnomaly(90*time.Second, 90*time.Second) // both clocks agree: a real stall
	})
	if !strings.Contains(out, "STALLED") {
		t.Fatalf("scheduler stall not reported, got: %q", out)
	}
	if strings.Contains(out, "WALL CLOCK STEPPED") {
		t.Fatalf("a stall must not be reported as a clock step, got: %q", out)
	}
}

func TestOrdinaryTickIsSilent(t *testing.T) {
	out := captureLog(func() {
		logTimeAnomaly(time.Second, time.Second+3*time.Millisecond)
	})
	if out != "" {
		t.Fatalf("a normal tick should log nothing, got: %q", out)
	}
}

// A missed occurrence is the signature we need in the log to diagnose the next
// failure, so assert the line is actually written.
func TestLogsMissedOccurrence(t *testing.T) {
	app, r := newTestApp(alarm.Alarm{Enabled: true, Hour: 7, Minute: 0, Rhythm: alarm.FullWeek})
	out := captureLog(func() {
		app.evaluate(time.Date(2026, 7, 22, 6, 59, 0, 0, time.UTC))
		app.evaluate(time.Date(2026, 7, 22, 10, 15, 0, 0, time.UTC))
	})
	if !strings.Contains(out, "MISSED occurrence") {
		t.Fatalf("missed occurrence not reported, got: %q", out)
	}
	if r.started != 0 {
		t.Fatalf("a missed occurrence must not ring: started=%d", r.started)
	}
}

// The warm-up exists because a Spotify Connect session can die overnight
// without librespot noticing, and the alarm itself is the worst possible
// moment to find out. It must run ahead of the alarm, exactly once.
func TestPreparesShortlyBeforeTheAlarm(t *testing.T) {
	app, r := newTestApp(alarm.Alarm{Enabled: true, Hour: 8, Minute: 0, Rhythm: alarm.FullWeek})

	app.evaluate(time.Date(2026, 9, 30, 7, 57, 0, 0, time.UTC)) // 3 min out: too early
	if len(r.prepared) != 0 {
		t.Fatalf("prepared too early: %d", len(r.prepared))
	}

	app.evaluate(time.Date(2026, 9, 30, 7, 58, 0, 0, time.UTC)) // 2 min out
	app.evaluate(time.Date(2026, 9, 30, 7, 59, 0, 0, time.UTC)) // still the same occurrence
	if len(r.prepared) != 1 {
		t.Fatalf("prepare ran %d times, want exactly 1 per occurrence", len(r.prepared))
	}
	if r.prepared[0].Hour != 8 {
		t.Fatalf("prepared the wrong alarm: %+v", r.prepared[0])
	}
	if want := time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC); !r.preparedAt[0].Equal(want) {
		t.Fatalf("prepared for %s, want the occurrence %s", r.preparedAt[0], want)
	}
}

// Warming up must not interfere with firing, and the next day's occurrence is
// a fresh one to prepare.
func TestPrepareDoesNotBlockFiringAndRepeatsDaily(t *testing.T) {
	app, r := newTestApp(alarm.Alarm{Enabled: true, Hour: 8, Minute: 0, Rhythm: alarm.FullWeek})

	app.evaluate(time.Date(2026, 9, 30, 7, 58, 0, 0, time.UTC)) // warms up
	app.evaluate(time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC))  // fires
	if !app.ringing() || r.started != 1 {
		t.Fatalf("alarm did not fire after a warm-up: ringing=%v started=%d", app.ringing(), r.started)
	}
	app.stopRinging()

	app.evaluate(time.Date(2026, 10, 1, 7, 58, 0, 0, time.UTC)) // next day
	if len(r.prepared) != 2 {
		t.Fatalf("prepare ran %d times over two days, want 2", len(r.prepared))
	}
}

// A rhythm that skips tomorrow must not be warmed up for a day it won't ring.
func TestDoesNotPrepareForASkippedDay(t *testing.T) {
	app, r := newTestApp(alarm.Alarm{Enabled: true, Hour: 8, Minute: 0, Rhythm: alarm.Workweek})

	// Saturday 07:58 — the next occurrence is Monday, not in prepare range.
	app.evaluate(time.Date(2026, 10, 3, 7, 58, 0, 0, time.UTC))
	if len(r.prepared) != 0 {
		t.Fatalf("warmed up for a day the alarm does not ring: %d", len(r.prepared))
	}
}

// Snoozing hands the ringer a Snooze, not a Stop: a Spotify alarm keeps its
// playlist going at volume 0 so ringing again is just a volume ramp.
func TestSnoozeSnoozesTheRinger(t *testing.T) {
	now := time.Date(2026, 7, 22, 7, 0, 0, 0, time.UTC)
	app, r := newTestApp(alarm.Alarm{Enabled: true, Hour: 7, Minute: 0, Rhythm: alarm.FullWeek})

	app.evaluate(now)
	app.snooze(now)
	if r.snoozed != 1 || r.stopped != 0 {
		t.Fatalf("snooze: snoozed=%d stopped=%d, want 1 and 0", r.snoozed, r.stopped)
	}
}

// An alarm edited during its warm-up must not ring as warmed up: the warm-up
// is abandoned and redone with the new settings.
func TestEditDuringWarmUpRedoesIt(t *testing.T) {
	al := alarm.Alarm{Enabled: true, Hour: 8, Minute: 0, Rhythm: alarm.FullWeek,
		Sound: alarm.Sound{Kind: alarm.SoundSpotify, Ref: "spotify:playlist:a"}}
	app, r := newTestApp(al)

	app.evaluate(time.Date(2026, 9, 30, 7, 58, 30, 0, time.UTC)) // warms up
	al.Sound.Ref = "spotify:playlist:b"
	app.setAlarm(0, al)
	app.evaluate(time.Date(2026, 9, 30, 7, 58, 31, 0, time.UTC)) // notices the edit
	if r.cancelled != 1 {
		t.Fatalf("warm-up not cancelled after an edit: cancelled=%d", r.cancelled)
	}
	app.evaluate(time.Date(2026, 9, 30, 7, 58, 32, 0, time.UTC)) // warms up again
	if len(r.prepared) != 2 || r.prepared[1].Sound.Ref != "spotify:playlist:b" {
		t.Fatalf("warm-up not redone with the new settings: %+v", r.prepared)
	}
}

// Switching an alarm off during its warm-up ends the warm-up for good.
func TestDisableDuringWarmUpCancelsIt(t *testing.T) {
	app, r := newTestApp(alarm.Alarm{Enabled: true, Hour: 8, Minute: 0, Rhythm: alarm.FullWeek})

	app.evaluate(time.Date(2026, 9, 30, 7, 58, 30, 0, time.UTC))
	app.setAlarmEnabled(0, false)
	app.evaluate(time.Date(2026, 9, 30, 7, 58, 31, 0, time.UTC))
	app.evaluate(time.Date(2026, 9, 30, 7, 58, 32, 0, time.UTC))
	if r.cancelled != 1 || len(r.prepared) != 1 {
		t.Fatalf("disable during warm-up: cancelled=%d prepared=%d, want 1 and 1", r.cancelled, len(r.prepared))
	}
}

// A warmed-up occurrence that is then missed must not leave the ringer
// playing silently.
func TestMissedOccurrenceCancelsItsWarmUp(t *testing.T) {
	app, r := newTestApp(alarm.Alarm{Enabled: true, Hour: 8, Minute: 0, Rhythm: alarm.FullWeek})

	app.evaluate(time.Date(2026, 9, 30, 7, 58, 30, 0, time.UTC))
	captureLog(func() { app.evaluate(time.Date(2026, 9, 30, 8, 5, 0, 0, time.UTC)) }) // stalled past the grace
	if r.started != 0 || r.cancelled != 1 {
		t.Fatalf("missed occurrence: started=%d cancelled=%d, want 0 and 1", r.started, r.cancelled)
	}
}

// The ringer holds one alarm at a time: a snoozed alarm keeps it, so another
// alarm due meanwhile is not warmed up over it.
func TestNoWarmUpWhileSnoozed(t *testing.T) {
	store := &config.Store{}
	store.Alarms[0] = alarm.Alarm{Enabled: true, Hour: 7, Minute: 0, Rhythm: alarm.FullWeek}
	store.Alarms[1] = alarm.Alarm{Enabled: true, Hour: 7, Minute: 3, Rhythm: alarm.FullWeek}
	r := &fakeRinger{}
	app := NewApp(NewTheme(), store, r)

	now := time.Date(2026, 9, 30, 7, 0, 0, 0, time.UTC)
	app.evaluate(now) // alarm 0 fires
	app.snooze(now)
	app.evaluate(now.Add(90 * time.Second)) // alarm 1 is 90s out
	if len(r.prepared) != 0 {
		t.Fatalf("warmed up alarm 1 over a snoozed alarm 0: %+v", r.prepared)
	}
}
