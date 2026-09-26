# Matlistan

A weekly meal planner for the household. Every Sunday it drafts next week's
dinners from the family's preferences, ages, allergies and history, lets you
swap and approve meals, and produces a shopping list that an iOS Shortcut
pulls into Apple Notes.

Status: design phase.

## Development

Tools come from `mise install`. Tests need Docker or Podman (testcontainers).

    mise run dev:db    # throwaway Postgres on :5432
    mise run devidp    # fake OIDC provider on 127.0.0.1:5556
    mise run dev       # http://localhost:8080

`mise run test`, `mise run lint`, `mise run templ` and `mise run css` cover the rest.
See [docs/shortcut.md](docs/shortcut.md) for putting the shopping list in Apple Notes.
English is the default; set `MATLISTAN_LOCALE=sv` and `MATLISTAN_TIMEZONE=Europe/Stockholm`
for the homelab setup.

## Licence

[Apache License 2.0](LICENSE)
