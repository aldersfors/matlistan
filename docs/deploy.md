# Deploying Matlistan

Matlistan ships as a container image, `ghcr.io/aldersfors/matlistan`, and a Helm chart,
`oci://ghcr.io/aldersfors/helm-charts/matlistan`. Both are signed with cosign (keyless, from
this repository's release workflow).

## What you need

- Kubernetes 1.30 or later (the Sunday CronJob uses `timeZone`).
- PostgreSQL 18. A CloudNativePG cluster works well: its app secret and CA plug straight
  into the chart.
- An OpenID Connect provider, for example Keycloak.
- An API key for Anthropic or OpenAI, or an OpenAI-compatible server (see "Choosing the
  model provider").
- Optional: a Gateway API implementation for the HTTPRoute, and the Prometheus Operator for
  the ServiceMonitor.

## Secrets

Create these Secrets in the release namespace. The chart mounts them as files readable only
by the app, except the database URL, which the app reads from an environment variable.

| Value | Default key | Contents |
|---|---|---|
| `database.urlSecret` | `uri` | PostgreSQL URL, for example the CNPG `<cluster>-app` secret. Not needed with `database.cnpg.enabled` |
| `database.caSecret` (optional) | `ca.crt` | Database CA, for example CNPG `<cluster>-ca`. When set, the app refuses any connection that is not TLS verified against it |
| `oidc.clientSecret` | `client-secret` | The OIDC client secret |
| `session.keySecret` | `key` | At least 32 random bytes: `openssl rand -hex 32` |
| `llm.apiKeySecret` | `api-key` | The API key for the model provider, used by the web UI and the CronJob. Not needed for an OpenAI-compatible server without a key |

## Database with CloudNativePG (optional)

With the CloudNativePG operator installed, the chart can create the database itself:

```yaml
database:
  cnpg:
    enabled: true
    size: 2Gi
    # Leave empty to use the cluster's default StorageClass, or name another one, for
    # example an encrypted class.
    storageClass: ""
```

The cluster is named `<release>-matlistan-db` (`matlistan-db` when the release is called
`matlistan`). With it on, `database.urlSecret` and `database.caSecret` default to the
cluster's own `<cluster>-app` and `<cluster>-ca` Secrets, so you do not create them, and
the connection is TLS verified. Optional settings:

- `database.cnpg.instances`: replicas (default 1).
- `database.cnpg.imageCatalog.name` and `.major`: a ClusterImageCatalog to take the
  Postgres image from.
- `database.cnpg.backup.barmanObjectName`: a barman-cloud ObjectStore you created, for WAL
  archiving. Add a ScheduledBackup for base backups.
- `database.cnpg.podMonitor`: a PodMonitor for the database.

Choose the StorageClass with care: the database holds allergies, which can be health data.

## Choosing the model provider

`llm.provider` picks who plans the weeks. There is one provider per deployment.

- **Anthropic** (`llm.provider: anthropic`, the default). `llm.model` is optional and
  defaults to the app's choice.
- **OpenAI** (`llm.provider: openai`). `llm.model` is required. The key is an OpenAI
  platform API key; a ChatGPT subscription does not include API access.
- **An OpenAI-compatible server** (`llm.provider: openai` with `llm.baseURL`), such as
  Azure OpenAI, LiteLLM or an Ollama or vLLM server of your own. `llm.model` is the model
  name on that server. `llm.baseURL` becomes `MATLISTAN_OPENAI_BASE_URL`. It must be https,
  except `http://` for a cluster service (`*.svc`) or localhost. `llm.apiKeySecret` can be
  left out when the server takes no key.

```yaml
llm:
  provider: openai
  model: llama4
  baseURL: http://ollama.ai.svc:11434/v1
networkPolicy:
  llm:              # the server listens on 11434, not 443
    cidrs: [10.0.0.0/8]
    port: 11434
```

An answer may use up to 64000 tokens. Models and servers with a smaller output cap or
context reject such requests, so every plan fails. Set `llm.maxOutputTokens` (for example
`16000`, at least 1024) to fit them. It becomes `MATLISTAN_MAX_OUTPUT_TOKENS`.

Leave the deprecated `anthropic:` block out when the provider is `openai`. The chart
refuses to render with both, so an Anthropic key is never sent to another server.

The planner asks for answers that follow a strict JSON schema. Servers that do not enforce
the schema produce more failed plans. The planner retries once with the errors, then shows
the problem on the week.

## OIDC client

Create a confidential client (the chart's default client ID is `matlistan`) with:

- Redirect URI `<baseURL>/auth/callback`, for example
  `https://matlistan.example.org/auth/callback`.
- A claim, `groups` by default (`auth.claim`), that carries one of the values in
  `auth.allowed` for everyone who may sign in.

If the provider uses a certificate from your own CA, put the CA in a ConfigMap and set
`oidc.caConfigMap`.

## Install

```yaml
# values.yaml
baseURL: https://matlistan.example.org
locale: sv            # en (default) or sv
timezone: Europe/Stockholm
database:
  urlSecret:
    name: matlistan-db-app
  caSecret:
    name: matlistan-db-ca
oidc:
  issuer: https://idp.example.org/realms/home
  clientSecret:
    name: matlistan-oidc
auth:
  allowed:
    - Matlistan
session:
  keySecret:
    name: matlistan-session
llm:
  apiKeySecret:
    name: matlistan-llm
httpRoute:
  enabled: true
  parentRefs:
    - name: my-gateway
      namespace: infra
  hostnames:
    - matlistan.example.org
```

```sh
helm install matlistan oci://ghcr.io/aldersfors/helm-charts/matlistan --version X.Y.Z \
  -n matlistan --create-namespace -f values.yaml
```

The app applies its database migrations at startup. It runs as one replica by design:
planning and swapping run in the web process.

To check a release's signatures before installing (both are signed by this repository's
release workflow on `main`):

```sh
cosign verify ghcr.io/aldersfors/matlistan:X.Y.Z \
  --certificate-identity https://github.com/aldersfors/matlistan/.github/workflows/release-please.yml@refs/heads/main \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
cosign verify ghcr.io/aldersfors/helm-charts/matlistan:X.Y.Z \
  --certificate-identity https://github.com/aldersfors/matlistan/.github/workflows/release-please.yml@refs/heads/main \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

## The Sunday draft

A CronJob plans the upcoming week every Sunday at 07:00 in `timezone`
(`generate.schedule`). It does nothing when that week already has a draft or an approved
plan. To run it now:

```sh
kubectl create job -n matlistan --from=cronjob/matlistan-generate generate-now
```

The job name is `<release>-matlistan-generate`, or `<release>-generate` when the release
name already contains `matlistan`.

## Network

The chart's NetworkPolicy (on by default) admits traffic from the Gateway and the metrics
scraper only; set `networkPolicy.gateway` and `networkPolicy.metricsScraper` to their
selectors. Egress is limited to DNS, the database and port 443 (the OIDC provider and the model
provider), plus `networkPolicy.llm` when set. Importing a recipe fetches the page you paste, over
https and from public addresses only, through the same 443 egress; internal and cluster
addresses are refused. Metrics are served on port 9091, which the HTTPRoute does not expose.

## Personal data

When a week is planned, Matlistan sends each family member's age, diets, allergies, likes
and dislikes to the configured model provider. Names are never sent. Allergies can count as
data concerning health under GDPR Article 9 (Regulation (EU) 2016/679, applicable
2018-05-25).

- **Anthropic and OpenAI** process the request in the United States: a transfer governed
  by GDPR Chapter V (Articles 44 to 49).
- **A self-hosted OpenAI-compatible server** in your own network keeps the data at home,
  with no transfer.
- **A third-party compatible service** (Azure OpenAI and others) is a processor in whatever
  region it runs. Check the region and its terms.

A purely private household use may fall under the household exemption in GDPR
Article 2(2)(c), but that has not been verified.
**VERIFY WITH LEGAL COUNSEL** before anyone outside your own household uses the app.

Everything else (names, history, ratings, shopping lists) stays in your database.

> This document provides technical guidance based on published regulatory frameworks. It
> does not constitute legal advice. Consult qualified legal counsel for compliance
> decisions.
