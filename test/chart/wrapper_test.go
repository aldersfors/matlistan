package chart

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

// _wrapper is the homelab wrapper chart, in a homelab worktree next to this repo.
const _wrapper = "../../../homelab-matlistan/charts/apps/matlistan"

// The homelab values render against this chart: a key the chart dropped or renamed fails
// here instead of in ArgoCD.
func TestHomelabValuesRender(t *testing.T) {
	raw, err := os.ReadFile(_wrapper + "/values.yaml")
	if err != nil {
		t.Skip("homelab worktree not present")
	}
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	var all map[string]any
	if err := yaml.Unmarshal(raw, &all); err != nil {
		t.Fatal(err)
	}
	sub, err := yaml.Marshal(all["matlistan"])
	if err != nil {
		t.Fatal(err)
	}
	f := t.TempDir() + "/values.yaml"
	if err := os.WriteFile(f, sub, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.CommandContext(t.Context(), "helm", "template", "matlistan", _chart,
		"-n", "matlistan", "-f", f).CombinedOutput()
	if err != nil {
		t.Fatalf("homelab values do not render: %v\n%s", err, out)
	}
	for _, want := range []string{"matlistan.example.org", `value: "sv"`,
		`value: "Europe/Stockholm"`, "matlistan-db-ca", "kind: CronJob"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("rendered homelab chart lacks %q", want)
		}
	}
	if strings.Contains(string(out), "kind: NetworkPolicy") {
		t.Error("chart NetworkPolicy on in the homelab (Cilium policy replaces it)")
	}
}
