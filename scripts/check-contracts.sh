#!/usr/bin/env bash
# ponytail: real checks are JSON-only (no new deps); MCN-003 upgrades this to
# Spectral + oasdiff + full ISO vector round-trip once those tools are wired in.
set -euo pipefail
cd "$(dirname "$0")/.."

fail=0

check_json() {
	local f="$1"
	if ! node -e "JSON.parse(require('fs').readFileSync('$f','utf8'))" 2>/dev/null; then
		echo "INVALID JSON: $f"
		fail=1
	fi
}

if [ ! -s contracts/openapi.yaml ]; then
	echo "MISSING/EMPTY: contracts/openapi.yaml"
	fail=1
fi

check_json contracts/ws-events.schema.json
check_json contracts/fixtures/cards.json
check_json contracts/events/transaction-event.schema.json

shopt -s nullglob
for f in contracts/iso8583/vectors/*.json; do
	check_json "$f"
done

if [ "$fail" -eq 0 ]; then
	echo "contracts: OK"
else
	echo "contracts: FAILED"
fi
exit "$fail"
