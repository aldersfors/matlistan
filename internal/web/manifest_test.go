package web

import (
	"encoding/json"
	"image/png"
	"os"
	"strings"
	"testing"

	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/splash"
)

func TestManifest(t *testing.T) {
	h := newServer(t, i18n.SV, false, newFakeStore()) // public: no session
	res, body := get(t, h, "/manifest.webmanifest")
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "application/manifest+json" {
		t.Fatalf("manifest: %d %q", res.StatusCode, res.Header.Get("Content-Type"))
	}
	var m struct {
		Name, Lang, Display, Description string
		StartURL                         string `json:"start_url"`
		Icons                            []struct{ Src, Sizes, Purpose string }
	}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatal(err)
	}
	if m.Name != "Matlistan" || m.Lang != "sv" || m.Display != "standalone" ||
		m.Description != "Veckans middagar och inköpslista." || len(m.Icons) != 3 ||
		m.StartURL != "/week?launch=1" {
		t.Fatalf("manifest = %+v", m)
	}
	for _, ic := range m.Icons {
		if res, _ := get(t, h, ic.Src); res.StatusCode != 200 {
			t.Errorf("icon %s: %d", ic.Src, res.StatusCode)
		}
	}
}

func TestIconsHaveTheirSizes(t *testing.T) {
	for name, size := range map[string]int{"icon-180.png": 180, "icon-192.png": 192,
		"icon-512.png": 512, "icon-maskable-512.png": 512} {
		f, err := os.Open("static/icons/" + name)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := png.DecodeConfig(f)
		_ = f.Close()
		if err != nil || cfg.Width != size || cfg.Height != size {
			t.Errorf("%s: %dx%d, %v", name, cfg.Width, cfg.Height, err)
		}
	}
}

func TestLayoutLinksTheApp(t *testing.T) {
	_, body := get(t, newServer(t, i18n.SV, true, newFakeStore()), "/week")
	for _, want := range []string{`rel="manifest" href="/manifest.webmanifest"`,
		`rel="apple-touch-icon" href="/static/icons/icon-180.png"`, `name="theme-color"`,
		`name="apple-mobile-web-app-capable" content="yes"`} {
		if !strings.Contains(body, want) {
			t.Errorf("head lacks %s", want)
		}
	}
}

// iOS shows a launch image only when one matches the screen exactly: every iPhone screen
// gets a link, and every linked image exists at that screen's pixel size.
func TestLayoutLinksALaunchImagePerScreen(t *testing.T) {
	_, body := get(t, newServer(t, i18n.SV, true, newFakeStore()), "/week")
	for _, s := range splash.Screens {
		link := `<link rel="apple-touch-startup-image" media="` + s.Media() +
			`" href="/static/splash/` + s.File() + `">`
		if !strings.Contains(body, link) {
			t.Errorf("head lacks %s", link)
		}
		f, err := os.Open("static/splash/" + s.File())
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := png.DecodeConfig(f)
		_ = f.Close()
		if p := s.Pixels(); err != nil || cfg.Width != p[0] || cfg.Height != p[1] {
			t.Errorf("%s: %dx%d, %v", s.File(), cfg.Width, cfg.Height, err)
		}
	}
}

// The navy launch overlay shows only when the Home Screen app starts, never on navigation.
func TestLaunchOverlayOnlyWhenTheAppStarts(t *testing.T) {
	h := newServer(t, i18n.SV, true, newFakeStore())
	if _, body := get(t, h, "/week?launch=1"); !strings.Contains(body, `class="launch"`) ||
		!strings.Contains(body, `src="/static/splash/app-icon.svg"`) {
		t.Fatal("no launch overlay on launch")
	}
	if _, body := get(t, h, "/week"); strings.Contains(body, `class="launch"`) {
		t.Fatal("launch overlay on a normal visit")
	}
}
