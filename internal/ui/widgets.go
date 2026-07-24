package ui

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// flexSpacer fills the whole slot a Flexed child is allocated (unlike an empty
// widget, which would collapse to zero and shift the layout).
func flexSpacer(gtx layout.Context) layout.Dimensions {
	return layout.Dimensions{Size: gtx.Constraints.Min}
}

// colorIf returns yes when cond is true, otherwise no.
func colorIf(cond bool, yes, no color.NRGBA) color.NRGBA {
	if cond {
		return yes
	}
	return no
}

// roundedPanel lays out w on top of a rounded-rectangle background of colour bg
// sized to w.
func roundedPanel(gtx layout.Context, bg color.NRGBA, radius unit.Dp, w layout.Widget) layout.Dimensions {
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			r := gtx.Dp(radius)
			rect := image.Rectangle{Max: gtx.Constraints.Min}
			defer clip.RRect{Rect: rect, SE: r, SW: r, NE: r, NW: r}.Push(gtx.Ops).Pop()
			paint.Fill(gtx.Ops, bg)
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
		layout.Stacked(w),
	)
}

// header renders a screen title with a "‹ Terug" (back) button on the left.
func header(gtx layout.Context, th *material.Theme, back *widget.Clickable, title string) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			b := material.Button(th, back, "‹ Terug")
			b.Background = Mocha.Surface0
			b.Color = Mocha.Text
			b.TextSize = unit.Sp(22)
			b.CornerRadius = unit.Dp(12)
			b.Inset = layout.UniformInset(unit.Dp(14))
			return b.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(24)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := material.Label(th, unit.Sp(36), title)
			l.Color = Mocha.Text
			l.Font.Weight = 600
			return l.Layout(gtx)
		}),
	)
}

// segButton renders a selectable pill; when selected it uses the accent fill.
func segButton(gtx layout.Context, th *material.Theme, click *widget.Clickable, label string, selected bool, accent color.NRGBA) layout.Dimensions {
	b := material.Button(th, click, label)
	if selected {
		b.Background = accent
		b.Color = Mocha.Crust
	} else {
		b.Background = Mocha.Surface0
		b.Color = Mocha.Subtext1
	}
	b.TextSize = unit.Sp(22)
	b.CornerRadius = unit.Dp(12)
	b.Inset = layout.Inset{Top: unit.Dp(14), Bottom: unit.Dp(14), Left: unit.Dp(22), Right: unit.Dp(22)}
	return b.Layout(gtx)
}

// stepButton renders a large square +/- stepper control.
func stepButton(gtx layout.Context, th *material.Theme, click *widget.Clickable, label string) layout.Dimensions {
	b := material.Button(th, click, label)
	b.Background = Mocha.Surface1
	b.Color = Mocha.Text
	b.TextSize = unit.Sp(44)
	b.CornerRadius = unit.Dp(12)
	b.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(10), Left: unit.Dp(28), Right: unit.Dp(28)}
	return b.Layout(gtx)
}
