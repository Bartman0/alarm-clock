package ui

import (
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"alarmclock/internal/clock"
)

// Fill paints the whole context area with a single colour.
func Fill(gtx layout.Context, c color.NRGBA) {
	paint.Fill(gtx.Ops, c)
}

// layoutHome is the always-on landing screen: a tappable "next alarm" bar on
// top, the analog clock beside the digital time/date, and the Radio / Spotify
// buttons along the bottom.
func (a *App) layoutHome(gtx layout.Context) layout.Dimensions {
	if a.btnAlarms.Clicked(gtx) {
		a.cur = screenAlarms
	}
	if a.btnRadio.Clicked(gtx) {
		a.openRadio()
	}
	// Spotify opens in M6; harmless no-op for now.
	a.btnSpotify.Clicked(gtx)

	inset := layout.UniformInset(unit.Dp(24))
	return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			// Clock + time/date side by side.
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return AnalogClock(gtx, a.now)
						})
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Right: unit.Dp(40)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return a.layoutDigital(gtx)
						})
					}),
				)
			}),
			// Bottom bar: Radio/Spotify centred, alarm access at the far right.
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						// Equal-weight spacers on both sides keep Radio/Spotify
						// centred on screen; the alarm button right-aligns in
						// the right spacer.
						layout.Flexed(1, flexSpacer),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return actionButton(gtx, a.th, &a.btnRadio, "Radio", Mocha.Blue)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(20)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return actionButton(gtx, a.th, &a.btnSpotify, "Spotify", Mocha.Green)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.E.Layout(gtx, a.alarmAccessButton)
						}),
					)
				})
			}),
		)
	})
}

// alarmAccessButton shows the next-alarm summary as a subdued button that opens
// the alarms screen. It sits bottom-right, in line with Radio/Spotify.
func (a *App) alarmAccessButton(gtx layout.Context) layout.Dimensions {
	b := material.Button(a.th, &a.btnAlarms, a.nextAlarmText(a.now))
	b.Background = Mocha.Surface0
	b.Color = Mocha.Subtext1
	b.TextSize = unit.Sp(22)
	b.CornerRadius = unit.Dp(16)
	b.Inset = layout.Inset{Top: unit.Dp(18), Bottom: unit.Dp(18), Left: unit.Dp(24), Right: unit.Dp(24)}
	return b.Layout(gtx)
}

func (a *App) layoutDigital(gtx layout.Context) layout.Dimensions {
	now := a.now
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.End}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := material.Label(a.th, unit.Sp(180), clock.Time(now))
			l.Color = Mocha.Text
			l.Font.Weight = 600
			l.Alignment = text.End
			return l.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := material.Label(a.th, unit.Sp(52), clock.Weekday(now))
			l.Color = Mocha.Mauve
			l.Alignment = text.End
			return l.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := material.Label(a.th, unit.Sp(52), clock.Date(now))
			l.Color = Mocha.Subtext0
			l.Alignment = text.End
			return l.Layout(gtx)
		}),
	)
}

// actionButton renders a touch-friendly rounded button with a solid accent
// fill and a dark label.
func actionButton(gtx layout.Context, th *material.Theme, click *widget.Clickable, label string, accent color.NRGBA) layout.Dimensions {
	b := material.Button(th, click, label)
	b.Background = accent
	b.Color = Mocha.Crust
	b.TextSize = unit.Sp(30)
	b.CornerRadius = unit.Dp(16)
	b.Inset = layout.Inset{Top: unit.Dp(18), Bottom: unit.Dp(18), Left: unit.Dp(36), Right: unit.Dp(36)}
	return b.Layout(gtx)
}
