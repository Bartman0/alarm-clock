package ui

import (
	"image"
	"image/color"
	"math"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

// AnalogClock draws a Catppuccin-styled analog clock face with hour, minute
// and second hands. It draws the largest square that fits the constraints,
// centred, and returns that square as its dimensions.
func AnalogClock(gtx layout.Context, t time.Time) layout.Dimensions {
	d := gtx.Constraints.Max.X
	if gtx.Constraints.Max.Y < d {
		d = gtx.Constraints.Max.Y
	}
	if d <= 0 {
		return layout.Dimensions{}
	}
	fd := float32(d)
	center := f32.Pt(fd/2, fd/2)
	radius := fd/2 - fd*0.04

	// Face: filled disc with a ring.
	fillCircle(gtx, center, radius, Mocha.Surface0)
	strokeCircle(gtx, center, radius, fd*0.012, Mocha.Surface2)

	// Hour ticks.
	for i := 0; i < 12; i++ {
		angle := float64(i) / 12 * 2 * math.Pi
		outer := radius * 0.96
		inner := radius * 0.86
		w := fd * 0.008
		col := Mocha.Overlay1
		if i%3 == 0 { // emphasise 12/3/6/9
			inner = radius * 0.80
			w = fd * 0.016
			col = Mocha.Overlay2
		}
		drawSpoke(gtx, center, angle, inner, outer, w, col)
	}

	hour := float64(t.Hour()%12) + float64(t.Minute())/60
	minute := float64(t.Minute()) + float64(t.Second())/60
	second := float64(t.Second())

	// Hands.
	drawHand(gtx, center, hour/12*2*math.Pi, radius*0.50, fd*0.022, Mocha.Text)
	drawHand(gtx, center, minute/60*2*math.Pi, radius*0.78, fd*0.014, Mocha.Lavender)
	drawHand(gtx, center, second/60*2*math.Pi, radius*0.85, fd*0.006, Mocha.Red)

	// Centre cap.
	fillCircle(gtx, center, fd*0.02, Mocha.Red)

	return layout.Dimensions{Size: image.Pt(d, d)}
}

// endpoint returns the point at the given clock angle (0 = up, clockwise) and
// distance from centre.
func endpoint(center f32.Point, angle float64, dist float32) f32.Point {
	return f32.Pt(
		center.X+dist*float32(math.Sin(angle)),
		center.Y-dist*float32(math.Cos(angle)),
	)
}

func drawHand(gtx layout.Context, center f32.Point, angle float64, length, width float32, col color.NRGBA) {
	drawSpoke(gtx, center, angle, 0, length, width, col)
}

// drawSpoke strokes a line at the given angle between two radii from centre.
func drawSpoke(gtx layout.Context, center f32.Point, angle float64, from, to, width float32, col color.NRGBA) {
	var p clip.Path
	p.Begin(gtx.Ops)
	p.MoveTo(endpoint(center, angle, from))
	p.LineTo(endpoint(center, angle, to))
	paint.FillShape(gtx.Ops, col, clip.Stroke{Path: p.End(), Width: width}.Op())
}

func fillCircle(gtx layout.Context, center f32.Point, r float32, col color.NRGBA) {
	rect := image.Rect(int(center.X-r), int(center.Y-r), int(center.X+r), int(center.Y+r))
	defer clip.Ellipse(rect).Push(gtx.Ops).Pop()
	paint.ColorOp{Color: col}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
}

func strokeCircle(gtx layout.Context, center f32.Point, r, width float32, col color.NRGBA) {
	// Approximate a circular stroke by filling the annulus between two discs.
	fillCircle(gtx, center, r+width/2, col)
	fillCircle(gtx, center, r-width/2, Mocha.Surface0)
}
