# Deploying Matlistan

Matlistan ships as a container image, `ghcr.io/jalet/matlistan`, and a Helm chart,
`oci://ghcr.io/jalet/helm-charts/matlistan`. Both are signed with cosign (keyless, from
this repository's release workflow).

## What you need

- Kubernetes 1.30 or later (the Sunday CronJob uses `timeZone`).
- PostgreSQL 18. A CloudNativePG cluster works well: its app secret and CA plug straight
  into the chart.
- An OpenID Connect provider, for example Keycloak.
- An Anthropic API key.
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
| `anthropic.apiKeySecret` | `api-key` | The Anthropic API key, used by the web UI and the CronJob |

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
anthropic:
  apiKeySecret:
    name: matlistan-anthropic
httpRoute:
  enabled: true
  parentRefs:
    - name: my-gateway
      namespace: infra
  hostnames:
    - matlistan.example.org
```

```sh
helm install matlistan oci://ghcr.io/jalet/helm-charts/matlistan --version X.Y.Z \
  -n matlistan --create-namespace -f values.yaml
```

The app applies its database migrations at startup. It runs as one replica by design:
planning and swapping run in the web process.

To check a release's signature before installing:

```sh
cosign verify ghcr.io/jalet/matlistan:X.Y.Z \
  --certificate-identity-regexp 'https://github.com/jalet/matlistan/' \
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
selectors. Egress is limited to DNS, the database and port 443 (the OIDC provider and
`api.anthropic.com`). Metrics are served on port 9091, which the HTTPRoute does not expose.

## Personal data

When a week is planned, Matlistan sends each family member's age, diets, allergies, likes
and dislikes to the Anthropic API. Names are never sent. Allergies can count as data
concerning health under GDPR Article 9 (Regulation (EU) 2016/679, applicable 2018-05-25),
and Anthropic processes the request in the United States, a transfer governed by GDPR
Chapter V (Articles 44 to 49). A purely private household use may fall under the household
exemption in GDPR Article 2(2)(c), but that has not been verified. **VERIFY WITH LEGAL
COUNSEL** before anyone outside your own household uses the app.

Everything else (names, history, ratings, shopping lists) stays in your database.

> This document provides technical guidance based on published regulatory frameworks. It
> does not constitute legal advice. Consult qualified legal counsel for compliance
> decisions.
