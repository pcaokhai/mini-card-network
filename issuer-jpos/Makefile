SHELL := /usr/bin/env bash
.SHELLFLAGS := -euo pipefail -c
.PHONY: test lint fmt seed run
test: ; ./gradlew --no-daemon test
lint: ; ./gradlew --no-daemon spotlessCheck
fmt:  ; ./gradlew --no-daemon spotlessApply
seed: ; @echo "issuer seed arrives with MCN-301"
run:  ; ./gradlew --no-daemon run
