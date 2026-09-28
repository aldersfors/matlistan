package main

import (
	"image"
	"image/color"
	"image/draw"
	"testing"
)

var (
	_red  = color.RGBA{0xff, 0, 0, 0xff}
	_blue = color.RGBA{0, 0, 0xff, 0xff}
)

// split is red left of column at and blue from it on.
func split(size, at int) *image.RGBA {
	src := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(src, image.Rect(0, 0, at, size), &image.Uniform{_red}, image.Point{}, draw.Src)
	draw.Draw(src, image.Rect(at, 0, size, size), &image.Uniform{_blue}, image.Point{}, draw.Src)
	return src
}

// Downscaling averages the source area behind each pixel, so solid areas keep their colour
// and a pixel straddling an edge gets a mix.
func TestResizeAverages(t *testing.T) {
	img := resize(split(2048, 1030), 180) // output pixel 90 covers source 1024 to 1035.4
	if img.Bounds().Dx() != 180 || img.Bounds().Dy() != 180 {
		t.Fatalf("size %v", img.Bounds())
	}
	if img.RGBAAt(10, 90) != _red || img.RGBAAt(170, 90) != _blue {
		t.Fatalf("edges: %v %v", img.RGBAAt(10, 90), img.RGBAAt(170, 90))
	}
	if mid := img.RGBAAt(90, 90); mid.R == 0 || mid.B == 0 {
		t.Fatalf("pixel on the edge is not a mix: %v", mid)
	}
}

// A maskable icon keeps its art inside the centre circle Android may crop to: the art is
// scaled down onto the background, which fills the corners.
func TestMaskableKeepsTheArtInTheSafeZone(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2048, 2048))
	draw.Draw(src, src.Bounds(), &image.Uniform{_red}, image.Point{}, draw.Src)
	img := icon(src, 512, 0.7, _blue)
	if img.RGBAAt(0, 0) != _blue || img.RGBAAt(511, 511) != _blue || img.RGBAAt(40, 256) != _blue {
		t.Fatalf("corners or margin are not the background: %v %v %v", img.RGBAAt(0, 0),
			img.RGBAAt(511, 511), img.RGBAAt(40, 256))
	}
	if img.RGBAAt(256, 256) != _red || img.RGBAAt(100, 256) != _red {
		t.Fatalf("art missing: %v %v", img.RGBAAt(256, 256), img.RGBAAt(100, 256))
	}
}

// A launch image is the screen's size in navy, with the art centred at ArtShare of its width.
func TestSplashCentresTheArt(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2048, 2048))
	draw.Draw(src, src.Bounds(), &image.Uniform{_red}, image.Point{}, draw.Src)
	img := splashImage(src, 750, 1334, _blue)
	if img.Bounds().Dx() != 750 || img.Bounds().Dy() != 1334 {
		t.Fatalf("size %v", img.Bounds())
	}
	if img.RGBAAt(0, 0) != _blue || img.RGBAAt(749, 1333) != _blue || img.RGBAAt(375, 400) != _blue {
		t.Fatalf("background: %v %v %v", img.RGBAAt(0, 0), img.RGBAAt(749, 1333), img.RGBAAt(375, 400))
	}
	if img.RGBAAt(375, 667) != _red {
		t.Fatalf("centre is not the art: %v", img.RGBAAt(375, 667))
	}
	// 36% of 750 is 270 px wide, so the art spans x 240 to 509.
	if img.RGBAAt(245, 667) != _red || img.RGBAAt(235, 667) != _blue {
		t.Fatalf("art edges: %v %v", img.RGBAAt(245, 667), img.RGBAAt(235, 667))
	}
}
