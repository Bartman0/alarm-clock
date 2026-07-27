package ui

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"alarmclock/internal/alarm"
)

var spacer12 = layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout)

// beginEdit loads alarm i into the draft and opens the editor.
func (a *App) beginEdit(i int) {
	a.editIdx = i
	a.draft = a.store.Alarms[i]
	a.editEnable.Value = a.draft.Enabled
	a.cur = screenEdit
}

// layoutEdit is the alarm editor: the time with the on/off switch beside it,
// centred rhythm and sound choices below, and Save pinned to the lower right.
func (a *App) layoutEdit(gtx layout.Context) layout.Dimensions {
	a.handleEditEvents(gtx)

	return layout.UniformInset(unit.Dp(24)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(a.layoutEditHeader),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(a.layoutTimeRow),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return a.editSection(gtx, "Ritme", func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
							a.rhythmSeg(alarm.FullWeek), spacer12,
							a.rhythmSeg(alarm.Workweek), spacer12,
							a.rhythmSeg(alarm.Weekend), spacer12,
							a.rhythmSeg(alarm.Once),
						)
					})
				})
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(14)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return a.editSection(gtx, "Geluid", func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
							a.soundSeg(alarm.SoundAlarm), spacer12,
							a.soundSeg(alarm.SoundSpotify),
						)
					})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if a.draft.Sound.Kind != alarm.SoundSpotify {
					return layout.Dimensions{}
				}
				return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, a.layoutSpotifyChoice)
			}),
			// Push Save to the bottom-right corner.
			layout.Flexed(1, flexSpacer),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.E.Layout(gtx, a.saveButton)
			}),
		)
	})
}

func (a *App) handleEditEvents(gtx layout.Context) {
	if a.editCancel.Clicked(gtx) {
		a.cur = screenAlarms
	}
	if a.editSave.Clicked(gtx) {
		a.draft.Enabled = a.editEnable.Value
		a.store.Alarms[a.editIdx] = a.draft
		a.rows[a.editIdx].toggle.Value = a.draft.Enabled
		a.save()
		a.cur = screenAlarms
	}
	if a.hourUp.Clicked(gtx) {
		a.draft.Hour = (a.draft.Hour + 1) % 24
	}
	if a.hourDown.Clicked(gtx) {
		a.draft.Hour = (a.draft.Hour + 23) % 24
	}
	if a.minUp.Clicked(gtx) {
		a.draft.Minute = (a.draft.Minute + 1) % 60
	}
	if a.minDown.Clicked(gtx) {
		a.draft.Minute = (a.draft.Minute + 59) % 60
	}
	for k := range a.rhythmBtns {
		if a.rhythmBtns[k].Clicked(gtx) {
			a.draft.Rhythm = alarm.Rhythm(k)
		}
	}
	for k := range a.soundBtns {
		if a.soundBtns[k].Clicked(gtx) {
			a.draft.Sound.Kind = alarm.SoundKind(k)
		}
	}
	if a.editPick.Clicked(gtx) {
		a.openSpotifyForPick()
	}
}

// layoutSpotifyChoice shows the chosen Spotify playlist for the alarm and a
// button to pick one; only rendered when the sound kind is Spotify.
func (a *App) layoutSpotifyChoice(gtx layout.Context) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := a.draft.Sound.Label
				if label == "" {
					label = "Geen afspeellijst gekozen"
				}
				l := material.Label(a.th, unit.Sp(22), label)
				l.Color = colorIf(a.draft.Sound.Label != "", Mocha.Green, Mocha.Overlay1)
				l.MaxLines = 1
				return l.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				b := material.Button(a.th, &a.editPick, "Kies afspeellijst")
				b.Background = Mocha.Surface1
				b.Color = Mocha.Text
				b.TextSize = unit.Sp(20)
				b.CornerRadius = unit.Dp(10)
				b.Inset = layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(12), Left: unit.Dp(18), Right: unit.Dp(18)}
				return b.Layout(gtx)
			}),
		)
	})
}

// layoutEditHeader shows the back button, the alarm's name and the on/off
// switch beside the name.
func (a *App) layoutEditHeader(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return header(gtx, a.th, &a.editCancel, fmt.Sprintf("Alarm %d", a.editIdx+1))
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(24)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			sw := material.Switch(a.th, &a.editEnable, "ingeschakeld")
			sw.Color.Enabled = Mocha.Green
			sw.Color.Disabled = Mocha.Surface2
			return sw.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := material.Label(a.th, unit.Sp(22), "Ingeschakeld")
			l.Color = Mocha.Subtext0
			return l.Layout(gtx)
		}),
	)
}

// layoutTimeRow centres the HH:MM steppers.
func (a *App) layoutTimeRow(gtx layout.Context) layout.Dimensions {
	col := func(gtx layout.Context, up, down *widget.Clickable, v int) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return stepButton(gtx, a.th, up, "+")
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := material.Label(a.th, unit.Sp(96), fmt.Sprintf("%02d", v))
				l.Color = Mocha.Text
				l.Font.Weight = 600
				return layout.UniformInset(unit.Dp(8)).Layout(gtx, l.Layout)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return stepButton(gtx, a.th, down, "−")
			}),
		)
	}
	colon := func(gtx layout.Context) layout.Dimensions {
		l := material.Label(a.th, unit.Sp(96), ":")
		l.Color = Mocha.Overlay1
		return layout.UniformInset(unit.Dp(8)).Layout(gtx, l.Layout)
	}

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return col(gtx, &a.hourUp, &a.hourDown, a.draft.Hour)
			}),
			layout.Rigid(colon),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return col(gtx, &a.minUp, &a.minDown, a.draft.Minute)
			}),
		)
	})
}

func (a *App) rhythmSeg(r alarm.Rhythm) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return segButton(gtx, a.th, &a.rhythmBtns[r], r.String(), a.draft.Rhythm == r, Mocha.Mauve)
	})
}

func (a *App) soundSeg(k alarm.SoundKind) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return segButton(gtx, a.th, &a.soundBtns[k], k.String(), a.draft.Sound.Kind == k, Mocha.Green)
	})
}

// editSection stacks a centred title above centred body content.
func (a *App) editSection(gtx layout.Context, title string, body layout.Widget) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := material.Label(a.th, unit.Sp(24), title)
			l.Color = Mocha.Subtext0
			l.Alignment = text.Middle
			return layout.Inset{Bottom: unit.Dp(10)}.Layout(gtx, l.Layout)
		}),
		layout.Rigid(body),
	)
}

func (a *App) saveButton(gtx layout.Context) layout.Dimensions {
	b := material.Button(a.th, &a.editSave, "Opslaan")
	b.Background = Mocha.Green
	b.Color = Mocha.Crust
	b.TextSize = unit.Sp(20)
	b.CornerRadius = unit.Dp(10)
	b.Inset = layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(22), Right: unit.Dp(22)}
	return b.Layout(gtx)
}
