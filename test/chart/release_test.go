package chart

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// release-please must bump the chart with the app, or a release ships a chart that pins
// the previous image.
func TestReleaseBumpsChartVersions(t *testing.T) {
	raw, err := os.ReadFile("../../release-please-config.json")
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Packages map[string]struct {
			ExtraFiles []struct {
				Path     string `json:"path"`
				JSONPath string `json:"jsonpath"`
			} `json:"extra-files"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, f := range cfg.Packages["."].ExtraFiles {
		if f.Path == "charts/matlistan/Chart.yaml" {
			seen[f.JSONPath] = true
		}
	}
	if !seen["$.version"] || !seen["$.appVersion"] {
		t.Errorf("release-please does not bump the chart: %v", seen)
	}
}

func TestReleaseSignsImageAndChart(t *testing.T) {
	raw, err := os.ReadFile("../../.github/workflows/release-please.yml")
	if err != nil {
		t.Fatal(err)
	}
	wf := string(raw)
	for _, want := range []string{"KO_DOCKER_REPO: ghcr.io/jalet/matlistan", "--sbom spdx",
		"cosign sign --yes \"$ref\"", "oci://ghcr.io/jalet/helm-charts",
		"ghcr.io/jalet/helm-charts/matlistan@", "id-token: write", "persist-credentials: false"} {
		if !strings.Contains(wf, want) {
			t.Errorf("release workflow lacks %q", want)
		}
	}
}
