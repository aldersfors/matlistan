package main

import (
	"bytes"
	"strings"
	"testing"
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
	if !strings.Contains(errOut.String(), "usage: matlistan <migrate|serve|version>") {
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
