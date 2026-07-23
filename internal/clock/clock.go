// Package clock provides the time source and Dutch-locale formatting used by
// the alarm clock. Go's standard library has no locale support, so weekday
// names are provided here explicitly.
package clock

import "time"

// dutchWeekdays is indexed by time.Weekday (0 = Sunday .. 6 = Saturday).
var dutchWeekdays = [...]string{
	time.Sunday:    "zondag",
	time.Monday:    "maandag",
	time.Tuesday:   "dinsdag",
	time.Wednesday: "woensdag",
	time.Thursday:  "donderdag",
	time.Friday:    "vrijdag",
	time.Saturday:  "zaterdag",
}

// Weekday returns the Dutch name of the weekday, e.g. "woensdag".
func Weekday(t time.Time) string {
	return dutchWeekdays[t.Weekday()]
}

// Date formats the date as DD-MM-YYYY.
func Date(t time.Time) string {
	return t.Format("02-01-2006")
}

// Time formats the time as HH:MM in 24-hour notation.
func Time(t time.Time) string {
	return t.Format("15:04")
}

// Seconds formats just the seconds as SS, for the smaller secondary display.
func Seconds(t time.Time) string {
	return t.Format("05")
}
