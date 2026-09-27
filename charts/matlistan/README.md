# matlistan Helm chart

Runs [Matlistan](https://github.com/aldersfors/matlistan), a weekly meal planner that drafts
next week's dinners every Sunday and builds the shopping list. The chart deploys the web
app, a CronJob for the Sunday draft, and optionally an HTTPRoute, a NetworkPolicy and a
ServiceMonitor.

See [docs/deploy.md](https://github.com/aldersfors/matlistan/blob/main/docs/deploy.md) for the
Secrets it needs, the OIDC client and an example install.
