SHELL := /usr/bin/env bash
.SHELLFLAGS := -euo pipefail -c
.PHONY: test lint fmt seed run
test: ; ./gradlew --no-daemon test
lint: ; ./gradlew --no-daemon spotlessCheck
fmt:  ; ./gradlew --no-daemon spotlessApply
seed: ; ./gradlew --no-daemon run -PmainClass=io.mcn.issuer.adapter.seed.SeedMain
run:  ; ./gradlew --no-daemon run
