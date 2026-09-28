package splash

import "testing"

func TestScreensAreDistinctPortraitIPhones(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range Screens {
		if s.Width >= s.Height || s.Scale < 2 || s.Scale > 3 {
			t.Errorf("%+v is not an iPhone portrait screen", s)
		}
		if seen[s.File()] {
			t.Errorf("%s twice", s.File())
		}
		seen[s.File()] = true
	}
	if len(Screens) != 11 {
		t.Errorf("%d screens, want the 11 current iPhone sizes", len(Screens))
	}
}

func TestScreenFileAndMedia(t *testing.T) {
	s := Screen{Width: 440, Height: 956, Scale: 3}
	if s.File() != "splash-1320x2868.png" || s.Pixels() != [2]int{1320, 2868} {
		t.Fatalf("file %q pixels %v", s.File(), s.Pixels())
	}
	want := "(device-width: 440px) and (device-height: 956px) and " +
		"(-webkit-device-pixel-ratio: 3) and (orientation: portrait)"
	if s.Media() != want {
		t.Fatalf("media %q", s.Media())
	}
	if ArtShare <= 0 || ArtShare >= 1 {
		t.Fatal("art share out of range")
	}
}
