# contracts/Makefile — MCN-003. `make contracts` at the root runs `check`.
SHELL := /usr/bin/env bash
.SHELLFLAGS := -euo pipefail -c
SPECTRAL := npx -y @stoplight/spectral-cli@6
.PHONY: check deps lint ruleset-selftest vectors schemas unit breaking

check: deps lint ruleset-selftest vectors schemas unit breaking

deps:
	@python3 -m pip install -q -r requirements.txt

lint: ## MCN-003-AC1
	$(SPECTRAL) lint openapi.yaml --ruleset .spectral.yaml --fail-severity error

ruleset-selftest: ## the project rules must reject a known-bad spec
	@out=$$($(SPECTRAL) lint scripts/testdata/bad-openapi.yaml --ruleset .spectral.yaml --fail-severity error 2>/dev/null || true); \
	for r in mcn-idempotency-key-on-writes mcn-kebab-case-paths mcn-camel-case-properties mcn-writes-have-problem-response; do \
	  grep -q "$$r" <<<"$$out" || { echo "ruleset self-test: rule $$r did not fire"; exit 1; }; \
	done; echo "ruleset self-test ok"

vectors: ## MCN-003-AC3
	python3 scripts/validate_vectors.py

schemas: ## MCN-003-AC4
	python3 scripts/check_schemas.py

unit:
	cd scripts && python3 -m pytest -q

breaking: ## MCN-003-AC2
	bash scripts/breaking.sh
