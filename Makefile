.PHONY: up down test lint fmt contracts e2e

COMPOSE := infra/docker-compose.yml

up:
	@if [ -f $(COMPOSE) ]; then docker compose -f $(COMPOSE) up -d; \
	else echo "skip: $(COMPOSE) not scaffolded yet (MCN-002)"; fi

down:
	@if [ -f $(COMPOSE) ]; then docker compose -f $(COMPOSE) down -v; \
	else echo "skip: $(COMPOSE) not scaffolded yet (MCN-002)"; fi

test:
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

lint:
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

fmt:
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

contracts:
	@./scripts/check-contracts.sh

e2e:
	@if [ -f web-next/package.json ]; then \
		echo "== e2e web-next =="; (cd web-next && npx playwright test); \
	else echo "skip: web-next not scaffolded yet"; fi
