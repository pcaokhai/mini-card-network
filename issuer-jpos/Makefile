SHELL := /usr/bin/env bash
.SHELLFLAGS := -euo pipefail -c
.PHONY: test lint fmt seed run
test: ; ./gradlew --no-daemon test
lint: ; ./gradlew --no-daemon spotlessCheck
fmt:  ; ./gradlew --no-daemon spotlessApply
seed:
	@set -a; [ -f ../.env ] && . ../.env; set +a; \
	DATABASE_URL="jdbc:postgresql://localhost:$${POSTGRES_HOST_PORT:-5432}/issuer" \
	DB_USER=issuer \
	DB_PASSWORD="$${ISSUER_DB_PASSWORD:-lab-issuer}" \
	PAN_ENCRYPTION_KEY_HEX="0000000000000000000000000000000000000000000000000000000000000000" \
	PAN_HMAC_KEY_HEX="1111111111111111111111111111111111111111111111111111111111111111" \
	./gradlew --no-daemon run -PmainClass=io.mcn.issuer.adapter.seed.SeedMain
run:  ; ./gradlew --no-daemon run
