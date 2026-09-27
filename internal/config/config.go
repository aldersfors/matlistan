// Package config reads the process configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jalet/matlistan/internal/i18n"
)

// OIDC configures sign-in.
type OIDC struct {
	Issuer, ClientID, ClientSecretFile, CAFile, Claim string
	Allowed                                           []string
}

// Database locates Postgres.
type Database struct{ URL, CAFile string }

// Config is everything serve needs.
type Config struct {
	Addr, MetricsAddr, BaseURL, ThemeFile, SessionKeyFile string
	Database                                              Database
	Locale                                                i18n.Locale
	Location                                              *time.Location
	OIDC                                                  OIDC
}

// RedirectURL is the OIDC callback under BaseURL.
func (c Config) RedirectURL() string {
	return strings.TrimSuffix(c.BaseURL, "/") + "/auth/callback"
}

// ParseDatabase reads only the database settings, for the migrate command.
func ParseDatabase(getenv func(string) string) (Database, error) {
	d := Database{URL: strings.TrimSpace(getenv("MATLISTAN_DATABASE_URL")),
		CAFile: strings.TrimSpace(getenv("MATLISTAN_DATABASE_CA_FILE"))}
	if d.URL == "" {
		return Database{}, errors.New("MATLISTAN_DATABASE_URL is required")
	}
	return d, nil
}

// Parse reads and validates the full configuration, reporting every problem at once.
func Parse(getenv func(string) string) (Config, error) {
	var errs []error
	get := func(name string) string { return strings.TrimSpace(getenv(name)) }
	req := func(name string) string {
		v := get(name)
		if v == "" {
			errs = append(errs, fmt.Errorf("%s is required", name))
		}
		return v
	}
	c := Config{
		Addr:        or(get("MATLISTAN_ADDR"), ":8080"),
		MetricsAddr: or(get("MATLISTAN_METRICS_ADDR"), ":9091"),
		BaseURL:     req("MATLISTAN_BASE_URL"),
		Database: Database{
			URL:    req("MATLISTAN_DATABASE_URL"),
			CAFile: get("MATLISTAN_DATABASE_CA_FILE"),
		},
		ThemeFile:      get("MATLISTAN_THEME_FILE"),
		SessionKeyFile: req("MATLISTAN_SESSION_KEY_FILE"),
		OIDC: OIDC{
			Issuer:           req("MATLISTAN_OIDC_ISSUER"),
			ClientID:         req("MATLISTAN_OIDC_CLIENT_ID"),
			ClientSecretFile: req("MATLISTAN_OIDC_CLIENT_SECRET_FILE"),
			CAFile:           get("MATLISTAN_OIDC_CA_FILE"),
			Claim:            or(get("MATLISTAN_OIDC_CLAIM"), "groups"),
			Allowed:          splitList(req("MATLISTAN_OIDC_ALLOWED")),
		},
	}
	var err error
	if c.Locale, err = i18n.ParseLocale(get("MATLISTAN_LOCALE")); err != nil {
		errs = append(errs, fmt.Errorf("MATLISTAN_LOCALE: %w", err))
	}
	if c.Location, err = time.LoadLocation(or(get("MATLISTAN_TIMEZONE"), "UTC")); err != nil {
		errs = append(errs, fmt.Errorf("MATLISTAN_TIMEZONE: %w", err))
	}
	if c.BaseURL != "" {
		if err := checkBaseURL(c.BaseURL); err != nil {
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return Config{}, err
	}
	return c, nil
}

func checkBaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return errors.New("MATLISTAN_BASE_URL: not an absolute URL")
	}
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"
	if u.Scheme != "https" && (u.Scheme != "http" || !local) {
		return errors.New("MATLISTAN_BASE_URL: must be https (http only for localhost)")
	}
	return nil
}

func or(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
