SHELL := /usr/bin/env bash
.SHELLFLAGS := -euo pipefail -c
.PHONY: test lint fmt seed run

test:
	go test -race -count=1 ./...

lint:
	test -z "$$(gofmt -l .)" || { gofmt -l .; echo "run make fmt"; exit 1; }
	golangci-lint run ./...

fmt:
	gofmt -w .
	golangci-lint fmt ./...

# Needs the stack up (`make up`) and the issuer seeded first; the root `make seed` runs issuer-jpos
# before gateway-go. See cmd/seed for what it writes.
seed:
	@set -a; [ -f ../.env ] && . ../.env; set +a; \
	DATABASE_URL="postgres://acquirer:$${ACQUIRER_DB_PASSWORD:-lab-acquirer}@localhost:$${POSTGRES_HOST_PORT:-5432}/acquirer" \
	CGO_ENABLED=0 go run ./cmd/seed # pure Go; avoids a host C toolchain dependency

run:
	go run ./cmd/gateway
