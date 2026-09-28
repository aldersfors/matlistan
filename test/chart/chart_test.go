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
	"time"

	"sigs.k8s.io/yaml"

	"github.com/aldersfors/matlistan/internal/config"
	"github.com/aldersfors/matlistan/internal/declared"
	"github.com/aldersfors/matlistan/internal/household"
	"github.com/aldersfors/matlistan/internal/recipes"
	"github.com/aldersfors/matlistan/internal/theme"
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
	if !strings.HasPrefix(c["image"].(string), "ghcr.io/aldersfors/matlistan:") {
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
		"MATLISTAN_API_KEY_FILE":            "/etc/matlistan/secrets/llm-api-key",
	} {
		if env[k] != want {
			t.Errorf("%s = %q", k, env[k])
		}
	}
	raw, _ := json.Marshal(spec)
	for _, secret := range []string{"matlistan-session", "matlistan-oidc", "matlistan-llm"} {
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
		"oidc.clientSecret.name=", "session.keySecret.name=", "llm.apiKeySecret.name=",
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

func TestCNPGOffByDefault(t *testing.T) {
	if got := find(render(t), "Cluster"); len(got) != 0 {
		t.Error("CNPG Cluster rendered by default")
	}
}

// With database.cnpg on and no secrets named, the chart uses the cluster's own app and CA
// secrets, and the cluster's default StorageClass.
func TestCNPGDefaults(t *testing.T) {
	objs := render(t, "--set", "database.cnpg.enabled=true", "--set", "database.urlSecret.name=")
	c := one(t, objs, "Cluster")
	if c["apiVersion"] != "postgresql.cnpg.io/v1" || path(c, "metadata", "name") != "ml-matlistan-db" {
		t.Fatalf("cluster = %v %v", c["apiVersion"], path(c, "metadata", "name"))
	}
	storage, _ := path(c, "spec", "storage").(obj)
	if _, set := storage["storageClass"]; set {
		t.Errorf("storageClass set by default: %v", storage)
	}
	if storage["size"] != "2Gi" || path(c, "spec", "instances") != 1.0 {
		t.Errorf("spec = %v", c["spec"])
	}
	if path(c, "spec", "bootstrap", "initdb", "database") != "matlistan" ||
		path(c, "spec", "bootstrap", "initdb", "owner") != "matlistan" {
		t.Errorf("bootstrap = %v", path(c, "spec", "bootstrap"))
	}
	for _, key := range []string{"imageCatalogRef", "plugins"} {
		if path(c, "spec", key) != nil {
			t.Errorf("%s set by default", key)
		}
	}
	spec := podSpec(t, objs)
	raw, _ := json.Marshal(spec)
	for _, want := range []string{`"name":"ml-matlistan-db-app"`, `"secretName":"ml-matlistan-db-ca"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("web pod lacks %s", want)
		}
	}
	if env := envOf(container(t, spec)); env["MATLISTAN_DATABASE_CA_FILE"] == "" {
		t.Error("CNPG database not TLS verified")
	}
	if env := envOf(container(t, cronPod(t, objs))); env["MATLISTAN_DATABASE_CA_FILE"] == "" {
		t.Error("job does not verify the CNPG database")
	}
}

func TestCNPGOverrides(t *testing.T) {
	c := one(t, render(t, "--set", "database.cnpg.enabled=true",
		"--set", "database.cnpg.storageClass=longhorn-crypt", "--set", "database.cnpg.size=5Gi",
		"--set", "database.cnpg.imageCatalog.name=postgresql-minimal-trixie",
		"--set", "database.cnpg.backup.barmanObjectName=s3-eu-north-1"), "Cluster")
	if path(c, "spec", "storage", "storageClass") != "longhorn-crypt" ||
		path(c, "spec", "storage", "size") != "5Gi" {
		t.Errorf("storage = %v", path(c, "spec", "storage"))
	}
	if path(c, "spec", "imageCatalogRef", "name") != "postgresql-minimal-trixie" ||
		path(c, "spec", "imageCatalogRef", "major") != 18.0 {
		t.Errorf("imageCatalogRef = %v", path(c, "spec", "imageCatalogRef"))
	}
	if path(c, "spec", "plugins", "0", "name") != "barman-cloud.cloudnative-pg.io" ||
		path(c, "spec", "plugins", "0", "isWALArchiver") != true ||
		path(c, "spec", "plugins", "0", "parameters", "barmanObjectName") != "s3-eu-north-1" {
		t.Errorf("plugins = %v", path(c, "spec", "plugins"))
	}
}

// The app's policies select app.kubernetes.io/name=matlistan. A CNPG operator that copies
// cluster labels to its pods must not make the database pods match them.
func TestCNPGClusterIsNotSelectedAsTheApp(t *testing.T) {
	c := one(t, render(t, "--set", "database.cnpg.enabled=true"), "Cluster")
	if got := path(c, "metadata", "labels", "app.kubernetes.io/name"); got != "matlistan-db" {
		t.Errorf("cluster name label = %v, want matlistan-db", got)
	}
	if got := path(c, "metadata", "labels", "app.kubernetes.io/part-of"); got != "matlistan" {
		t.Errorf("cluster part-of label = %v", got)
	}
}

func TestOpenAIEnv(t *testing.T) {
	objs := render(t, "--set", "llm.provider=openai", "--set", "llm.model=gpt-6",
		"--set", "llm.baseURL=https://llm.example.org/v1")
	for _, spec := range []any{podSpec(t, objs), cronPod(t, objs)} {
		env := envOf(container(t, spec))
		for k, want := range map[string]string{"MATLISTAN_PROVIDER": "openai",
			"MATLISTAN_MODEL": "gpt-6", "MATLISTAN_OPENAI_BASE_URL": "https://llm.example.org/v1",
			"MATLISTAN_API_KEY_FILE": "/etc/matlistan/secrets/llm-api-key"} {
			if env[k] != want {
				t.Errorf("%s = %q, want %q", k, env[k], want)
			}
		}
	}
	env := envOf(container(t, podSpec(t, objs)))
	if _, err := config.Parse(func(k string) string { return env[k] }); err != nil {
		t.Fatalf("serve config: %v", err)
	}
	jobEnv := envOf(container(t, cronPod(t, objs)))
	if _, err := config.ParseGenerate(func(k string) string { return jobEnv[k] }); err != nil {
		t.Fatalf("generate config: %v", err)
	}
}

func TestOpenAINeedsAModel(t *testing.T) {
	if _, err := helmTemplate(t, "--set", "llm.provider=openai"); err == nil {
		t.Error("openai rendered without llm.model")
	}
	if _, err := helmTemplate(t, "--set", "llm.provider=gemini"); err == nil {
		t.Error("unknown provider accepted")
	}
}

// Review focus 1: the running homelab values still render the same key and model.
func TestDeprecatedAnthropicValuesStillRender(t *testing.T) {
	objs := render(t, "--set", "llm.apiKeySecret.name=", "--set", "anthropic.apiKeySecret.name=old",
		"--set", "anthropic.model=claude-sonnet-5")
	env := envOf(container(t, podSpec(t, objs)))
	if env["MATLISTAN_API_KEY_FILE"] != "/etc/matlistan/secrets/llm-api-key" ||
		env["MATLISTAN_MODEL"] != "claude-sonnet-5" || env["MATLISTAN_ANTHROPIC_API_KEY_FILE"] != "" {
		t.Errorf("env = %v", env)
	}
	raw, _ := json.Marshal(podSpec(t, objs))
	if !strings.Contains(string(raw), `"name":"old"`) {
		t.Error("deprecated key secret not mounted")
	}
}

// Review focus 5: a keyless local server leaves the job without a secrets volume.
func TestCronJobWithoutKeyHasNoSecretVolume(t *testing.T) {
	objs := render(t, "--set", "llm.apiKeySecret.name=", "--set", "llm.provider=openai",
		"--set", "llm.model=llama4", "--set", "llm.baseURL=http://ollama.ml.svc:11434/v1")
	spec := cronPod(t, objs)
	raw, _ := json.Marshal(spec)
	if strings.Contains(string(raw), `"name":"secrets"`) {
		t.Errorf("job has a secrets volume without any secret: %s", raw)
	}
	env := envOf(container(t, spec))
	if env["MATLISTAN_API_KEY_FILE"] != "" {
		t.Errorf("key file env without a key: %v", env)
	}
	if _, err := config.ParseGenerate(func(k string) string { return env[k] }); err != nil {
		t.Fatalf("generate config: %v", err)
	}
}

func TestKeyRequiredForHostedProviders(t *testing.T) {
	if _, err := helmTemplate(t, "--set", "llm.apiKeySecret.name="); err == nil {
		t.Error("anthropic rendered without a key secret")
	}
	if _, err := helmTemplate(t, "--set", "llm.apiKeySecret.name=", "--set", "llm.provider=openai",
		"--set", "llm.model=gpt-6"); err == nil {
		t.Error("api.openai.com rendered without a key secret")
	}
}

func TestNetworkPolicyModelServerPort(t *testing.T) {
	np := one(t, render(t, "--set", "networkPolicy.llm.port=11434",
		"--set", "networkPolicy.llm.cidrs[0]=10.0.0.0/8"), "NetworkPolicy")
	raw, _ := json.Marshal(np["spec"])
	if !strings.Contains(string(raw), `"port":11434`) || !strings.Contains(string(raw), `"cidr":"10.0.0.0/8"`) {
		t.Errorf("policy lacks the model server rule: %s", raw)
	}
	np = one(t, render(t), "NetworkPolicy")
	raw, _ = json.Marshal(np["spec"])
	if strings.Contains(string(raw), `"port":11434`) {
		t.Error("model server rule rendered without a port")
	}
}

func TestMaxOutputTokensEnv(t *testing.T) {
	objs := render(t, "--set", "llm.provider=openai", "--set", "llm.model=gpt-4.1-mini",
		"--set", "llm.maxOutputTokens=16000")
	for _, spec := range []any{podSpec(t, objs), cronPod(t, objs)} {
		if env := envOf(container(t, spec)); env["MATLISTAN_MAX_OUTPUT_TOKENS"] != "16000" {
			t.Errorf("MATLISTAN_MAX_OUTPUT_TOKENS = %q", env["MATLISTAN_MAX_OUTPUT_TOKENS"])
		}
	}
	if env := envOf(container(t, podSpec(t, render(t)))); env["MATLISTAN_MAX_OUTPUT_TOKENS"] != "" {
		t.Error("token cap set by default")
	}
}

// Review: an Anthropic key left in the deprecated block must never go to another provider.
func TestAnthropicKeyNeverGoesToOpenAI(t *testing.T) {
	if _, err := helmTemplate(t, "--set", "llm.apiKeySecret.name=", "--set", "llm.provider=openai",
		"--set", "llm.model=llama4", "--set", "llm.baseURL=http://ollama.ai.svc:11434/v1",
		"--set", "anthropic.apiKeySecret.name=matlistan-anthropic"); err == nil {
		t.Error("openai rendered with the deprecated anthropic key set")
	}
}

// Review: a model-server port without destinations would open that port to everywhere.
func TestModelServerPortNeedsDestinations(t *testing.T) {
	if _, err := helmTemplate(t, "--set", "networkPolicy.llm.port=11434"); err == nil {
		t.Error("networkPolicy.llm.port rendered without cidrs")
	}
}
func TestPushOffByDefault(t *testing.T) {
	objs := render(t)
	if len(find(objs, "Certificate")) != 0 || len(find(objs, "Issuer")) != 0 {
		t.Error("push objects without push.enabled")
	}
	for _, env := range []map[string]string{envOf(container(t, podSpec(t, objs))),
		envOf(container(t, cronPod(t, objs)))} {
		if env["MATLISTAN_VAPID_KEY_FILE"] != "" || env["MATLISTAN_VAPID_SUBJECT"] != "" {
			t.Errorf("push env with push off: %v", env)
		}
	}
}

// Review focus 5: a renewal keeps the key, since every subscription is bound to it.
func TestPushCertificateKeepsTheKey(t *testing.T) {
	objs := render(t, "--set", "push.enabled=true")
	certs, issuers := find(objs, "Certificate"), find(objs, "Issuer")
	if len(certs) != 1 || len(issuers) != 1 {
		t.Fatalf("certificates %d issuers %d", len(certs), len(issuers))
	}
	spec := certs[0]["spec"].(obj)
	pk := spec["privateKey"].(obj)
	if pk["algorithm"] != "ECDSA" || pk["size"] != float64(256) || pk["encoding"] != "PKCS8" ||
		pk["rotationPolicy"] != "Never" {
		t.Errorf("privateKey = %v", pk)
	}
	if spec["secretName"] != "ml-matlistan-vapid" || spec["duration"] != "87600h0m0s" && spec["duration"] != "87600h" {
		t.Errorf("spec = %v", spec)
	}
	if ref := spec["issuerRef"].(obj); ref["kind"] != "Issuer" || ref["name"] != issuers[0]["metadata"].(obj)["name"] {
		t.Errorf("issuerRef = %v", ref)
	}
	if _, ok := issuers[0]["spec"].(obj)["selfSigned"]; !ok {
		t.Errorf("issuer = %v", issuers[0]["spec"])
	}
	for name, pod := range map[string]any{"web": podSpec(t, objs), "job": cronPod(t, objs)} {
		env := envOf(container(t, pod))
		if env["MATLISTAN_VAPID_KEY_FILE"] != "/etc/matlistan/secrets/vapid-key" {
			t.Errorf("%s env = %v", name, env)
		}
		raw, _ := json.Marshal(pod)
		if !strings.Contains(string(raw), `"name":"ml-matlistan-vapid"`) ||
			!strings.Contains(string(raw), `"key":"tls.key","path":"vapid-key"`) {
			t.Errorf("%s: key not mounted: %s", name, raw)
		}
	}
	jobEnv := envOf(container(t, cronPod(t, objs)))
	if _, err := config.ParseGenerate(func(k string) string { return jobEnv[k] }); err != nil {
		t.Fatalf("job config: %v", err)
	}
	if jobEnv["MATLISTAN_BASE_URL"] == "" {
		t.Error("job lacks MATLISTAN_BASE_URL")
	}
}

func TestPushWithOwnSecret(t *testing.T) {
	objs := render(t, "--set", "push.enabled=true", "--set", "push.vapidKeySecret.name=matlistan-vapid",
		"--set", "push.vapidKeySecret.key=private-key", "--set", "push.subject=mailto:admin@example.org")
	if len(find(objs, "Certificate")) != 0 || len(find(objs, "Issuer")) != 0 {
		t.Error("certificate rendered although a Secret is named")
	}
	for name, pod := range map[string]any{"web": podSpec(t, objs), "job": cronPod(t, objs)} {
		raw, _ := json.Marshal(pod)
		if !strings.Contains(string(raw), `"name":"matlistan-vapid"`) ||
			!strings.Contains(string(raw), `"key":"private-key","path":"vapid-key"`) {
			t.Errorf("%s: own secret not mounted", name)
		}
		if env := envOf(container(t, pod)); env["MATLISTAN_VAPID_SUBJECT"] != "mailto:admin@example.org" {
			t.Errorf("%s subject = %q", name, env["MATLISTAN_VAPID_SUBJECT"])
		}
	}
}

// ArgoCD does not pass CRD APIs to Helm, so the chart must render the Certificate without
// seeing cert-manager.io/v1; a cluster without cert-manager fails at apply instead.
func TestPushRendersWithoutTheCertManagerAPI(t *testing.T) {
	objs := render(t, "--set", "push.enabled=true")
	if len(find(objs, "Certificate")) != 1 || len(find(objs, "Issuer")) != 1 {
		t.Fatal("no Certificate or Issuer without the cert-manager API")
	}
}

// A keyless model server leaves the job without a secrets volume, but a push key needs it.
func TestPushKeyMountsTheJobSecretsAlone(t *testing.T) {
	objs := render(t, "--set", "llm.apiKeySecret.name=", "--set", "llm.provider=openai",
		"--set", "llm.model=llama4", "--set", "llm.baseURL=http://ollama.ml.svc:11434/v1",
		"--set", "push.enabled=true")
	raw, _ := json.Marshal(cronPod(t, objs))
	if !strings.Contains(string(raw), `"path":"vapid-key"`) ||
		!strings.Contains(string(raw), `"mountPath":"/etc/matlistan/secrets"`) {
		t.Fatalf("job: %s", raw)
	}
}
func TestHouseholdOffByDefault(t *testing.T) {
	objs := render(t)
	for _, o := range find(objs, "Secret") {
		if strings.HasSuffix(path(o, "metadata", "name").(string), "-household") {
			t.Fatal("household secret without household.enabled")
		}
	}
	if env := envOf(container(t, podSpec(t, objs))); env["MATLISTAN_HOUSEHOLD_FILE"] != "" {
		t.Fatalf("env = %v", env)
	}
}

func TestHouseholdRendersASecret(t *testing.T) {
	objs := render(t, "-f", "testdata/household.yaml")
	var secret obj
	for _, o := range find(objs, "Secret") {
		if path(o, "metadata", "name") == "ml-matlistan-household" {
			secret = o
		}
	}
	if secret == nil {
		t.Fatal("no household secret")
	}
	file := path(secret, "stringData", "household.yaml").(string)
	h, err := declared.Parse([]byte(file), "sv", time.Now(), func(s string) string { return s })
	if err != nil || len(h.Members) != 1 || len(h.Recipes) != 1 || len(h.RecipeURLs) != 1 {
		t.Fatalf("file does not parse: %v\n%s", err, file)
	}
	spec := podSpec(t, objs)
	if env := envOf(container(t, spec)); env["MATLISTAN_HOUSEHOLD_FILE"] != "/etc/matlistan/household/household.yaml" {
		t.Fatalf("env = %v", env)
	}
	raw, _ := json.Marshal(spec)
	if !strings.Contains(string(raw), `"secretName":"ml-matlistan-household"`) ||
		!strings.Contains(string(raw), `"mountPath":"/etc/matlistan/household"`) {
		t.Fatalf("not mounted: %s", raw)
	}
	cronRaw, _ := json.Marshal(cronPod(t, objs))
	if strings.Contains(string(cronRaw), "household") {
		t.Fatal("the CronJob mounts the household")
	}
}

func TestHouseholdChecksumFollowsTheContent(t *testing.T) {
	a := render(t, "-f", "testdata/household.yaml")
	b := render(t, "-f", "testdata/household.yaml", "--set", "household.members[0].name=Anna J")
	sum := func(objs []obj) any {
		return path(one(t, objs, "Deployment"), "spec", "template", "metadata", "annotations", "checksum/household")
	}
	if sum(a) == nil || sum(a) == sum(b) {
		t.Fatalf("checksum %v vs %v", sum(a), sum(b))
	}
}

func TestHouseholdSchemaRejects(t *testing.T) {
	for _, set := range []string{"household.members[0].key=Bad Key",
		"household.recipes[0].ingredients[0].unit=bucket", "household.members[0].allergens[0]=dust"} {
		if out, err := helmTemplate(t, "-f", "testdata/household.yaml", "--set", set); err == nil {
			t.Errorf("%s accepted:\n%.300s", set, out)
		}
	}
}

// The schema's enums must be the app's own lists.
func TestHouseholdSchemaEnumsMatchTheApp(t *testing.T) {
	raw, _ := os.ReadFile(_chart + "/values.schema.json")
	s := string(raw)
	for _, list := range [][]string{recipes.Units, recipes.Sections, household.Diets, household.Allergens} {
		for _, v := range list {
			if !strings.Contains(s, `"`+v+`"`) {
				t.Errorf("schema lacks %q", v)
			}
		}
	}
}

// The example in docs/household-as-code.md is what people paste first: it must render.
func TestHouseholdDocsExampleRenders(t *testing.T) {
	doc, err := os.ReadFile("../../docs/household-as-code.md")
	if err != nil {
		t.Fatal(err)
	}
	s := string(doc)
	start := strings.Index(s, "```yaml\nhousehold:")
	if start < 0 {
		t.Fatal("no household example in the docs")
	}
	s = s[start+len("```yaml\n"):]
	end := strings.Index(s, "```")
	if end < 0 {
		t.Fatal("household example is not closed")
	}
	example := s[:end]
	f := t.TempDir() + "/example.yaml"
	if err := os.WriteFile(f, []byte(example), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := helmTemplate(t, "-f", f); err != nil {
		t.Fatalf("docs example does not render: %v\n%s", err, out)
	}
}
