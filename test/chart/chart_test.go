// Package chart tests the Helm chart by rendering it with helm template.
package chart

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/jalet/matlistan/internal/config"
	"github.com/jalet/matlistan/internal/theme"
)

const _chart = "../../charts/matlistan"

type obj = map[string]any

// readChart reads every chart file in the test process. Go caches test results by the
// files a test opens, and helm's own reads are invisible to it, so without this a changed
// template could pass from the cache (CI restores GOCACHE).
func readChart(t *testing.T) {
	t.Helper()
	err := filepath.WalkDir(_chart, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		_, err = os.ReadFile(p)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func helmTemplate(t *testing.T, extra ...string) (string, error) {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	readChart(t)
	args := append([]string{"template", "ml", _chart, "-n", "matlistan", "-f",
		"testdata/minimal.yaml"}, extra...)
	var out bytes.Buffer
	cmd := exec.CommandContext(t.Context(), "helm", args...)
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

func render(t *testing.T, extra ...string) []obj {
	t.Helper()
	out, err := helmTemplate(t, extra...)
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	var objs []obj
	for _, doc := range strings.Split(out, "\n---") {
		j, err := yaml.YAMLToJSON([]byte(doc))
		if err != nil {
			t.Fatalf("parse: %v\n%s", err, doc)
		}
		var o obj
		if err := json.Unmarshal(j, &o); err != nil || o == nil {
			continue
		}
		objs = append(objs, o)
	}
	return objs
}

func find(objs []obj, kind string) []obj {
	var out []obj
	for _, o := range objs {
		if o["kind"] == kind {
			out = append(out, o)
		}
	}
	return out
}

func one(t *testing.T, objs []obj, kind string) obj {
	t.Helper()
	got := find(objs, kind)
	if len(got) != 1 {
		t.Fatalf("%d %s objects, want 1", len(got), kind)
	}
	return got[0]
}

// path walks maps by key and lists by index ("0").
func path(v any, keys ...string) any {
	for _, k := range keys {
		switch c := v.(type) {
		case map[string]any:
			v = c[k]
		case []any:
			i := 0
			for _, r := range k {
				i = i*10 + int(r-'0')
			}
			if i >= len(c) {
				return nil
			}
			v = c[i]
		default:
			return nil
		}
	}
	return v
}

func container(t *testing.T, pod any) obj {
	t.Helper()
	c, ok := path(pod, "containers", "0").(obj)
	if !ok {
		t.Fatal("no container")
	}
	return c
}

func podSpec(t *testing.T, objs []obj) any {
	t.Helper()
	return path(one(t, objs, "Deployment"), "spec", "template", "spec")
}

// envOf returns the container env as name -> value; secretKeyRef values become "set".
func envOf(c obj) map[string]string {
	env := map[string]string{}
	list, _ := c["env"].([]any)
	for _, e := range list {
		m := e.(obj)
		v, _ := m["value"].(string)
		if m["valueFrom"] != nil {
			v = "postgres://set"
		}
		env[m["name"].(string)] = v
	}
	return env
}

func TestPodHardening(t *testing.T) {
	spec := podSpec(t, render(t))
	if path(spec, "automountServiceAccountToken") != false {
		t.Error("service account token mounted")
	}
	for k, want := range map[string]any{"runAsNonRoot": true, "runAsUser": 65532.0,
		"fsGroup": 65532.0} {
		if got := path(spec, "securityContext", k); got != want {
			t.Errorf("pod securityContext.%s = %v", k, got)
		}
	}
	if path(spec, "securityContext", "seccompProfile", "type") != "RuntimeDefault" {
		t.Error("seccomp")
	}
	c := container(t, spec)
	if path(c, "securityContext", "readOnlyRootFilesystem") != true ||
		path(c, "securityContext", "allowPrivilegeEscalation") != false ||
		path(c, "securityContext", "capabilities", "drop", "0") != "ALL" {
		t.Errorf("container securityContext = %v", c["securityContext"])
	}
	if !strings.HasPrefix(c["image"].(string), "ghcr.io/jalet/matlistan:") {
		t.Errorf("image %v", c["image"])
	}
}

func TestOneReplicaRollingUpdate(t *testing.T) {
	d := one(t, render(t), "Deployment")
	if path(d, "spec", "replicas") != 1.0 ||
		path(d, "spec", "strategy", "rollingUpdate", "maxUnavailable") != 0.0 {
		t.Errorf("spec = %v", d["spec"])
	}
}

// Review focus 4: a database failover must not restart the pod.
func TestProbes(t *testing.T) {
	c := container(t, podSpec(t, render(t)))
	if path(c, "livenessProbe", "tcpSocket", "port") != "http" {
		t.Errorf("liveness = %v", c["livenessProbe"])
	}
	for _, p := range []string{"readinessProbe", "startupProbe"} {
		if path(c, p, "httpGet", "path") != "/healthz" {
			t.Errorf("%s = %v", p, c[p])
		}
	}
}

// Review focus 1: whatever the chart sets is exactly what the app needs to start.
func TestChartEnvSatisfiesConfig(t *testing.T) {
	env := envOf(container(t, podSpec(t, render(t))))
	if _, err := config.Parse(func(k string) string { return env[k] }); err != nil {
		t.Fatalf("serve config from chart env: %v", err)
	}
	for k := range env {
		if !strings.HasPrefix(k, "MATLISTAN_") {
			t.Errorf("unexpected env %s", k)
		}
	}
	for k, want := range map[string]string{"MATLISTAN_BASE_URL": "https://matlistan.example",
		"MATLISTAN_OIDC_ALLOWED": "Matlistan", "MATLISTAN_LOCALE": "en",
		"MATLISTAN_TIMEZONE": "UTC", "MATLISTAN_METRICS_ADDR": ":9091"} {
		if env[k] != want {
			t.Errorf("%s = %q, want %q", k, env[k], want)
		}
	}
}

func TestLocaleAndTimezone(t *testing.T) {
	env := envOf(container(t, podSpec(t, render(t, "--set", "locale=sv",
		"--set", "timezone=Europe/Stockholm"))))
	if env["MATLISTAN_LOCALE"] != "sv" || env["MATLISTAN_TIMEZONE"] != "Europe/Stockholm" {
		t.Errorf("env = %v", env)
	}
}

func TestSecretsAsFiles(t *testing.T) {
	spec := podSpec(t, render(t))
	env := envOf(container(t, spec))
	for k, want := range map[string]string{
		"MATLISTAN_SESSION_KEY_FILE":        "/etc/matlistan/secrets/session-key",
		"MATLISTAN_OIDC_CLIENT_SECRET_FILE": "/etc/matlistan/secrets/client-secret",
		"MATLISTAN_ANTHROPIC_API_KEY_FILE":  "/etc/matlistan/secrets/anthropic-api-key",
	} {
		if env[k] != want {
			t.Errorf("%s = %q", k, env[k])
		}
	}
	raw, _ := json.Marshal(spec)
	for _, secret := range []string{"matlistan-session", "matlistan-oidc", "matlistan-anthropic"} {
		if !strings.Contains(string(raw), `"name":"`+secret+`"`) {
			t.Errorf("secret %s not mounted", secret)
		}
	}
}

// Review focus 3: uid 65532 must be able to read the mounted secrets, nobody else.
func TestSecretFilesReadableByTheApp(t *testing.T) {
	vols, _ := path(podSpec(t, render(t)), "volumes").([]any)
	for _, v := range vols {
		if path(v, "name") == "secrets" {
			if mode := path(v, "projected", "defaultMode"); mode != 288.0 { // 0440
				t.Errorf("secrets defaultMode = %v, want 0440", mode)
			}
			return
		}
	}
	t.Fatal("no secrets volume")
}

func TestDatabaseCAIsVerified(t *testing.T) {
	spec := podSpec(t, render(t, "--set", "database.caSecret.name=matlistan-db-ca"))
	if env := envOf(container(t, spec)); env["MATLISTAN_DATABASE_CA_FILE"] !=
		"/etc/matlistan/db-ca/ca.crt" {
		t.Errorf("CA file env = %q", env["MATLISTAN_DATABASE_CA_FILE"])
	}
	if env := envOf(container(t, podSpec(t, render(t)))); env["MATLISTAN_DATABASE_CA_FILE"] != "" {
		t.Error("CA env set without a CA secret")
	}
}

func TestRequiredValues(t *testing.T) {
	for _, v := range []string{"baseURL=", "database.urlSecret.name=", "oidc.issuer=",
		"oidc.clientSecret.name=", "session.keySecret.name=", "anthropic.apiKeySecret.name=",
		"auth.allowed=null"} {
		if out, err := helmTemplate(t, "--set", v); err == nil {
			t.Errorf("rendered without %s:\n%.200s", v, out)
		}
	}
}

func TestServicePorts(t *testing.T) {
	ports, _ := path(one(t, render(t), "Service"), "spec", "ports").([]any)
	got := map[string]any{}
	for _, p := range ports {
		got[path(p, "name").(string)] = path(p, "port")
	}
	if got["http"] != 8080.0 || got["metrics"] != 9091.0 {
		t.Errorf("ports = %v", got)
	}
}

func TestLint(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	out, err := exec.CommandContext(t.Context(), "helm", "lint", "--strict", _chart, "-f",
		"testdata/minimal.yaml").CombinedOutput()
	if err != nil {
		t.Fatalf("helm lint: %v\n%s", err, out)
	}
}

func cronPod(t *testing.T, objs []obj) any {
	t.Helper()
	return path(one(t, objs, "CronJob"), "spec", "jobTemplate", "spec", "template", "spec")
}

func TestCronJobSchedule(t *testing.T) {
	cj := one(t, render(t, "--set", "timezone=Europe/Stockholm"), "CronJob")
	for k, want := range map[string]any{"schedule": "0 7 * * 0",
		"timeZone": "Europe/Stockholm", "concurrencyPolicy": "Forbid"} {
		if got := path(cj, "spec", k); got != want {
			t.Errorf("spec.%s = %v, want %v", k, got, want)
		}
	}
	job := path(cj, "spec", "jobTemplate", "spec")
	if path(job, "activeDeadlineSeconds") != 900.0 || path(job, "backoffLimit") != 1.0 {
		t.Errorf("job spec = %v", job)
	}
	c := container(t, cronPod(t, render(t)))
	if path(c, "args", "0") != "generate" || path(c, "args", "1") != nil {
		t.Errorf("args = %v", c["args"])
	}
}

// Review focus 2: the job has what generate needs, and no web secrets.
func TestCronJobEnvSatisfiesGenerate(t *testing.T) {
	spec := cronPod(t, render(t, "--set", "database.caSecret.name=matlistan-db-ca"))
	env := envOf(container(t, spec))
	if _, err := config.ParseGenerate(func(k string) string { return env[k] }); err != nil {
		t.Fatalf("generate config from chart env: %v", err)
	}
	if env["MATLISTAN_DATABASE_CA_FILE"] == "" {
		t.Error("job does not verify the database")
	}
	raw, _ := json.Marshal(spec)
	for _, secret := range []string{"matlistan-session", "matlistan-oidc"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("job mounts %s", secret)
		}
	}
	if path(spec, "restartPolicy") != "Never" || path(spec, "automountServiceAccountToken") != false {
		t.Errorf("job pod = %v", spec)
	}
}

func TestCronJobOptional(t *testing.T) {
	if got := find(render(t, "--set", "generate.enabled=false"), "CronJob"); len(got) != 0 {
		t.Error("CronJob rendered when disabled")
	}
}

func TestSelectorsKeepWebAndJobApart(t *testing.T) {
	objs := render(t)
	sel := path(one(t, objs, "Deployment"), "spec", "selector", "matchLabels", "app.kubernetes.io/component")
	svc := path(one(t, objs, "Service"), "spec", "selector", "app.kubernetes.io/component")
	if sel != "web" || svc != "web" {
		t.Errorf("deployment selector %v, service selector %v", sel, svc)
	}
}

func TestHTTPRoute(t *testing.T) {
	if got := find(render(t), "HTTPRoute"); len(got) != 0 {
		t.Fatal("route rendered by default")
	}
	r := one(t, render(t, "--set", "httpRoute.enabled=true",
		"--set", "httpRoute.hostnames[0]=matlistan.example",
		"--set", "httpRoute.parentRefs[0].name=ingress"), "HTTPRoute")
	if path(r, "spec", "hostnames", "0") != "matlistan.example" ||
		path(r, "spec", "rules", "0", "backendRefs", "0", "port") != 8080.0 {
		t.Errorf("route = %v", r["spec"])
	}
}

func TestNetworkPolicy(t *testing.T) {
	np := one(t, render(t), "NetworkPolicy")
	raw, _ := json.Marshal(np["spec"])
	for _, want := range []string{`"port":8080`, `"port":9091`, `"port":53`, `"port":5432`,
		`"port":443`, `"Ingress","Egress"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("policy lacks %s: %s", want, raw)
		}
	}
	if got := find(render(t, "--set", "networkPolicy.enabled=false"), "NetworkPolicy"); len(got) != 0 {
		t.Error("policy rendered when disabled")
	}
}

func TestServiceMonitorOptional(t *testing.T) {
	if len(find(render(t), "ServiceMonitor")) != 0 {
		t.Error("rendered by default")
	}
	sm := one(t, render(t, "--set", "serviceMonitor.enabled=true"), "ServiceMonitor")
	if path(sm, "spec", "endpoints", "0", "port") != "metrics" {
		t.Errorf("monitor = %v", sm["spec"])
	}
}

func TestThemeRendered(t *testing.T) {
	objs := render(t, "--set-string", "theme.light.day1=#112233")
	cm := one(t, objs, "ConfigMap")
	body, _ := path(cm, "data", "theme.yaml").(string)
	if _, err := theme.Parse([]byte(body)); err != nil {
		t.Fatalf("app rejects the rendered theme: %v\n%s", err, body)
	}
	env := envOf(container(t, podSpec(t, objs)))
	if env["MATLISTAN_THEME_FILE"] != "/etc/matlistan/theme/theme.yaml" {
		t.Error("theme file env missing")
	}
	if len(find(render(t), "ConfigMap")) != 0 {
		t.Error("theme ConfigMap without a theme")
	}
}

func TestThemeSchemaRejects(t *testing.T) {
	if _, err := helmTemplate(t, "--set-string", "theme.light.day1=blue"); err == nil {
		t.Error("schema accepted a non-hex colour")
	}
}

// Security review M6: the Sunday job runs as hardened as the web pod.
func TestCronJobPodHardening(t *testing.T) {
	spec := cronPod(t, render(t))
	for k, want := range map[string]any{"runAsNonRoot": true, "runAsUser": 65532.0,
		"fsGroup": 65532.0} {
		if got := path(spec, "securityContext", k); got != want {
			t.Errorf("job securityContext.%s = %v", k, got)
		}
	}
	c := container(t, spec)
	if path(c, "securityContext", "readOnlyRootFilesystem") != true ||
		path(c, "securityContext", "allowPrivilegeEscalation") != false ||
		path(c, "securityContext", "capabilities", "drop", "0") != "ALL" {
		t.Errorf("job container securityContext = %v", c["securityContext"])
	}
}

// Review M6: a node down at 07:00 must not cost the week's draft. generate plans next week
// for any time on Sunday, so the job may start as late as Sunday evening.
func TestCronJobStartsLateOnSunday(t *testing.T) {
	cj := one(t, render(t), "CronJob")
	if got := path(cj, "spec", "startingDeadlineSeconds"); got != 57600.0 {
		t.Errorf("startingDeadlineSeconds = %v, want 57600 (16h)", got)
	}
}

// Review M6: the app rejects unknown theme keys at startup, so the schema must too.
func TestThemeSchemaRejectsUnknownKeys(t *testing.T) {
	if _, err := helmTemplate(t, "--set-string", "theme.light.primray=#ffffff"); err == nil {
		t.Error("schema accepted a misspelt theme key")
	}
	for _, k := range theme.Tokens {
		if out, err := helmTemplate(t, "--set-string", "theme.dark."+k+"=#123456"); err != nil {
			t.Errorf("schema rejected theme token %s: %v\n%.200s", k, err, out)
		}
	}
}
