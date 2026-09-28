package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/aldersfors/matlistan/internal/push"
)

func noEnv(string) string { return "" }

func TestRunVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"matlistan", "version"}, &out, &errOut, noEnv); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}
	if got := strings.TrimSpace(out.String()); got != "dev" {
		t.Fatalf("version = %q, want dev", got)
	}
}

func TestRunWithoutCommandPrintsUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"matlistan"}, &out, &errOut, noEnv); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "usage: matlistan <generate|migrate|serve|vapid-keys|version>") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"matlistan", "bake"}, &out, &errOut, noEnv); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), `unknown command "bake"`) {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestMigrateWithoutDatabaseURL(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"matlistan", "migrate"}, &out, &errOut, noEnv); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "MATLISTAN_DATABASE_URL is required") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestServeReportsConfigErrors(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"matlistan", "serve"}, &out, &errOut, noEnv); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	for _, want := range []string{"MATLISTAN_BASE_URL is required", "MATLISTAN_OIDC_ISSUER is required"} {
		if !strings.Contains(errOut.String(), want) {
			t.Errorf("stderr lacks %q: %s", want, errOut.String())
		}
	}
}

func TestGenerateNeedsConfig(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"matlistan", "generate"}, &out, &errOut, noEnv); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(errOut.String(), "MATLISTAN_API_KEY_FILE is required") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestGenerateRejectsABadWeek(t *testing.T) {
	var out, errOut bytes.Buffer
	env := func(k string) string {
		return map[string]string{"MATLISTAN_DATABASE_URL": "postgres://x",
			"MATLISTAN_ANTHROPIC_API_KEY_FILE": "/k"}[k]
	}
	if code := run([]string{"matlistan", "generate", "--week", "2027-W53"}, &out, &errOut,
		env); code != 2 {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}
}
func TestVAPIDKeysPrintsAPair(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"matlistan", "vapid-keys"}, &out, &errOut, noEnv); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "private-key: ") || !strings.HasPrefix(lines[1], "public-key: ") {
		t.Fatalf("output %q", out.String())
	}
	priv := strings.TrimPrefix(lines[0], "private-key: ")
	if _, err := push.ParseKey(priv, "https://a.example"); err != nil {
		t.Fatalf("printed key does not parse: %v", err)
	}
}
