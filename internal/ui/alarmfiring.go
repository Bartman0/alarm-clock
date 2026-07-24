package ui

import (
	"image/color"

	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"alarmclock/internal/clock"
)

// layoutFiring is the full-screen wake view shown while an alarm rings: the big
// current time with Snooze (5 min) and Stop buttons.
func (a *App) layoutFiring(gtx layout.Context) layout.Dimensions {
	if a.btnSnooze.Clicked(gtx) {
		a.snooze()
	}
	if a.btnStop.Clicked(gtx) {
		a.stopRinging()
	}

	Fill(gtx, Mocha.Base)
	return layout.UniformInset(unit.Dp(24)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							l := material.Label(a.th, unit.Sp(48), "Alarm")
							l.Color = Mocha.Red
							l.Alignment = text.Middle
							return l.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							l := material.Label(a.th, unit.Sp(200), clock.Time(a.now))
							l.Color = Mocha.Text
							l.Font.Weight = 700
							l.Alignment = text.Middle
							return l.Layout(gtx)
						}),
					)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceSides}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return firingButton(gtx, a.th, &a.btnSnooze, "Sluimeren 5 min", Mocha.Yellow)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(24)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return firingButton(gtx, a.th, &a.btnStop, "Stoppen", Mocha.Red)
					}),
				)
			}),
		)
	})
}

func firingButton(gtx layout.Context, th *material.Theme, click *widget.Clickable, label string, accent color.NRGBA) layout.Dimensions {
	b := material.Button(th, click, label)
	b.Background = accent
	b.Color = Mocha.Crust
	b.TextSize = unit.Sp(34)
	b.CornerRadius = unit.Dp(18)
	b.Inset = layout.Inset{Top: unit.Dp(28), Bottom: unit.Dp(28), Left: unit.Dp(48), Right: unit.Dp(48)}
	return b.Layout(gtx)
}
