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

seed:
	@echo "acquirer seed arrives with MCN-303"

run:
	go run ./cmd/gateway
