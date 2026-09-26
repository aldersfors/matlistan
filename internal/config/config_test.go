package config

import (
	"strings"
	"testing"

	"github.com/jalet/matlistan/internal/i18n"
)

func valid() map[string]string {
	return map[string]string{
		"MATLISTAN_BASE_URL":                "https://matlistan.example.lan",
		"MATLISTAN_DATABASE_URL":            "postgres://u@db/matlistan",
		"MATLISTAN_SESSION_KEY_FILE":        "/secrets/session",
		"MATLISTAN_OIDC_ISSUER":             "https://idp.example.lan/realms/home",
		"MATLISTAN_OIDC_CLIENT_ID":          "matlistan",
		"MATLISTAN_OIDC_CLIENT_SECRET_FILE": "/secrets/oidc",
		"MATLISTAN_OIDC_ALLOWED":            "family, admins",
	}
}

func getenv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestParseDefaults(t *testing.T) {
	c, err := Parse(getenv(valid()))
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != ":8080" || c.MetricsAddr != ":9091" || c.Locale != i18n.EN || c.Location.String() != "UTC" ||
		c.OIDC.Claim != "groups" {
		t.Errorf("defaults = %+v", c)
	}
	if strings.Join(c.OIDC.Allowed, "|") != "family|admins" {
		t.Errorf("allowed = %q", c.OIDC.Allowed)
	}
	if c.RedirectURL() != "https://matlistan.example.lan/auth/callback" {
		t.Errorf("redirect = %q", c.RedirectURL())
	}
}

func TestParseHomelab(t *testing.T) {
	m := valid()
	m["MATLISTAN_LOCALE"] = "sv"
	m["MATLISTAN_TIMEZONE"] = "Europe/Stockholm"
	c, err := Parse(getenv(m))
	if err != nil {
		t.Fatal(err)
	}
	if c.Locale != i18n.SV || c.Location.String() != "Europe/Stockholm" {
		t.Errorf("got %q %q", c.Locale, c.Location)
	}
}

func TestParseReportsEveryMissingVariable(t *testing.T) {
	_, err := Parse(getenv(map[string]string{}))
	if err == nil {
		t.Fatal("want error")
	}
	for _, name := range []string{"MATLISTAN_BASE_URL", "MATLISTAN_DATABASE_URL",
		"MATLISTAN_SESSION_KEY_FILE", "MATLISTAN_OIDC_ISSUER", "MATLISTAN_OIDC_CLIENT_ID",
		"MATLISTAN_OIDC_CLIENT_SECRET_FILE", "MATLISTAN_OIDC_ALLOWED"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error does not name %s: %v", name, err)
		}
	}
}

func TestParseRejects(t *testing.T) {
	cases := map[string][2]string{
		"http base url": {"MATLISTAN_BASE_URL", "http://matlistan.example.lan"},
		"bad locale":    {"MATLISTAN_LOCALE", "de"},
		"bad timezone":  {"MATLISTAN_TIMEZONE", "Mars/Olympus"},
	}
	for name, kv := range cases {
		m := valid()
		m[kv[0]] = kv[1]
		if _, err := Parse(getenv(m)); err == nil || !strings.Contains(err.Error(), kv[0]) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestParseAllowsHTTPLocalhost(t *testing.T) {
	m := valid()
	m["MATLISTAN_BASE_URL"] = "http://localhost:8080"
	if _, err := Parse(getenv(m)); err != nil {
		t.Fatal(err)
	}
}

func TestParseDatabaseOnlyNeedsTheURL(t *testing.T) {
	d, err := ParseDatabase(getenv(map[string]string{"MATLISTAN_DATABASE_URL": "postgres://x"}))
	if err != nil || d.URL != "postgres://x" {
		t.Fatalf("d = %+v, err = %v", d, err)
	}
	if _, err := ParseDatabase(getenv(map[string]string{})); err == nil {
		t.Fatal("want error")
	}
}

func TestPlannerConfig(t *testing.T) {
	c, err := Parse(getenv(valid()))
	if err != nil || c.Planner.Model != "claude-opus-5" || c.Planner.APIKeyFile != "" {
		t.Fatalf("defaults = %+v, %v", c.Planner, err)
	}
	m := valid()
	m["MATLISTAN_ANTHROPIC_API_KEY_FILE"] = "/secrets/anthropic"
	m["MATLISTAN_MODEL"] = "claude-sonnet-5"
	if c, _ := Parse(getenv(m)); c.Planner.APIKeyFile != "/secrets/anthropic" ||
		c.Planner.Model != "claude-sonnet-5" {
		t.Fatalf("planner = %+v", c.Planner)
	}
}

func TestParseGenerateNeedsTheKey(t *testing.T) {
	m := map[string]string{"MATLISTAN_DATABASE_URL": "postgres://x"}
	if _, err := ParseGenerate(getenv(m)); err == nil ||
		!strings.Contains(err.Error(), "MATLISTAN_ANTHROPIC_API_KEY_FILE") {
		t.Fatalf("err = %v", err)
	}
	m["MATLISTAN_ANTHROPIC_API_KEY_FILE"] = "/k"
	m["MATLISTAN_LOCALE"] = "sv"
	g, err := ParseGenerate(getenv(m))
	if err != nil || g.Locale != i18n.SV || g.Planner.Model != "claude-opus-5" {
		t.Fatalf("generate = %+v, %v", g, err)
	}
}
