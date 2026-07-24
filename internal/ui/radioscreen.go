package ui

import (
	"context"
	"fmt"
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"alarmclock/internal/radio"
)

// openRadio switches to the radio screen and kicks off an initial "top
// stations" fetch if we have nothing yet.
func (a *App) openRadio() {
	a.cur = screenRadio
	a.radioMu.Lock()
	empty := len(a.radioResults) == 0 && !a.radioLoading
	a.radioMu.Unlock()
	if empty {
		a.fetchRadio("")
	}
}

// fetchRadio loads the top stations (empty query) or search results, off the
// UI goroutine.
func (a *App) fetchRadio(query string) {
	a.radioMu.Lock()
	a.radioLoading = true
	a.radioErr = ""
	a.radioMu.Unlock()

	go func() {
		c := a.ensureRadioClient()
		var (
			st  []radio.Station
			err error
		)
		if query == "" {
			st, err = c.Top(context.Background(), maxRadioResults)
		} else {
			st, err = c.Search(context.Background(), query, maxRadioResults)
		}
		a.radioMu.Lock()
		a.radioLoading = false
		if err != nil {
			a.radioErr = err.Error()
		} else {
			a.radioResults = st
		}
		a.radioMu.Unlock()
		if a.invalidate != nil {
			a.invalidate()
		}
	}()
}

func (a *App) ensureRadioClient() *radio.Client {
	a.radioMu.Lock()
	defer a.radioMu.Unlock()
	if a.radioClient == nil {
		a.radioClient = radio.NewClient()
	}
	return a.radioClient
}

func (a *App) radioSnapshot() (res []radio.Station, loading bool, errMsg string) {
	a.radioMu.Lock()
	defer a.radioMu.Unlock()
	return a.radioResults, a.radioLoading, a.radioErr
}

func (a *App) playStation(s radio.Station) {
	if a.radio == nil {
		return
	}
	if url := s.StreamURL(); url != "" {
		a.radio.PlayStream(url)
		a.nowPlaying = s.Name
	}
}

func (a *App) layoutRadio(gtx layout.Context) layout.Dimensions {
	// Navigation and controls. Leaving the screen does not stop playback.
	if a.radioBack.Clicked(gtx) {
		a.cur = screenHome
	}
	if a.radioSearch.Clicked(gtx) {
		a.fetchRadio(strings.TrimSpace(a.radioQuery.Text()))
	}
	if a.radioStop.Clicked(gtx) {
		if a.radio != nil {
			a.radio.StopStream()
		}
		a.nowPlaying = ""
	}
	// Enter in the search field triggers a search.
	for {
		ev, ok := a.radioQuery.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			a.fetchRadio(strings.TrimSpace(a.radioQuery.Text()))
		}
	}

	results, loading, errMsg := a.radioSnapshot()
	n := len(results)
	if n > len(a.radioRows) {
		n = len(a.radioRows)
	}
	for i := 0; i < n; i++ {
		if a.radioRows[i].Clicked(gtx) {
			a.playStation(results[i])
		}
	}

	return layout.UniformInset(unit.Dp(24)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return header(gtx, a.th, &a.radioBack, "Radio")
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(a.layoutRadioSearch),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if a.nowPlaying == "" {
					return layout.Dimensions{}
				}
				return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, a.layoutNowPlaying)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return a.layoutRadioStatus(gtx, len(results), loading, errMsg)
				})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return material.List(a.th, &a.radioList).Layout(gtx, n, func(gtx layout.Context, i int) layout.Dimensions {
					return a.layoutStationRow(gtx, results[i], i)
				})
			}),
		)
	})
}

func (a *App) layoutRadioSearch(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return roundedPanel(gtx, Mocha.Surface0, unit.Dp(12), func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					ed := material.Editor(a.th, &a.radioQuery, "Zoek een zender…")
					ed.TextSize = unit.Sp(24)
					ed.Color = Mocha.Text
					ed.HintColor = Mocha.Overlay0
					return ed.Layout(gtx)
				})
			})
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			b := material.Button(a.th, &a.radioSearch, "Zoek")
			b.Background = Mocha.Blue
			b.Color = Mocha.Crust
			b.TextSize = unit.Sp(24)
			b.CornerRadius = unit.Dp(12)
			b.Inset = layout.UniformInset(unit.Dp(18))
			return b.Layout(gtx)
		}),
	)
}

func (a *App) layoutNowPlaying(gtx layout.Context) layout.Dimensions {
	return roundedPanel(gtx, Mocha.Surface1, unit.Dp(12), func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(14)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					l := material.Label(a.th, unit.Sp(22), "Speelt nu: "+a.nowPlaying)
					l.Color = Mocha.Green
					l.MaxLines = 1
					return l.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					b := material.Button(a.th, &a.radioStop, "Stop")
					b.Background = Mocha.Red
					b.Color = Mocha.Crust
					b.TextSize = unit.Sp(20)
					b.CornerRadius = unit.Dp(10)
					b.Inset = layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(12), Left: unit.Dp(20), Right: unit.Dp(20)}
					return b.Layout(gtx)
				}),
			)
		})
	})
}

func (a *App) layoutRadioStatus(gtx layout.Context, count int, loading bool, errMsg string) layout.Dimensions {
	var (
		msg string
		col = Mocha.Overlay1
	)
	switch {
	case loading:
		msg = "Laden…"
		col = Mocha.Subtext0
	case errMsg != "":
		msg = "Kan zenders niet laden: " + errMsg
		col = Mocha.Red
	case count == 0:
		msg = "Geen zenders gevonden"
	default:
		return layout.Dimensions{}
	}
	l := material.Label(a.th, unit.Sp(22), msg)
	l.Color = col
	l.MaxLines = 2
	return l.Layout(gtx)
}

func (a *App) layoutStationRow(gtx layout.Context, s radio.Station, i int) layout.Dimensions {
	playing := s.Name != "" && s.Name == a.nowPlaying
	return layout.Inset{Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return a.radioRows[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return roundedPanel(gtx, colorIf(playing, Mocha.Surface1, Mocha.Surface0), unit.Dp(12), func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(14)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									l := material.Label(a.th, unit.Sp(26), s.Name)
									l.Color = colorIf(playing, Mocha.Green, Mocha.Text)
									l.MaxLines = 1
									return l.Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									l := material.Label(a.th, unit.Sp(18), stationSubtitle(s))
									l.Color = Mocha.Overlay1
									l.MaxLines = 1
									return l.Layout(gtx)
								}),
							)
						}),
					)
				})
			})
		})
	})
}

// stationSubtitle builds a compact secondary line: country · codec bitrate · tags.
func stationSubtitle(s radio.Station) string {
	var parts []string
	if s.Country != "" {
		parts = append(parts, s.Country)
	}
	if s.Codec != "" {
		if s.Bitrate > 0 {
			parts = append(parts, fmt.Sprintf("%s %dk", s.Codec, s.Bitrate))
		} else {
			parts = append(parts, s.Codec)
		}
	}
	if s.Tags != "" {
		parts = append(parts, s.Tags)
	}
	return strings.Join(parts, " · ")
}
