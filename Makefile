# Selaras developer interface — the only commands you need day to day.
# Tools are pinned and run via `go run`, so no global installs are required.

DATABASE_URL ?= postgres://selaras:selaras@localhost:5432/selaras?sslmode=disable
CORS_ORIGIN  ?= http://localhost:5173
# Dev-only signing secret (≥32 bytes). Production sets a real JWT_SECRET in Render.
JWT_SECRET   ?= dev-only-insecure-jwt-secret-change-me

MIGRATE        := cd server && go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.19.1
GOLANGCI_LINT  := cd server && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
MIGRATIONS_DIR := migrations

.PHONY: dev dev-server dev-web test test-server test-web lint lint-server lint-web \
        migrate-up migrate-down migrate-new

## dev: run the Go API (:8080) and the Vite dev server (:5173) together
dev:
	$(MAKE) -j2 dev-server dev-web

dev-server:
	cd server && DATABASE_URL='$(DATABASE_URL)' CORS_ORIGIN='$(CORS_ORIGIN)' JWT_SECRET='$(JWT_SECRET)' go run ./cmd/api

dev-web:
	cd web && npm run dev

## test: Go suite (incl. dependency-rule test) + web suite; fails if either fails
test: test-server test-web

test-server:
	cd server && go vet ./... && go test ./...

test-web:
	cd web && npm run test -- --run

## lint: golangci-lint for server, eslint + tsc for web
lint: lint-server lint-web

lint-server:
	$(GOLANGCI_LINT) run

lint-web:
	cd web && npm run lint && npm run typecheck

migrate-up:
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database '$(DATABASE_URL)' up

migrate-down:
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database '$(DATABASE_URL)' down 1

## migrate-new name=add_boards: create a sequential up/down migration pair
migrate-new:
	@test -n "$(name)" || (echo "usage: make migrate-new name=<snake_case_name>" && exit 1)
	$(MIGRATE) create -dir $(MIGRATIONS_DIR) -seq -digits 4 -ext sql $(name)
