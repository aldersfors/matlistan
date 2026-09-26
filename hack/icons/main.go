// Command icons draws the app icons: the week strip, seven rounded day bars on the light
// page colour. Run it with
// `mise run icons` after a palette change; the PNGs are committed.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

// Palette values from web/styles/input.css (light mode): the icon cannot read CSS.
var (
	_page = color.RGBA{0xee, 0xf2, 0xf6, 0xff}
	_days = []color.RGBA{{0x8a, 0xb8, 0xd0, 0xff}, {0x5a, 0x8f, 0xa8, 0xff},
		{0x3a, 0x65, 0x82, 0xff}, {0x2a, 0x4f, 0x6b, 0xff}, {0x1f, 0x30, 0x58, 0xff},
		{0xd8, 0xb8, 0x5e, 0xff}, {0x7a, 0x5f, 0x10, 0xff}}
)

// drawIcon renders a size x size icon; inset is the share of each side kept empty
// (maskable icons need a larger safe zone).
func drawIcon(size int, inset float64) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), &image.Uniform{_page}, image.Point{}, draw.Src)
	pad := int(float64(size) * inset)
	inner := size - 2*pad
	gap := inner / 28
	barW := (inner - gap*6) / 7
	h := inner * 3 / 4
	top := pad + (inner-h)/2
	x := pad + (inner-(barW*7+gap*6))/2
	for _, c := range _days {
		pill(img, image.Rect(x, top, x+barW, top+h), barW/2, c)
		x += barW + gap
	}
	return img
}

// pill fills r with c, rounding its corners with radius rad.
func pill(img *image.RGBA, r image.Rectangle, rad int, c color.RGBA) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			cx := min(max(x, r.Min.X+rad), r.Max.X-rad-1)
			cy := min(max(y, r.Min.Y+rad), r.Max.Y-rad-1)
			if dx, dy := x-cx, y-cy; dx*dx+dy*dy <= rad*rad {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: icons <output dir>")
		os.Exit(2)
	}
	dir := os.Args[1]
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // build tool output dir
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for name, spec := range map[string]struct {
		size  int
		inset float64
	}{"icon-180.png": {180, 0.08}, "icon-192.png": {192, 0.08}, "icon-512.png": {512, 0.08},
		"icon-maskable-512.png": {512, 0.2}} {
		f, err := os.Create(filepath.Join(dir, name)) //nolint:gosec // build tool output path
		if err == nil {
			err = png.Encode(f, drawIcon(spec.size, spec.inset))
			if cerr := f.Close(); err == nil {
				err = cerr
			}
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, name, err)
			os.Exit(1)
		}
	}
}
