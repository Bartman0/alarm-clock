package alarm

import (
	"testing"
	"time"
)

// onWeekday returns noon on the first day on/after 2026-07-01 that falls on wd.
func onWeekday(wd time.Weekday) time.Time {
	d := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	for d.Weekday() != wd {
		d = d.AddDate(0, 0, 1)
	}
	return d
}

func TestRhythmActive(t *testing.T) {
	cases := []struct {
		r    Rhythm
		wd   time.Weekday
		want bool
	}{
		{FullWeek, time.Monday, true},
		{FullWeek, time.Sunday, true},
		{Workweek, time.Monday, true},
		{Workweek, time.Friday, true},
		{Workweek, time.Saturday, false},
		{Workweek, time.Sunday, false},
		{Weekend, time.Saturday, true},
		{Weekend, time.Sunday, true},
		{Weekend, time.Wednesday, false},
	}
	for _, c := range cases {
		if got := c.r.Active(c.wd); got != c.want {
			t.Errorf("%v.Active(%v) = %v, want %v", c.r, c.wd, got, c.want)
		}
	}
}

func TestNextDisabled(t *testing.T) {
	a := Alarm{Enabled: false, Hour: 7, Rhythm: FullWeek}
	if _, ok := a.Next(time.Now()); ok {
		t.Fatal("disabled alarm should not have a next fire time")
	}
}

func TestNextLaterToday(t *testing.T) {
	now := onWeekday(time.Wednesday) // 12:00
	a := Alarm{Enabled: true, Hour: 14, Minute: 30, Rhythm: FullWeek}
	got, ok := a.Next(now)
	if !ok {
		t.Fatal("expected a next fire time")
	}
	if got.Day() != now.Day() || got.Hour() != 14 || got.Minute() != 30 {
		t.Errorf("got %v, want today 14:30", got)
	}
}

func TestNextRollsToTomorrow(t *testing.T) {
	now := onWeekday(time.Wednesday) // 12:00
	a := Alarm{Enabled: true, Hour: 7, Rhythm: FullWeek}
	got, ok := a.Next(now)
	if !ok {
		t.Fatal("expected a next fire time")
	}
	if got.Weekday() != time.Thursday || got.Hour() != 7 {
		t.Errorf("got %v, want Thursday 07:00", got)
	}
}

func TestNextWorkweekSkipsWeekend(t *testing.T) {
	now := onWeekday(time.Saturday) // 12:00
	a := Alarm{Enabled: true, Hour: 7, Rhythm: Workweek}
	got, ok := a.Next(now)
	if !ok {
		t.Fatal("expected a next fire time")
	}
	if got.Weekday() != time.Monday || got.Hour() != 7 {
		t.Errorf("got %v (%v), want Monday 07:00", got, got.Weekday())
	}
}

func TestNextWeekendFromMidweek(t *testing.T) {
	now := onWeekday(time.Wednesday) // 12:00
	a := Alarm{Enabled: true, Hour: 9, Rhythm: Weekend}
	got, ok := a.Next(now)
	if !ok {
		t.Fatal("expected a next fire time")
	}
	if got.Weekday() != time.Saturday || got.Hour() != 9 {
		t.Errorf("got %v (%v), want Saturday 09:00", got, got.Weekday())
	}
}
