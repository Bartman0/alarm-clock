package ui

import (
	"image/color"
	"time"

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
						return layout.Inset{Right: unit.Dp(40)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return h.layoutDigital(gtx, th, now)
						})
					}),
				)
			}),
			// Bottom: the two big action buttons.
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceSides}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return actionButton(gtx, th, &h.Radio, "Radio", Mocha.Blue)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(20)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return actionButton(gtx, th, &h.Spotify, "Spotify", Mocha.Green)
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
			l.Alignment = text.End
			return l.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := material.Label(th, unit.Sp(52), clock.Weekday(now))
			l.Color = Mocha.Mauve
			l.Alignment = text.End
			return l.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := material.Label(th, unit.Sp(52), clock.Date(now))
			l.Color = Mocha.Subtext0
			l.Alignment = text.End
			return l.Layout(gtx)
		}),
	)
}

// actionButton renders a touch-friendly rounded button with a solid accent
// fill and a dark label — sized between the original large buttons and the
// compact variant.
func actionButton(gtx layout.Context, th *material.Theme, click *widget.Clickable, label string, accent color.NRGBA) layout.Dimensions {
	b := material.Button(th, click, label)
	b.Background = accent
	b.Color = Mocha.Crust
	b.TextSize = unit.Sp(30)
	b.CornerRadius = unit.Dp(16)
	b.Inset = layout.Inset{Top: unit.Dp(18), Bottom: unit.Dp(18), Left: unit.Dp(36), Right: unit.Dp(36)}
	return b.Layout(gtx)
}
