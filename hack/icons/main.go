// Command icons makes the app icons and the iOS launch images from the source artwork,
// app-icon-2048.png (drawn from app-icon.svg in this directory), and copies the SVG for the
// in-app launch overlay. Run it with `mise run icons` after changing the artwork; the output
// is committed. It uses the standard library only, so it averages the source
// area behind each output pixel instead of using a resampling filter.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"

	"github.com/aldersfors/matlistan/internal/splash"
)

// resize scales src down to size x size by averaging the source area each output pixel
// covers, weighting partly covered source pixels by their overlap.
func resize(src image.Image, size int) *image.RGBA {
	b := src.Bounds()
	rx := float64(b.Dx()) / float64(size)
	ry := float64(b.Dy()) / float64(size)
	out := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		y0, y1 := float64(y)*ry, float64(y+1)*ry
		for x := range size {
			x0, x1 := float64(x)*rx, float64(x+1)*rx
			var r, g, bl, a, w float64
			for sy := int(y0); float64(sy) < y1 && sy < b.Dy(); sy++ {
				wy := min(y1, float64(sy+1)) - max(y0, float64(sy))
				for sx := int(x0); float64(sx) < x1 && sx < b.Dx(); sx++ {
					wx := min(x1, float64(sx+1)) - max(x0, float64(sx))
					cr, cg, cb, ca := src.At(b.Min.X+sx, b.Min.Y+sy).RGBA() // premultiplied
					k := wx * wy
					r, g, bl, a, w = r+k*float64(cr), g+k*float64(cg), bl+k*float64(cb),
						a+k*float64(ca), w+k
				}
			}
			out.SetRGBA(x, y, color.RGBA{R: to8(r / w), G: to8(g / w), B: to8(bl / w),
				A: to8(a / w)})
		}
	}
	return out
}

// to8 rounds a 16-bit channel to 8 bits; truncating would turn full colour into 254.
func to8(v float64) uint8 { return uint8(min(v/257+0.5, 255)) }

// icon renders the artwork at size x size. Scale below 1 shrinks the art onto bg and centres
// it, which a maskable icon needs: Android may crop it to a circle of 80% of its width.
func icon(src image.Image, size int, scale float64, bg color.RGBA) *image.RGBA {
	if scale >= 1 {
		return resize(src, size)
	}
	out := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(out, out.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)
	inner := int(float64(size)*scale + 0.5)
	off := (size - inner) / 2
	draw.Draw(out, image.Rect(off, off, off+inner, off+inner), resize(src, inner),
		image.Point{}, draw.Over)
	return out
}

// splashImage is a w x h launch image: the art centred at splash.ArtShare of the width, on
// the art's own background colour.
func splashImage(src image.Image, w, h int, bg color.RGBA) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(out, out.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)
	art := int(float64(w)*splash.ArtShare + 0.5)
	x, y := (w-art)/2, (h-art)/2
	draw.Draw(out, image.Rect(x, y, x+art, y+art), resize(src, art), image.Point{}, draw.Over)
	return out
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: icons <artwork dir> <static dir>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "icons:", err)
		os.Exit(1)
	}
}

func run(artDir, static string) error {
	//nolint:gosec // build tool input
	f, err := os.Open(filepath.Join(artDir, "app-icon-2048.png"))
	if err != nil {
		return err
	}
	src, err := png.Decode(f)
	_ = f.Close()
	if err != nil {
		return fmt.Errorf("app-icon-2048.png: %w", err)
	}
	// The artwork's background fills the whole square; margins and launch images use it too.
	bg := color.RGBAModel.Convert(src.At(src.Bounds().Min.X, src.Bounds().Min.Y)).(color.RGBA)
	icons, splashes := filepath.Join(static, "icons"), filepath.Join(static, "splash")
	for _, d := range []string{icons, splashes} {
		if err := os.MkdirAll(d, 0o755); err != nil { //nolint:gosec // build tool output dir
			return err
		}
	}
	for name, spec := range map[string]struct {
		size  int
		scale float64
	}{"icon-180.png": {180, 1}, "icon-192.png": {192, 1}, "icon-512.png": {512, 1},
		"icon-maskable-512.png": {512, 0.7}} {
		img := icon(src, spec.size, spec.scale, bg)
		if err := writePNG(filepath.Join(icons, name), img); err != nil {
			return err
		}
	}
	for _, s := range splash.Screens {
		p := s.Pixels()
		img := splashImage(src, p[0], p[1], bg)
		if err := writePNG(filepath.Join(splashes, s.File()), img); err != nil {
			return err
		}
	}
	svg, err := os.ReadFile(filepath.Join(artDir, "app-icon.svg")) //nolint:gosec // build tool input
	if err != nil {
		return err
	}
	//nolint:gosec // a public asset, served as-is
	return os.WriteFile(filepath.Join(splashes, "app-icon.svg"), svg, 0o644)
}

func writePNG(path string, img image.Image) error {
	out, err := os.Create(path) //nolint:gosec // build tool output path
	if err != nil {
		return err
	}
	err = png.Encode(out, img)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return nil
}
