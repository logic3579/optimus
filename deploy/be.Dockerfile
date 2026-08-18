# syntax=docker/dockerfile:1

# One backend image contains the server and all operational CLIs. Compose keeps
# them as separate services and selects the required binary with entrypoint.
# Build context MUST be the repository root.

FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache git

COPY optimus-be/go.mod optimus-be/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY optimus-be/ ./

ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    mkdir -p /out && \
    CGO_ENABLED=0 go build -ldflags "-s -w -X main.Version=${VERSION}" \
      -o /out/optimus-be ./cmd/server && \
    CGO_ENABLED=0 go build -ldflags "-s -w" \
      -o /out/optimus-migrate ./cmd/migrate && \
    CGO_ENABLED=0 go build -ldflags "-s -w" \
      -o /out/optimus-seed ./cmd/seed && \
    CGO_ENABLED=0 go build -ldflags "-s -w" \
      -o /out/optimus-vault-keygen ./cmd/vault-keygen

FROM alpine:3.20 AS backend
RUN apk add --no-cache ca-certificates tzdata wget
COPY --from=build /out/optimus-be /usr/local/bin/optimus-be
COPY --from=build /out/optimus-migrate /usr/local/bin/optimus-migrate
COPY --from=build /out/optimus-seed /usr/local/bin/optimus-seed
COPY --from=build /out/optimus-vault-keygen /usr/local/bin/optimus-vault-keygen
COPY optimus-be/configs/config.yaml /etc/optimus/config.yaml
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/optimus-be"]
CMD ["-config", "/etc/optimus/config.yaml"]
