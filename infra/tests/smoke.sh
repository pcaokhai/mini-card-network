#!/usr/bin/env bash
# Proves MCN-002 AC1–AC2 against a running stack. Usage: bash infra/tests/smoke.sh
set -euo pipefail
cd "$(dirname "$0")/../.."
fail() { echo "FAIL: $*" >&2; exit 1; }
dc() { docker compose --project-directory . -f infra/docker-compose.yml --env-file .env "$@"; }

start=$(date +%s)
make up >/dev/null
elapsed=$(( $(date +%s) - start ))
echo "make up took ${elapsed}s"
[ "$elapsed" -lt 90 ] || fail "make up took ${elapsed}s (MCN-002-AC2 requires < 90s warm)"

dbs=$(dc exec -T postgres psql -U postgres -tAc "select string_agg(datname, ',' order by datname) from pg_database where datname in ('issuer','acquirer','settlement')")
[ "$dbs" = "acquirer,issuer,settlement" ] || fail "databases missing: got '$dbs' (MCN-002-AC1)"
for role in issuer acquirer settlement; do
  pw_var="$(echo "${role}" | tr a-z A-Z)_DB_PASSWORD"
  pw=$(grep "^${pw_var}=" .env | cut -d= -f2-)
  dc exec -T -e PGPASSWORD="$pw" postgres psql -h localhost -U "$role" -d "$role" -tAc "select 1" | grep -q 1 \
    || fail "role $role cannot log in to its own database"
  if dc exec -T -e PGPASSWORD="$pw" postgres psql -h localhost -U "$role" -d postgres -tAc "select 1" >/dev/null 2>&1; then
    fail "role $role must not connect to other databases"
  fi
done

topics=$(dc exec -T kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list)
grep -qx "issuer.transactions.v1" <<<"$topics" || fail "topic issuer.transactions.v1 missing"
grep -qx "acquirer.transactions.v1" <<<"$topics" || fail "topic acquirer.transactions.v1 missing"

curl -fsS localhost:8474/proxies | grep -q '"issuer"' || fail "toxiproxy proxy 'issuer' missing"
curl -fsS localhost:13133/ >/dev/null || fail "otel collector health endpoint down"
# Tempo's ingester needs a short settle time after startup before /ready succeeds;
# it has no Docker healthcheck (no shell in the image), so poll here instead.
tempo_ready=false
for _ in $(seq 1 20); do
  if curl -fsS localhost:3200/ready 2>/dev/null | grep -qi ready; then tempo_ready=true; break; fi
  sleep 2
done
[ "$tempo_ready" = true ] || fail "tempo not ready after 40s"
curl -fsS localhost:9090/-/ready >/dev/null || fail "prometheus not ready"
curl -fsS localhost:3001/api/health | grep -q '"database": *"ok"' || fail "grafana not healthy"
curl -fsS -u "admin:$(grep '^GRAFANA_ADMIN_PASSWORD=' .env | cut -d= -f2-)" localhost:3001/api/datasources \
  | grep -q '"type":"tempo"' || fail "grafana tempo datasource not provisioned"

make down ARGS=-v >/dev/null
[ -z "$(docker volume ls -q --filter label=com.docker.compose.project=mini-card-network)" ] || fail "volumes left after down -v"
echo "PASS smoke"
