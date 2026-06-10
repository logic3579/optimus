# Multi-stage Dockerfile for the optimus-be Go services.
# Build context MUST be the repo root (not optimus-be/) so this file can
# also access deploy/nginx.conf via the fe Dockerfile build.
#
# Targets:
#   server       — the main HTTP API (cmd/server)
#   migrate      — goose schema runner (cmd/migrate, init container)
#   seed         — permission registry + bootstrap admin (cmd/seed, init container)
#   vault-keygen — P1 master-key minter (cmd/vault-keygen, one-shot CLI)

FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache git

# Cache go.sum / go.mod resolution as its own layer
COPY optimus-be/go.mod optimus-be/go.sum ./
RUN go mod download

COPY optimus-be/ ./

ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.Version=${VERSION}" \
    -o /out/optimus-be ./cmd/server
RUN CGO_ENABLED=0 go build -ldflags "-s -w" \
    -o /out/optimus-migrate ./cmd/migrate
RUN CGO_ENABLED=0 go build -ldflags "-s -w" \
    -o /out/optimus-seed ./cmd/seed
RUN CGO_ENABLED=0 go build -ldflags "-s -w" \
    -o /out/optimus-vault-keygen ./cmd/vault-keygen

# ---- runtime: server ----
FROM alpine:3.20 AS server
RUN apk add --no-cache ca-certificates tzdata wget
COPY --from=build /out/optimus-be /usr/local/bin/optimus-be
COPY optimus-be/configs/config.yaml /etc/optimus/config.yaml
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/optimus-be"]
CMD ["-config", "/etc/optimus/config.yaml"]

# ---- runtime: migrate ----
FROM alpine:3.20 AS migrate
RUN apk add --no-cache ca-certificates
COPY --from=build /out/optimus-migrate /usr/local/bin/optimus-migrate
COPY optimus-be/configs/config.yaml /etc/optimus/config.yaml
ENTRYPOINT ["/usr/local/bin/optimus-migrate"]
CMD ["-config", "/etc/optimus/config.yaml", "-dir", "up"]

# ---- runtime: seed ----
FROM alpine:3.20 AS seed
RUN apk add --no-cache ca-certificates
COPY --from=build /out/optimus-seed /usr/local/bin/optimus-seed
COPY optimus-be/configs/config.yaml /etc/optimus/config.yaml
ENTRYPOINT ["/usr/local/bin/optimus-seed"]
CMD ["-config", "/etc/optimus/config.yaml"]

# ---- runtime: vault-keygen (small one-shot CLI to mint a master key) ----
FROM alpine:3.20 AS vault-keygen
COPY --from=build /out/optimus-vault-keygen /usr/local/bin/optimus-vault-keygen
ENTRYPOINT ["/usr/local/bin/optimus-vault-keygen"]
