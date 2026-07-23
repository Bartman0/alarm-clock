// Package ui contains the Gio-based screens and the Catppuccin Mocha theme
// used throughout the alarm clock.
package ui

import (
	"image/color"

	"gioui.org/font/gofont"
	"gioui.org/text"
	"gioui.org/widget/material"
)

// Mocha is the Catppuccin Mocha palette.
// https://github.com/catppuccin/catppuccin
var Mocha = struct {
	Rosewater, Flamingo, Pink, Mauve, Red, Maroon, Peach, Yellow, Green,
	Teal, Sky, Sapphire, Blue, Lavender,
	Text, Subtext1, Subtext0, Overlay2, Overlay1, Overlay0,
	Surface2, Surface1, Surface0, Base, Mantle, Crust color.NRGBA
}{
	Rosewater: hex(0xf5e0dc),
	Flamingo:  hex(0xf2cdcd),
	Pink:      hex(0xf5c2e7),
	Mauve:     hex(0xcba6f7),
	Red:       hex(0xf38ba8),
	Maroon:    hex(0xeba0ac),
	Peach:     hex(0xfab387),
	Yellow:    hex(0xf9e2af),
	Green:     hex(0xa6e3a1),
	Teal:      hex(0x94e2d5),
	Sky:       hex(0x89dceb),
	Sapphire:  hex(0x74c7ec),
	Blue:      hex(0x89b4fa),
	Lavender:  hex(0xb4befe),
	Text:      hex(0xcdd6f4),
	Subtext1:  hex(0xbac2de),
	Subtext0:  hex(0xa6adc8),
	Overlay2:  hex(0x9399b2),
	Overlay1:  hex(0x7f849c),
	Overlay0:  hex(0x6c7086),
	Surface2:  hex(0x585b70),
	Surface1:  hex(0x45475a),
	Surface0:  hex(0x313244),
	Base:      hex(0x1e1e2e),
	Mantle:    hex(0x181825),
	Crust:     hex(0x11111b),
}

// hex converts a 0xRRGGBB literal into an opaque NRGBA colour.
func hex(c uint32) color.NRGBA {
	return color.NRGBA{
		R: uint8(c >> 16),
		G: uint8(c >> 8),
		B: uint8(c),
		A: 0xff,
	}
}

// NewTheme returns a material theme pre-styled with Catppuccin Mocha colours
// so the standard widgets render in-palette by default.
func NewTheme() *material.Theme {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	th.Palette = material.Palette{
		Bg:         Mocha.Base,
		Fg:         Mocha.Text,
		ContrastBg: Mocha.Mauve,
		ContrastFg: Mocha.Crust,
	}
	return th
}
