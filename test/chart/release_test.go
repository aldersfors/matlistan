package chart

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
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
	for _, want := range []string{"KO_DOCKER_REPO: ghcr.io/aldersfors/matlistan", "--sbom spdx",
		"cosign sign --yes \"$ref\"", "oci://ghcr.io/aldersfors/helm-charts",
		"ghcr.io/aldersfors/helm-charts/matlistan@", "id-token: write", "persist-credentials: false"} {
		if !strings.Contains(wf, want) {
			t.Errorf("release workflow lacks %q", want)
		}
	}
}

// The docs name the published chart and every Secret the chart needs.
func TestDeployDocsNameEverySecret(t *testing.T) {
	raw, err := os.ReadFile("../../docs/deploy.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	for _, want := range []string{"oci://ghcr.io/aldersfors/helm-charts/matlistan",
		"database.urlSecret", "oidc.clientSecret", "session.keySecret",
		"llm.apiKeySecret", "llm.provider", "MATLISTAN_OPENAI_BASE_URL",
		"VERIFY WITH LEGAL COUNSEL", "/auth/callback", "kubectl create job", "database.cnpg",
		"storageClass"} {
		if !strings.Contains(doc, want) {
			t.Errorf("docs/deploy.md lacks %q", want)
		}
	}
	if strings.ContainsRune(doc, '\u2014') {
		t.Error("em-dash in docs/deploy.md")
	}
}

// The project lives at github.com/aldersfors/matlistan; nothing may still publish to or
// import from the old personal namespace.
func TestNoOldOwnerNames(t *testing.T) {
	old := []string{"ghcr.io/" + "jalet", "github.com/" + "jalet/matlistan"}
	err := filepath.WalkDir("../..", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".dev", ".superpowers", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, o := range old {
			if strings.Contains(string(b), o) {
				t.Errorf("%s still names %s", p, o)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Review M6: the documented cosign check pins the exact release workflow identity.
func TestDocsVerifyTheReleaseIdentity(t *testing.T) {
	raw, err := os.ReadFile("../../docs/deploy.md")
	if err != nil {
		t.Fatal(err)
	}
	id := "--certificate-identity https://github.com/aldersfors/matlistan/.github/workflows/" +
		"release-please.yml@refs/heads/main"
	if strings.Count(string(raw), id) != 2 {
		t.Errorf("docs/deploy.md should verify image and chart with %q", id)
	}
}
