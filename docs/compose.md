# Running Matlistan with Docker Compose

For a home server without Kubernetes. [deploy/compose](../deploy/compose) holds a compose
file for Matlistan and PostgreSQL 18, and an example `.env`. It works with Docker Compose and
with `podman compose`.

## What you need

- Docker with Compose v2, or Podman.
- An OpenID Connect provider, for example Pocket ID, Authelia, Authentik or Keycloak.
  Matlistan has no passwords of its own: everyone signs in through it.
- An API key for Anthropic or OpenAI, or an OpenAI-compatible server reachable over https
  (see "Choosing the model provider" in [deploy.md](deploy.md)).
- For phones: a reverse proxy with TLS in front, such as Caddy or Traefik. Web push and
  "Add to Home Screen" need https.

## Set it up

1. Copy `deploy/compose` to the server, for example to `/opt/matlistan`, and work there.

2. Create the `.env` file and fill it in:

   ```sh
   cp .env.example .env
   ```

   `MATLISTAN_BASE_URL` is the address people open, and `DB_PASSWORD` a long random
   password (`openssl rand -hex 24`).

3. Create the secrets. The directory keeps them private on the host; the files themselves
   must be readable by the container's user (uid 65532):

   ```sh
   mkdir -m 700 secrets
   openssl rand -hex 32 > secrets/session_key
   printf '%s' 'the OIDC client secret' > secrets/oidc_client_secret
   printf '%s' 'the model API key' > secrets/llm_api_key
   chmod 644 secrets/*
   ```

   For an OpenAI-compatible server without a key, leave `secrets/llm_api_key` empty.

4. Register Matlistan as a client in your OIDC provider. The redirect URI is
   `MATLISTAN_BASE_URL` followed by `/auth/callback`. Signing in needs the ID token's
   `OIDC_CLAIM` (default `groups`) to contain one of `OIDC_ALLOWED`, so put the family in
   that group.

5. Start it:

   ```sh
   docker compose up -d
   docker compose logs -f matlistan
   ```

   Matlistan creates and migrates its tables when it starts. It listens on
   `127.0.0.1:8080`; point the reverse proxy there. With Caddy:

   ```
   matlistan.example.com {
       reverse_proxy 127.0.0.1:8080
   }
   ```

## The Sunday plan

In Kubernetes a CronJob drafts next week every Sunday. With Compose, run the `generate`
service from the host's crontab instead:

```cron
0 7 * * 0  cd /opt/matlistan && docker compose run --rm generate
```

"Plan the week" in the app works without it.

## Upgrading

Change the image tag in `compose.yaml` to the new release, then:

```sh
docker compose pull
docker compose up -d
```

Migrations run when the new version starts.

## Backups

The database holds the family's ages, allergies and diets. Allergies can be health data, so
keep the `db` volume on an encrypted disk and the dumps somewhere just as private:

```sh
docker compose exec db pg_dump -U matlistan matlistan > matlistan.sql
```

## More

- The family and recipes can be declared in git ([household-as-code.md](household-as-code.md)).
  With Compose, write the `members`, `recipes` and `recipeURLs` from that page into a YAML
  file, mount it read-only and set `MATLISTAN_HOUSEHOLD_FILE` to its path.
- Web push needs a VAPID key: [notifications.md](notifications.md).
- The shopping list in Apple Notes: [shortcut.md](shortcut.md).
