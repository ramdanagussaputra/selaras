# Design D9: Node web build → Go build with embedded assets + migrate CLI →
# distroless static, non-root, < 30 MB.

# ── Stage 1: build the React app ────────────────────────────────────────
FROM node:24-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ── Stage 2: build the Go binary (embeds web/dist) + migrate CLI ────────
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
# Replace the committed placeholder with the real Vite build before compiling.
COPY --from=web /web/dist internal/adapter/http/dist
RUN CGO_ENABLED=0 go build -ldflags='-s -w' -o /out/api ./cmd/api
RUN CGO_ENABLED=0 GOBIN=/out go install -tags 'postgres' -ldflags='-s -w' \
    github.com/golang-migrate/migrate/v4/cmd/migrate@v4.19.1

# ── Stage 3: minimal runtime ─────────────────────────────────────────────
# distroless/static over scratch: CA certs + nonroot user out of the box.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /api
COPY --from=build /out/migrate /migrate
COPY server/migrations /migrations
EXPOSE 8080
# Pre-deploy step (design D3): /migrate -path /migrations -database $DATABASE_URL up
ENTRYPOINT ["/api"]
