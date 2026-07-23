package ui

import (
	"image/color"
	"time"

	"gioui.org/layout"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"alarmclock/internal/clock"
)

// Fill paints the whole context area with a single colour.
func Fill(gtx layout.Context, c color.NRGBA) {
	paint.Fill(gtx.Ops, c)
}

// Home is the always-on landing screen: analog clock on the left, digital time
// and Dutch date on the right, and the Radio / Spotify buttons along the
// bottom. Button actions are wired up in later milestones; the clickables are
// exposed so the app loop can react to them.
type Home struct {
	Radio   widget.Clickable
	Spotify widget.Clickable
}

func (h *Home) Layout(gtx layout.Context, th *material.Theme, now time.Time) layout.Dimensions {
	Fill(gtx, Mocha.Base)
	inset := layout.UniformInset(unit.Dp(24))
	return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			// Top: clock + time/date side by side.
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return AnalogClock(gtx, now)
						})
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return h.layoutDigital(gtx, th, now)
					}),
				)
			}),
			// Bottom: the two big action buttons.
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return bigButton(gtx, th, &h.Radio, "Radio", Mocha.Blue)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return bigButton(gtx, th, &h.Spotify, "Spotify", Mocha.Green)
						}),
					)
				})
			}),
		)
	})
}

func (h *Home) layoutDigital(gtx layout.Context, th *material.Theme, now time.Time) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.End}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := material.Label(th, unit.Sp(180), clock.Time(now))
			l.Color = Mocha.Text
			l.Font.Weight = 600
			return l.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := material.Label(th, unit.Sp(40), clock.Weekday(now))
			l.Color = Mocha.Mauve
			return l.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := material.Label(th, unit.Sp(40), clock.Date(now))
			l.Color = Mocha.Subtext0
			return l.Layout(gtx)
		}),
	)
}

// bigButton renders a large, touch-friendly rounded button in the given accent
// colour with a dark label.
func bigButton(gtx layout.Context, th *material.Theme, click *widget.Clickable, label string, accent color.NRGBA) layout.Dimensions {
	b := material.Button(th, click, label)
	b.Background = accent
	b.Color = Mocha.Crust
	b.TextSize = unit.Sp(40)
	b.CornerRadius = unit.Dp(20)
	b.Inset = layout.UniformInset(unit.Dp(28))
	return b.Layout(gtx)
}
