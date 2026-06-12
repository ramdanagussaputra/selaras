# Selaras

Real-time collaborative kanban — Go 1.26 + React 19 monorepo.

**Live demo**: _coming soon_ <!-- TODO(task 8.4): replace with the Railway URL -->

## Stack

- **API** — Go 1.26, chi, pgx, golang-migrate, `log/slog`; pragmatic hexagonal
  layout with the Clean Architecture dependency rule enforced by a test
- **Web** — React 19 + TypeScript + Vite, TanStack Query, React Router v7, Tailwind
- **Infra** — Postgres 16, multi-stage Docker (distroless, < 30 MB), GitHub Actions, Railway

## Local setup (3 commands)

Prerequisites: Docker, Go 1.26+, Node 20+.

```sh
docker compose up -d   # Postgres 16 on :5432
make migrate-up        # apply database migrations
make dev               # Go API on :8080 + Vite dev server on :5173
```

Open http://localhost:5173 — the shell reports API/database health via `GET /healthz`.

## Development

| Command             | What it does                                      |
| ------------------- | ------------------------------------------------- |
| `make dev`          | API + web dev servers (Vite proxies to the API)   |
| `make test`         | Go tests (incl. dependency-rule test) + web tests |
| `make lint`         | golangci-lint + eslint + `tsc --noEmit`           |
| `make migrate-up`   | apply migrations                                  |
| `make migrate-down` | roll back one migration                           |
| `make migrate-new name=…` | create a new migration pair                 |

## Architecture decisions

See [docs/adr/](docs/adr/).
