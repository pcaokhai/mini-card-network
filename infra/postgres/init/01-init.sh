#!/usr/bin/env bash
# Runs once on an empty data directory. One database and one owner role per organization (ADR-004).
set -euo pipefail
for org in issuer acquirer settlement; do
  var="$(echo "$org" | tr a-z A-Z)_DB_PASSWORD"
  pw="${!var:?missing $var}"
  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres <<-EOSQL
    CREATE ROLE ${org} LOGIN PASSWORD '${pw}';
    CREATE DATABASE ${org} OWNER ${org};
    REVOKE CONNECT ON DATABASE ${org} FROM PUBLIC;
    GRANT CONNECT ON DATABASE ${org} TO ${org};
EOSQL
done
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres -c "REVOKE CONNECT ON DATABASE postgres FROM PUBLIC;"
