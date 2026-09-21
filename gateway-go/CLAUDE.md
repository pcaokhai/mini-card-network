# gateway-go — CLAUDE.md

Acquirer gateway. Serves the REST/WebSocket API used by the web console, builds and sends ISO 8583 to the issuer (through Toxiproxy), matches responses, owns the SAF queue, and hosts the switch binary (STIP, circuit breaker) from Sprint 10.

Lane: **GW**. Owns: `gateway-go/**`, database `acquirer`, migrations `gateway-go/migrations/`.

## Commands

```bash
make -C gateway-go generate   # oapi-codegen (strict server) + sqlc
make -C gateway-go test       # go test ./... -race -count=1
make -C gateway-go itest      # integration tests (Testcontainers Postgres, in-process fake issuer)
make -C gateway-go lint       # golangci-lint run
make -C gateway-go run        # go run ./cmd/gateway
```

## Layout

```
cmd/gateway/          main: config load, wiring, graceful shutdown
cmd/switch/           main for the switch (Sprint 10)
internal/
  iso8583/            Codec: MTI, bitmap, field specs, packager (ADR-003). Pure, no I/O.
  isonet/             Framing (2-byte length header), connection manager, sign-on/echo, reconnect
  mux/                Request/response matching by (STAN, field 7), timeouts, late-response hook
  txn/                Domain: Transaction aggregate, state machine, commands, errors. No I/O.
  saf/                Store-and-forward worker (SKIP LOCKED), backoff policy, repeat marking
  hsm/                Security module port + simulator (PIN translate, MAC)
  switch/             BIN routing, circuit breaker, STIP rules (Sprint 10)
  api/                HTTP (chi + generated strict server), WebSocket hub, problem+json mapper
  store/              pgx + sqlc repositories, transactions, migrations runner (goose)
  chaos/              Toxiproxy admin client
  obs/                slog JSON handler with masking, OpenTelemetry setup, metrics
  config/             Typed config from env, validated on start
migrations/           goose SQL migrations (acquirer schema)
```

## Go rules

- `context.Context` is the first parameter of every function that does I/O or may block. Respect cancellation everywhere.
- Every goroutine is started by an owner that can stop it (`errgroup.Group` or a `Run(ctx) error` method). No fire-and-forget goroutines. `go test -race` must pass.
- Errors: wrap with `%w` and context (`fmt.Errorf("send 0200 stan=%s: %w", stan, err)`). Domain errors are typed (`var ErrDuplicate = errors.New(...)` or structs implementing `error`); check with `errors.Is/As`. Map to HTTP in `api/errors.go` only; map to RC in `txn/rc.go` only.
- No `panic` outside `main` init. No global mutable state; pass dependencies explicitly via constructors (`NewService(deps...)`).
- Interfaces are small and defined where they are consumed. Return concrete types.
- The `txn` package is the domain core: pure functions and a state machine (`Created → Sent → Approved | Declined | TimedOut → ReversalPending → Reversed`). Illegal transitions return an error; every transition is recorded in `tran_state_history`.
- **MUX:** register the pending request *before* writing to the socket. Timeout path must enqueue the reversal in the same DB transaction that marks `TIMED_OUT`.
- **SAF worker:** `SELECT ... FOR UPDATE SKIP LOCKED LIMIT n`, exponential backoff with full jitter (2 s base, 60 s cap), `attempts > 0` ⇒ send x21 repeat, `max_attempts` ⇒ `DEAD` + alert metric.
- Money is `int64` minor units wrapped in `txn.Money`; never `float64`.
- Logging with `slog` JSON via `obs.Logger`; card data only through `obs.MaskPAN` and friends. Never log raw ISO bytes outside the lab endpoints, and the lab endpoints mask on output.
- Config and time are injected (`Clock` interface) so tests are deterministic.

## Tests

- Table-driven tests with `t.Run`; `testify/require` for assertions.
- Codec tests: golden vectors from `contracts/iso8583/vectors/` must round-trip byte-identical; fuzz tests (`go test -fuzz`) for unpack.
- Cross-check: CI runs the same vectors through `moov-io/iso8583` to validate our codec (test-only dependency).
- API tests validate every response against `contracts/openapi.yaml` (kin-openapi validator middleware in test mode).
- Integration: Testcontainers Postgres + an in-process fake issuer that can delay, drop, duplicate and reorder responses.
