#!/usr/bin/env bash
# MCN-003-AC2: fail on breaking OpenAPI changes unless the PR carries the api-breaking label (API_BREAKING=true).
set -euo pipefail
cd "$(dirname "$0")/.."
BASE_REF="${BASE_REF:-origin/main}"
if [ "${API_BREAKING:-false}" = "true" ]; then echo "api-breaking label present (ADR required): skipping"; exit 0; fi
if ! git cat-file -e "${BASE_REF}:contracts/openapi.yaml" 2>/dev/null; then echo "no base spec at ${BASE_REF}: skipping"; exit 0; fi
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
git show "${BASE_REF}:contracts/openapi.yaml" > "$tmp/base.yaml"
docker run --rm -v "$tmp:/base:ro" -v "$PWD:/head:ro" tufin/oasdiff breaking /base/base.yaml /head/openapi.yaml --fail-on ERR
echo "no breaking changes"
