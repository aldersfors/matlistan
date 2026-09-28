// Package splash lists the iPhone screens that get an iOS launch image. iOS uses a
// startup image only when its size matches the screen exactly, so there is one per screen.
// Sizes are portrait points at the device's scale, current as of the iPhone 17 family
// (2025).
package splash

import "fmt"

// Screen is one iPhone screen in points and its scale factor.
type Screen struct{ Width, Height, Scale int }

// ArtShare is the artwork's width as a share of the screen width, on the launch image and
// on the in-app launch overlay alike, so the two line up.
const ArtShare = 0.36

// Background is the artwork's navy, which fills the launch image around it.
const Background = "#0d1c28"

// Screens are the distinct iPhone portrait screens, newest first.
var Screens = []Screen{
	{440, 956, 3}, // 16 Pro Max, 17 Pro Max
	{420, 912, 3}, // Air
	{402, 874, 3}, // 16 Pro, 17, 17 Pro
	{430, 932, 3}, // 14 Pro Max, 15 Plus, 15 Pro Max, 16 Plus
	{393, 852, 3}, // 14 Pro, 15, 15 Pro, 16
	{428, 926, 3}, // 12 Pro Max, 13 Pro Max, 14 Plus
	{390, 844, 3}, // 12, 12 Pro, 13, 13 Pro, 14, 16e
	{375, 812, 3}, // X, XS, 11 Pro, 12 mini, 13 mini
	{414, 896, 3}, // XS Max, 11 Pro Max
	{414, 896, 2}, // XR, 11
	{375, 667, 2}, // 8, SE (2nd and 3rd generation)
}

// Pixels is the launch image's size.
func (s Screen) Pixels() [2]int { return [2]int{s.Width * s.Scale, s.Height * s.Scale} }

// File is the launch image's name under /static/splash/.
func (s Screen) File() string {
	p := s.Pixels()
	return fmt.Sprintf("splash-%dx%d.png", p[0], p[1])
}

// Media is the query that picks this image in an apple-touch-startup-image link.
func (s Screen) Media() string {
	return fmt.Sprintf("(device-width: %dpx) and (device-height: %dpx) and "+
		"(-webkit-device-pixel-ratio: %d) and (orientation: portrait)", s.Width, s.Height, s.Scale)
}
