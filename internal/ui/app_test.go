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

func TestTickFiresAtMatchingMinute(t *testing.T) {
	now := time.Date(2026, 7, 22, 7, 0, 5, 0, time.UTC)
	app, r := newTestApp(alarm.Alarm{Enabled: true, Hour: 7, Minute: 0, Rhythm: alarm.FullWeek})

	app.now = now
	app.tick(now)

	if app.cur != screenFiring {
		t.Fatalf("cur = %v, want screenFiring", app.cur)
	}
	if r.started != 1 {
		t.Fatalf("ringer started %d times, want 1", r.started)
	}
}

func TestTickDoesNotRefireSameMinute(t *testing.T) {
	now := time.Date(2026, 7, 22, 7, 0, 0, 0, time.UTC)
	app, r := newTestApp(alarm.Alarm{Enabled: true, Hour: 7, Minute: 0, Rhythm: alarm.FullWeek})

	app.now = now
	app.tick(now)      // fires
	app.stopRinging()  // user stops
	app.tick(now.Add(10 * time.Second)) // still same minute

	if r.started != 1 {
		t.Fatalf("ringer started %d times, want 1 (guarded within the minute)", r.started)
	}
}

func TestSnoozeReArmsAfterFiveMinutes(t *testing.T) {
	now := time.Date(2026, 7, 22, 7, 0, 0, 0, time.UTC)
	app, r := newTestApp(alarm.Alarm{Enabled: true, Hour: 7, Minute: 0, Rhythm: alarm.FullWeek})

	app.now = now
	app.tick(now) // fires
	app.snooze()  // snooze from firing screen

	if app.cur != screenHome {
		t.Fatalf("after snooze cur = %v, want screenHome", app.cur)
	}

	// Not yet: 4 minutes later nothing happens.
	early := now.Add(4 * time.Minute)
	app.now = early
	app.tick(early)
	if app.cur == screenFiring {
		t.Fatal("alarm re-fired before the 5-minute snooze elapsed")
	}

	// After 5 minutes it rings again.
	later := now.Add(5 * time.Minute)
	app.now = later
	app.tick(later)
	if app.cur != screenFiring || r.started != 2 {
		t.Fatalf("snooze did not re-fire: cur=%v started=%d", app.cur, r.started)
	}
}

// Regression: snoozing partway through the firing minute must not be
// overridden by a second trigger later in that same minute.
func TestSnoozeNotOverriddenLaterInSameMinute(t *testing.T) {
	fire := time.Date(2026, 7, 22, 7, 0, 3, 0, time.UTC)
	app, r := newTestApp(alarm.Alarm{Enabled: true, Hour: 7, Minute: 0, Rhythm: alarm.FullWeek})

	app.now = fire
	app.tick(fire) // fires at 07:00:03

	snoozeAt := time.Date(2026, 7, 22, 7, 0, 10, 0, time.UTC)
	app.now = snoozeAt
	app.snooze()

	// Later in the SAME minute (07:00:40) the alarm must stay silent.
	later := time.Date(2026, 7, 22, 7, 0, 40, 0, time.UTC)
	app.now = later
	app.tick(later)

	if app.cur == screenFiring || r.started != 1 {
		t.Fatalf("alarm re-fired within the same minute after snooze: cur=%v started=%d", app.cur, r.started)
	}
}

func TestDisabledAlarmDoesNotFire(t *testing.T) {
	now := time.Date(2026, 7, 22, 7, 0, 0, 0, time.UTC)
	app, r := newTestApp(alarm.Alarm{Enabled: false, Hour: 7, Minute: 0, Rhythm: alarm.FullWeek})

	app.now = now
	app.tick(now)

	if r.started != 0 || app.cur != screenHome {
		t.Fatalf("disabled alarm fired: started=%d cur=%v", r.started, app.cur)
	}
}

func TestAutoStopAfterMaxDuration(t *testing.T) {
	now := time.Date(2026, 7, 22, 7, 0, 0, 0, time.UTC)
	app, r := newTestApp(alarm.Alarm{Enabled: true, Hour: 7, Minute: 0, Rhythm: alarm.FullWeek})

	app.now = now
	app.tick(now) // fires

	past := now.Add(maxRingDuration + time.Second)
	app.now = past
	app.tick(past)

	if app.cur != screenHome || r.stopped == 0 {
		t.Fatalf("alarm did not auto-stop: cur=%v stopped=%d", app.cur, r.stopped)
	}
}
