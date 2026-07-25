package ui

import (
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// kbRows is the QWERTY layout for the on-screen keyboard. Search is
// case-insensitive, so lowercase-only keeps it simple.
var kbRows = []string{
	"1234567890",
	"qwertyuiop",
	"asdfghjkl",
	"zxcvbnm",
}

// keyboard is a touch on-screen keyboard for the search fields. A single
// instance is shared across the search screens (only one is visible at a
// time); each Layout call targets the given editor.
type keyboard struct {
	chars     map[rune]*widget.Clickable
	space     widget.Clickable
	backspace widget.Clickable
	search    widget.Clickable
}

func newKeyboard() *keyboard {
	k := &keyboard{chars: map[rune]*widget.Clickable{}}
	for _, row := range kbRows {
		for _, r := range row {
			k.chars[r] = new(widget.Clickable)
		}
	}
	return k
}

// Layout draws the keyboard and applies key presses to ed. The bool is true
// when the "Zoek" (search) key is tapped.
func (k *keyboard) Layout(gtx layout.Context, th *material.Theme, ed *widget.Editor) (layout.Dimensions, bool) {
	searchTapped := false
	for _, row := range kbRows {
		for _, r := range row {
			if k.chars[r].Clicked(gtx) {
				ed.Insert(string(r))
			}
		}
	}
	if k.space.Clicked(gtx) {
		ed.Insert(" ")
	}
	if k.backspace.Clicked(gtx) {
		if t := ed.Text(); t != "" {
			rs := []rune(t)
			trimmed := string(rs[:len(rs)-1])
			ed.SetText(trimmed)
			n := len([]rune(trimmed))
			ed.SetCaret(n, n)
		}
	}
	if k.search.Clicked(gtx) {
		searchTapped = true
	}

	rowFlex := func(runes string) layout.Dimensions {
		children := make([]layout.FlexChild, 0, len(runes))
		for _, r := range runes {
			r := r
			children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return k.key(gtx, th, k.chars[r], string(r), Mocha.Surface1, Mocha.Text)
			}))
		}
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
	}

	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return rowFlex(kbRows[0]) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return rowFlex(kbRows[1]) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return rowFlex(kbRows[2]) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return rowFlex(kbRows[3]) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Flexed(4, func(gtx layout.Context) layout.Dimensions {
					return k.key(gtx, th, &k.space, "spatie", Mocha.Surface1, Mocha.Text)
				}),
				layout.Flexed(2, func(gtx layout.Context) layout.Dimensions {
					return k.key(gtx, th, &k.backspace, "Wis", Mocha.Surface2, Mocha.Text)
				}),
				layout.Flexed(2, func(gtx layout.Context) layout.Dimensions {
					return k.key(gtx, th, &k.search, "Zoek", Mocha.Blue, Mocha.Crust)
				}),
			)
		}),
	)
	return dims, searchTapped
}

func (k *keyboard) key(gtx layout.Context, th *material.Theme, click *widget.Clickable, label string, bg, fg color.NRGBA) layout.Dimensions {
	return layout.UniformInset(unit.Dp(3)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		b := material.Button(th, click, label)
		b.Background = bg
		b.Color = fg
		b.TextSize = unit.Sp(24)
		b.CornerRadius = unit.Dp(8)
		b.Inset = layout.UniformInset(unit.Dp(14))
		return b.Layout(gtx)
	})
}
