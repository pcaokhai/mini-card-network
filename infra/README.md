# infra — local platform

Docker Compose stack for everything that isn't application code: Postgres (3 databases, one role
each), Kafka (KRaft, single node), Toxiproxy (fault injection on the ISO link), the observability
stack (OTel Collector, Tempo, Prometheus, Grafana).

Application services (`issuer-jpos`, `gateway-go`, `web-next`, `settlement`) add their own
`<service>/compose.yaml` in their own stories; the root `Makefile` discovers every `*/compose.yaml`
automatically — this directory never needs to change when a service is added.

## Commands

```bash
make up            # copies .env.example -> .env on first run, starts everything, waits healthy
make down          # stop containers, keep volumes
make down ARGS=-v  # stop and wipe all state (databases, Kafka log, Tempo traces)
```

## Ports

| Service | Port(s) | Notes |
| --- | --- | --- |
| postgres | `${POSTGRES_HOST_PORT:-5432}` | override in `.env` if 5432 is already taken by another project |
| kafka | 9092 | KRaft single node; topics `issuer.transactions.v1`, `acquirer.transactions.v1` created by `infra/scripts/post-up.sh` |
| toxiproxy | 8474 (admin API), 18000 (proxy → `issuer:8000`) | |
| otel-collector | 4317 (OTLP gRPC), 4318 (OTLP HTTP), 13133 (health) | |
| tempo | 3200 | no Docker healthcheck (image has no shell); `make up`/smoke test poll `/ready` instead |
| prometheus | 9090 | |
| grafana | 3001 (host) → 3000 (container) | anonymous viewer access enabled; admin password from `.env` |

Full service port table (including app services, once scaffolded): `docs/02-software-architecture.md` §8.

## Troubleshooting

- **Port already allocated** (commonly Postgres 5432, if you run other projects locally): set
  `POSTGRES_HOST_PORT` in `.env` to a free port before `make up`. Every other host binding can be
  freed the same way if needed — check with `lsof -nP -iTCP:<port> -sTCP:LISTEN`.
- **Tempo permission errors on first start**: the image's container user can't write to the
  `tempodata` volume on some hosts. The compose file already sets `user: "0"` (root) for the
  `tempo` service as a lab-only workaround; if you still see it, check the volume wasn't created
  with different ownership by an older run (`make down ARGS=-v` first).
- **Reset everything**: `make down ARGS=-v` removes all containers, the network and all volumes.
- **Verify the whole stack**: `bash infra/tests/smoke.sh` — proves every MCN-002 acceptance
  criterion against a live stack and tears it down again.
