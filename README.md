# Matlistan

A weekly meal planner for the household. Every Sunday it drafts next week's
dinners from the family's preferences, ages, allergies and history. You lock
the dinners you want to keep, plan the rest again and approve the week, and it
produces a shopping list that an iOS Shortcut pulls into Apple Notes. Recipes can be typed in or imported from a link to a recipe page.

Status: in use at home; see [docs/deploy.md](docs/deploy.md) to run it.

## Development

Tools come from `mise install`. Tests need Docker or Podman (testcontainers).

    mise run dev:db    # throwaway Postgres on :5432
    mise run devidp    # fake OIDC provider on 127.0.0.1:5556
    mise run dev       # http://localhost:8080

`mise run test`, `mise run lint`, `mise run templ` and `mise run css` cover the rest.
See [docs/shortcut.md](docs/shortcut.md) for putting the shopping list in Apple Notes.

## Deploy

The Helm chart is in `charts/matlistan` and published to
`oci://ghcr.io/aldersfors/helm-charts/matlistan`; [docs/deploy.md](docs/deploy.md) walks through
the Secrets, the OIDC client and an install. `mise run chart:test` renders and checks it.
English is the default; set `MATLISTAN_LOCALE=sv` and `MATLISTAN_TIMEZONE=Europe/Stockholm`
for the homelab setup.

## Licence

[Apache License 2.0](LICENSE)
