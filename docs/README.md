# Abyss Documentation

Project documentation for the Abyss monorepo (Go core + Vue 3 UI in `www/`).

## Documents

| Document | Description |
|---|---|
| [project-context.md](project-context.md) | Architecture overview: vision, tech stack, directory map, BoltDB schema, storage engine, plugin system. |
| [auth-and-permissions.md](auth-and-permissions.md) | JWT authentication and the permission model. |
| [boltdb-usage.md](boltdb-usage.md) | Guidelines for working with BoltDB via `boltdb.go` / `storage.go` abstractions. |
| [plugin-development.md](plugin-development.md) | How to write and register plugins (backend + frontend integration). |
| [code-review.md](code-review.md) | Internal code audit report for the backend core (bugs, security findings). |
| [repo-split-plan.md](repo-split-plan.md) | Historical plan for splitting plugins/pro/www into separate repos (partially superseded: `www` has been merged back). |

## Quick Links

- Build & development guide: root [README.md](../README.md)
- Frontend guide: [www/README.md](../www/README.md)
