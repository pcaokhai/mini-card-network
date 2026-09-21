#!/usr/bin/env bash
# Idempotent platform setup after `docker compose up --wait`.
set -euo pipefail
cd "$(dirname "$0")/../.."
dc() { docker compose --project-directory . -f infra/docker-compose.yml --env-file .env "$@"; }
for topic in issuer.transactions.v1 acquirer.transactions.v1; do
  dc exec -T kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 \
    --create --if-not-exists --topic "$topic" --partitions 3 --replication-factor 1 >/dev/null
done
echo "post-up: kafka topics ready"
