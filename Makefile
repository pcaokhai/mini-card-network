.PHONY: help up down test lint fmt contracts e2e chaos pci-scan seed

COMPOSE := infra/docker-compose.yml

help: ## List available targets
	@grep -E '^[a-z0-9-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-10s %s\n", $$1, $$2}'

up: ## Start infra + all services (docker compose)
	@if [ -f $(COMPOSE) ]; then \
		test -f .env || cp .env.example .env; \
		docker compose --project-directory . -f $(COMPOSE) --env-file .env up -d --wait; \
		if [ -x infra/scripts/post-up.sh ]; then infra/scripts/post-up.sh; fi; \
	else echo "skip: $(COMPOSE) not scaffolded yet (MCN-002)"; fi

down: ## Stop infra + all services; ARGS=-v to also remove volumes
	@if [ -f $(COMPOSE) ]; then \
		docker compose --project-directory . -f $(COMPOSE) --env-file .env down $(ARGS); \
	else echo "skip: $(COMPOSE) not scaffolded yet (MCN-002)"; fi

test: ## Unit + integration tests, every service
	@for d in issuer-jpos settlement; do \
		if [ -f $$d/build.gradle.kts ] || [ -f $$d/build.gradle ]; then \
			echo "== test $$d =="; (cd $$d && ./gradlew test); \
		else echo "skip: $$d not scaffolded yet"; fi; \
	done
	@if [ -f gateway-go/go.mod ]; then \
		echo "== test gateway-go =="; (cd gateway-go && go test ./...); \
	else echo "skip: gateway-go not scaffolded yet"; fi
	@if [ -f web-next/package.json ]; then \
		echo "== test web-next =="; (cd web-next && npm test); \
	else echo "skip: web-next not scaffolded yet"; fi

lint: ## All linters + formatters in check mode
	@for d in issuer-jpos settlement; do \
		if [ -f $$d/build.gradle.kts ] || [ -f $$d/build.gradle ]; then \
			echo "== lint $$d =="; (cd $$d && ./gradlew spotlessCheck); \
		else echo "skip: $$d not scaffolded yet"; fi; \
	done
	@if [ -f gateway-go/go.mod ]; then \
		echo "== lint gateway-go =="; (cd gateway-go && golangci-lint run ./...); \
	else echo "skip: gateway-go not scaffolded yet"; fi
	@if [ -f web-next/package.json ]; then \
		echo "== lint web-next =="; (cd web-next && npm run lint); \
	else echo "skip: web-next not scaffolded yet"; fi

fmt: ## Auto-format everything
	@for d in issuer-jpos settlement; do \
		if [ -f $$d/build.gradle.kts ] || [ -f $$d/build.gradle ]; then \
			echo "== fmt $$d =="; (cd $$d && ./gradlew spotlessApply); \
		else echo "skip: $$d not scaffolded yet"; fi; \
	done
	@if [ -f gateway-go/go.mod ]; then \
		echo "== fmt gateway-go =="; (cd gateway-go && gofmt -l -w .); \
	else echo "skip: gateway-go not scaffolded yet"; fi
	@if [ -f web-next/package.json ]; then \
		echo "== fmt web-next =="; (cd web-next && npm run fmt); \
	else echo "skip: web-next not scaffolded yet"; fi

contracts: ## Lint OpenAPI, validate WS schema, run ISO golden vectors
	@./scripts/check-contracts.sh

e2e: ## Playwright journeys against the running stack
	@if [ -f web-next/package.json ]; then \
		echo "== e2e web-next =="; (cd web-next && npx playwright test); \
	else echo "skip: web-next not scaffolded yet"; fi

chaos: ## Chaos suite with ledger invariant check (MCN-407)
	@echo "not implemented until MCN-407"; exit 2

pci-scan: ## Scan logs and fixtures for PAN / PIN block / track data (MCN-506)
	@echo "not implemented until MCN-506"; exit 2

seed: ## Load contracts/fixtures into issuer and acquirer databases
	@for d in issuer-jpos gateway-go; do \
		if [ -f "$$d/Makefile" ]; then echo "== seed $$d =="; $(MAKE) --no-print-directory -C "$$d" seed; \
		else echo "skip: $$d not scaffolded yet"; fi; \
	done
