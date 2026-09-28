// Package config reads the process configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aldersfors/matlistan/internal/i18n"
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
	HouseholdFile                                         string
	Database                                              Database
	Locale                                                i18n.Locale
	Location                                              *time.Location
	OIDC                                                  OIDC
	Planner                                               Planner
	Push                                                  Push
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
		HouseholdFile:  get("MATLISTAN_HOUSEHOLD_FILE"),
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
	c.Planner = parsePlanner(get, &errs)
	c.Push = parsePush(get, c.BaseURL, &errs)
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

// Planner configures the model provider. Planning is off unless Enabled.
type Planner struct {
	Provider   string // ProviderAnthropic or ProviderOpenAI
	Model      string
	BaseURL    string // openai only; "" means DefaultOpenAIBaseURL
	APIKeyFile string
	// MaxOutputTokens caps an OpenAI answer; smaller models and servers need less than the
	// default.
	MaxOutputTokens int64
}

// Model providers.
const (
	ProviderAnthropic    = "anthropic"
	ProviderOpenAI       = "openai"
	DefaultOpenAIBaseURL = "https://api.openai.com/v1"
)

// Enabled reports whether planning can run: with a key file, or against an
// OpenAI-compatible server other than api.openai.com, which may need no key.
func (p Planner) Enabled() bool {
	if p.APIKeyFile != "" {
		return true
	}
	if p.Provider != ProviderOpenAI || p.BaseURL == "" {
		return false
	}
	u, err := url.Parse(p.BaseURL)
	return err == nil && u.Hostname() != "api.openai.com"
}

// OpenAIBaseURL is the Chat Completions base URL.
func (p Planner) OpenAIBaseURL() string { return or(p.BaseURL, DefaultOpenAIBaseURL) }

func parsePlanner(get func(string) string, errs *[]error) Planner {
	p := Planner{Provider: or(get("MATLISTAN_PROVIDER"), ProviderAnthropic),
		Model: get("MATLISTAN_MODEL"), BaseURL: get("MATLISTAN_OPENAI_BASE_URL"),
		APIKeyFile: get("MATLISTAN_API_KEY_FILE")}
	if old := get("MATLISTAN_ANTHROPIC_API_KEY_FILE"); old != "" {
		if p.APIKeyFile != "" {
			*errs = append(*errs, errors.New(
				"MATLISTAN_ANTHROPIC_API_KEY_FILE: set only MATLISTAN_API_KEY_FILE"))
		}
		p.APIKeyFile = or(p.APIKeyFile, old)
	}
	p.MaxOutputTokens = 64000
	if raw := get("MATLISTAN_MAX_OUTPUT_TOKENS"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 1024 {
			*errs = append(*errs, errors.New("MATLISTAN_MAX_OUTPUT_TOKENS: a whole number, at least 1024"))
		}
		p.MaxOutputTokens = n
	}
	switch p.Provider {
	case ProviderAnthropic:
		p.Model = or(p.Model, "claude-opus-5")
		if p.BaseURL != "" {
			*errs = append(*errs, errors.New("MATLISTAN_OPENAI_BASE_URL: only for provider openai"))
		}
	case ProviderOpenAI:
		if p.Model == "" {
			*errs = append(*errs, errors.New("MATLISTAN_MODEL is required for provider openai"))
		}
		if p.BaseURL != "" {
			if err := checkModelURL(p.BaseURL); err != nil {
				*errs = append(*errs, err)
			}
		}
	default:
		*errs = append(*errs, fmt.Errorf("MATLISTAN_PROVIDER: %q is not anthropic or openai",
			p.Provider))
	}
	return p
}

// checkModelURL allows https anywhere and http only on this host or inside the cluster,
// for model servers that do not terminate TLS.
func checkModelURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return errors.New("MATLISTAN_OPENAI_BASE_URL: not an absolute URL")
	}
	h := u.Hostname()
	local := h == "localhost" || h == "127.0.0.1" || strings.HasSuffix(h, ".svc") ||
		strings.HasSuffix(h, ".svc.cluster.local")
	if u.Scheme != "https" && (u.Scheme != "http" || !local) {
		return errors.New("MATLISTAN_OPENAI_BASE_URL: must be https (http only for localhost " +
			"or a cluster service)")
	}
	return nil
}

// Generate is what the generate command needs.
type Generate struct {
	Database Database
	Locale   i18n.Locale
	Location *time.Location
	Planner  Planner
	BaseURL  string // optional; the default VAPID subject
	Push     Push
}

// ParseGenerate reads the database, locale, timezone and planner settings; the key is
// required.
func ParseGenerate(getenv func(string) string) (Generate, error) {
	get := func(name string) string { return strings.TrimSpace(getenv(name)) }
	var errs []error
	db, err := ParseDatabase(getenv)
	if err != nil {
		errs = append(errs, err)
	}
	g := Generate{Database: db, Planner: parsePlanner(get, &errs)}
	if !g.Planner.Enabled() {
		errs = append(errs, errors.New("MATLISTAN_API_KEY_FILE is required"))
	}
	if g.Locale, err = i18n.ParseLocale(get("MATLISTAN_LOCALE")); err != nil {
		errs = append(errs, fmt.Errorf("MATLISTAN_LOCALE: %w", err))
	}
	if g.Location, err = time.LoadLocation(or(get("MATLISTAN_TIMEZONE"), "UTC")); err != nil {
		errs = append(errs, fmt.Errorf("MATLISTAN_TIMEZONE: %w", err))
	}
	if g.BaseURL = get("MATLISTAN_BASE_URL"); g.BaseURL != "" {
		if err := checkBaseURL(g.BaseURL); err != nil {
			errs = append(errs, err)
		}
	}
	g.Push = parsePush(get, g.BaseURL, &errs)
	if err := errors.Join(errs...); err != nil {
		return Generate{}, err
	}
	return g, nil
}

// Push is web push: on when KeyFile is set. Subject is the VAPID contact.
type Push struct{ KeyFile, Subject string }

// Enabled reports whether web push is configured.
func (p Push) Enabled() bool { return p.KeyFile != "" }

func parsePush(get func(string) string, baseURL string, errs *[]error) Push {
	p := Push{KeyFile: get("MATLISTAN_VAPID_KEY_FILE"),
		Subject: or(get("MATLISTAN_VAPID_SUBJECT"), baseURL)}
	if !p.Enabled() {
		return Push{}
	}
	u, err := url.Parse(p.Subject)
	https := err == nil && u.Scheme == "https" && u.Host != ""
	mailto := err == nil && u.Scheme == "mailto" && u.Opaque != ""
	switch {
	case p.Subject == "":
		*errs = append(*errs, errors.New("MATLISTAN_VAPID_SUBJECT or MATLISTAN_BASE_URL is "+
			"required with MATLISTAN_VAPID_KEY_FILE"))
	case !https && !mailto:
		*errs = append(*errs, errors.New("MATLISTAN_VAPID_SUBJECT: must be a mailto: or https: URL"))
	}
	return p
}
