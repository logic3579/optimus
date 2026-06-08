.PHONY: run build test test-int lint swag swagger-diff migrate-up migrate-down migrate-new seed dump-perms perm-check perm-db-check air-install goose-install tools

DSN ?= host=localhost port=5432 user=optimus password=optimus dbname=optimus sslmode=disable

run:
	air

build:
	go build -o bin/optimus-be ./cmd/server

test:
	go test ./... -race -cover

test-int:
	go test ./... -tags=dbtest -race

lint:
	golangci-lint run

swag:
	swag init -g cmd/server/main.go -o api/docs --parseDependency --parseInternal
	cp api/docs/swagger.json ../docs/api/swagger.json

swagger-diff:
	@swag init -g cmd/server/main.go -o /tmp/optimus-swag --parseDependency --parseInternal >/dev/null
	@diff -q /tmp/optimus-swag/swagger.json ../docs/api/swagger.json || \
	  (echo "swagger.json is stale — run 'make swag' and commit"; exit 1)

migrate-up:
	goose -dir migrations postgres "$(DSN)" up

migrate-down:
	goose -dir migrations postgres "$(DSN)" down

migrate-new:
	@test -n "$(name)" || (echo "usage: make migrate-new name=<name>"; exit 1)
	goose -dir migrations create $(name) sql

seed:
	go run ./cmd/seed

dump-perms:
	go run ./cmd/dump-permissions > ../docs/permissions.md

perm-check:
	@go run ./cmd/dump-permissions > /tmp/optimus-perms.md
	@diff -q /tmp/optimus-perms.md ../docs/permissions.md || \
	  (echo "permissions.md is stale — run 'make dump-perms' and commit"; exit 1)

perm-db-check:
	go run ./cmd/server -check-permissions

tools:
	go install github.com/air-verse/air@v1.52.3
	go install github.com/pressly/goose/v3/cmd/goose@v3.20.0
	go install github.com/swaggo/swag/cmd/swag@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
