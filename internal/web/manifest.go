package web

import (
	"encoding/json"
	"net/http"
)

// Manifest colours match the light palette in web/styles/input.css; a manifest cannot read
// CSS variables.
const (
	_manifestBackground = "#eef2f6"
	_manifestTheme      = "#0d1b2a"
)

type manifestIcon struct {
	Src     string `json:"src"`
	Sizes   string `json:"sizes"`
	Type    string `json:"type"`
	Purpose string `json:"purpose,omitempty"`
}

func (s *server) manifest(w http.ResponseWriter, _ *http.Request) {
	c := s.Catalog
	m := map[string]any{
		"name": c.T("app.name"), "short_name": c.T("app.name"),
		"description": c.T("app.description"), "lang": string(c.Locale()),
		"start_url": "/week", "scope": "/", "display": "standalone",
		"background_color": _manifestBackground, "theme_color": _manifestTheme,
		"icons": []manifestIcon{
			{Src: "/static/icons/icon-192.png", Sizes: "192x192", Type: "image/png"},
			{Src: "/static/icons/icon-512.png", Sizes: "512x512", Type: "image/png"},
			{Src: "/static/icons/icon-maskable-512.png", Sizes: "512x512", Type: "image/png",
				Purpose: "maskable"},
		},
	}
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_ = json.NewEncoder(w).Encode(m)
}
