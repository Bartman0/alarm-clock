package ui

import (
	"testing"
	"time"

	"alarmclock/internal/alarm"
	"alarmclock/internal/config"
)

type fakeRinger struct {
	started, stopped int
	last             alarm.Alarm
}

func (f *fakeRinger) Start(a alarm.Alarm) { f.started++; f.last = a }
func (f *fakeRinger) Stop()               { f.stopped++ }

func newTestApp(al alarm.Alarm) (*App, *fakeRinger) {
	store := &config.Store{}
	store.Alarms[0] = al
	r := &fakeRinger{}
	return NewApp(NewTheme(), store, r), r
}

// test helpers mirroring the UI's mutex-guarded calls.
func (a *App) testStop() {
	a.mu.Lock()
	a.stopRingingLocked()
	a.mu.Unlock()
}

func (a *App) testSnooze(now time.Time) {
	a.mu.Lock()
	a.snoozeLocked(now)
	a.mu.Unlock()
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
	app.testStop()                          // user stops
	app.evaluate(now.Add(10 * time.Second)) // still same minute

	if r.started != 1 {
		t.Fatalf("ringer started %d times, want 1 (guarded within the minute)", r.started)
	}
}

func TestSnoozeReArmsAfterFiveMinutes(t *testing.T) {
	now := time.Date(2026, 7, 22, 7, 0, 0, 0, time.UTC)
	app, r := newTestApp(alarm.Alarm{Enabled: true, Hour: 7, Minute: 0, Rhythm: alarm.FullWeek})

	app.evaluate(now)   // fires
	app.testSnooze(now) // snooze from firing screen

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
	app.testSnooze(time.Date(2026, 7, 22, 7, 0, 10, 0, time.UTC))

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
