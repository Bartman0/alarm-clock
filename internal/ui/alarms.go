package ui

import (
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// layoutAlarms shows the three alarms with an on/off switch each; tapping a row
// opens the editor.
func (a *App) layoutAlarms(gtx layout.Context) layout.Dimensions {
	if a.btnAlarmsBack.Clicked(gtx) {
		a.cur = screenHome
	}
	for i := range a.rows {
		if a.rows[i].tap.Clicked(gtx) {
			a.beginEdit(i)
		}
	}

	return layout.UniformInset(unit.Dp(24)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return header(gtx, a.th, &a.btnAlarmsBack, "Alarmen")
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.layoutAlarmRow(gtx, 0) }),
					layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.layoutAlarmRow(gtx, 1) }),
					layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.layoutAlarmRow(gtx, 2) }),
				)
			}),
		)
	})
}

func (a *App) layoutAlarmRow(gtx layout.Context, i int) layout.Dimensions {
	al := a.store.Alarms[i]
	row := &a.rows[i]

	return widgetCard(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			// Tappable info area (time + rhythm/sound) opens the editor.
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return row.tap.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							l := material.Label(a.th, unit.Sp(64), al.TimeString())
							l.Color = colorIf(al.Enabled, Mocha.Text, Mocha.Overlay0)
							l.Font.Weight = 600
							return l.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(24)}.Layout),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									l := material.Label(a.th, unit.Sp(26), al.Rhythm.String())
									l.Color = colorIf(al.Enabled, Mocha.Mauve, Mocha.Overlay0)
									return l.Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									l := material.Label(a.th, unit.Sp(22), al.Sound.Kind.String())
									l.Color = Mocha.Overlay1
									return l.Layout(gtx)
								}),
							)
						}),
					)
				})
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
			// On/off switch — outside the tap area so it toggles independently.
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				sw := material.Switch(a.th, &row.toggle, "aan/uit")
				sw.Color.Enabled = Mocha.Green
				sw.Color.Disabled = Mocha.Surface2
				dims := sw.Layout(gtx)
				if row.toggle.Value != a.store.Alarms[i].Enabled {
					a.store.Alarms[i].Enabled = row.toggle.Value
					a.save()
				}
				return dims
			}),
		)
	})
}

// widgetCard wraps content in a rounded Surface0 panel.
func widgetCard(gtx layout.Context, w layout.Widget) layout.Dimensions {
	return roundedPanel(gtx, Mocha.Surface0, unit.Dp(16), func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(18)).Layout(gtx, w)
	})
}
