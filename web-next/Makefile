SHELL := /usr/bin/env bash
.SHELLFLAGS := -euo pipefail -c
.PHONY: install test lint fmt seed e2e

install:
	pnpm install --frozen-lockfile

test: install
	pnpm test

lint: install
	pnpm lint
	pnpm typecheck
	pnpm generate
	git diff --exit-code -- src/shared/api/generated src/mocks/generated || { echo "generated code is stale: run pnpm generate"; exit 1; }

fmt: install
	pnpm exec eslint --fix .

seed:
	@echo "web-next has no database"

e2e:
	@echo "not implemented until MCN-905"; exit 2
