# MCN-002 AC4 (acquirer half) — Seed that makes the Overview match the design

> **For Claude:** execute with superpowers:executing-plans (tightly coupled, one lane at a time), one worktree per PR, following root `CLAUDE.md` §4. Contract PR first.

**Goal:** after `make up && make seed`, the Overview page against the real backend shows the same kinds of data as the design canvas: several Vietnamese merchants, six cards, approvals plus each decline reason the stack can produce, a reversal, a filled 60-minute chart, a day-over-day delta and an active security key.

**Why now:** `docs/api/overview-page.md` §7 found `gateway-go`'s `make seed` is a stub (`acquirer seed arrives with MCN-303`), so the real page renders zeros for a single ASCII merchant. MCN-002 AC4 ("Seed command loads `contracts/fixtures/` into the issuer and acquirer databases") was only ever met on the issuer side.

**Spec:** `docs/api/overview-page.md` (§3.1 rules, §6 gaps, §7 seed check), `docs/06-user-stories.md` MCN-002 AC4, `docs/03-iso8583-interface-spec.md` §6/§8, root `CLAUDE.md` §6/§9.

## Rulings

1. **Decisions from the user (2026-09-25):** add fixture cards ••••1208 and ••••5540; backdating seeded rows is accepted, so the chart and the day-over-day delta have data; RC 55 is out of scope (ruling 4).
2. **Seed drives the real path, then backdates.** Every seeded transaction goes through `POST /v1/transactions/purchases` → ISO 0200 → issuer authorisation and ledger, so balances, `tran_log` and `tran_state_history` stay consistent (root CLAUDE.md §9: never bypass the ledger). Only afterwards does the seed shift gateway-side timestamps (`tran_log.created_at` and that transaction's `tran_state_history.created_at` rows, by the same delta, so latencies are unchanged). **Known cost:** issuer ledger rows keep their real timestamps, so a backdated row's acquirer date differs from its issuer date. Settlement reconciliation (R7, unbuilt) would see seeded rows on different business days. Acceptable for a lab seed; documented in the seed's `--help` and in docs/api/overview-page.md §7.
3. **Outcomes come from card state, never from forced codes.** 4417 / 5540 approve; 9021 (80 000 ₫) over-balance → RC 51; 1208 with a PER_TXN limit of 500 000 ₫ set through the issuer Card Admin API (`PUT /v1/cards/{cardRef}/limits`) → RC 61 above it; 3310 (BLOCKED) → RC 62; 7765 (expired 08/26) → RC 54; `POST /v1/transactions/{rrn}/cancellations` → REVERSED.
4. **RC 55 cannot be seeded.** The gateway never sends DE 52: `PurchaseRequest.encryptedPinBlock` is accepted and ignored, so the issuer's `VerifySecurity` PIN check never runs. Logged as risk R-12 with a separate story; the seed produces no "Sai mã PIN" rows.
5. **Merchant comes from the terminal.** `fixedMerchantID` / `fixedMerchantName` (purchase, advtxn) are replaced by a `terminal → merchant` lookup, so DE 42, `tran_log.mid` and the WS payload's `merchantName` agree. An unknown `terminalId` is a 422 problem (`unknown-terminal`) at the API boundary, never a DB error. The chaos runner's `TERM00000001` (12 chars against `tid CHAR(8)`, and not seeded) becomes the seeded `00000042`.
6. **Fixture is the source of truth for merchants and terminals.** The seed upserts `merchant`/`terminal` from `contracts/fixtures/cards.json` `terminals`. Migration 00002's single row stays, but its name is corrected to `Cà phê Góc Phố` by the upsert.
7. **Volume is bounded by fixture balances, and a rerun must not change outcomes.** Each run adds about 120 transactions "today" and 107 in yesterday's same window (+12 %), about 94 % approved, with amounts in the canvas's 45 000–650 000 ₫ range. Every run lands relative to its own `now`, so rerunning refreshes the chart. Approvals go mostly to ••••5540 (fixture balance 200 000 000 ₫); each run spends at most a third of ••••4417's and ••••1208's balance, so three runs never turn an intended approval into RC 51. Matching the canvas's absolute 1 950 would drain every account: the page's shape matches, its numbers do not.
8. **Also in scope (needed for the page, found in the same audit):** G2 dense 24 × 150 s throughput buckets; G4 register the configured ZPK/ZAK as ACTIVE at gateway startup when `key_store` has none. G1 (`/v1/network/switch`, MCN-802) and G3 (`transaction.updated`) stay out.

## PRs

| # | Lane | Branch | Content |
| --- | --- | --- | --- |
| 1 | PLAT/contracts | `chore/MCN-002-seed-fixtures` | `cards.json`: +`tok_limit` (1208), +`tok_second` (5540); 7 terminals/merchants with Vietnamese names and MCCs. This plan. Risk R-12. |
| 2 | GW | `feat/MCN-002-gw-merchant-terminal` | Terminal → merchant resolution (ruling 5); regenerate `cards_gen.go`; chaos terminal fix; G2 buckets; G4 key bootstrap |
| 3 | GW | `feat/MCN-002-acquirer-seed` | `cmd/seed` + `make seed` (rulings 2, 3, 6, 7) |
| 4 | WEB | `fix/MCN-002-web-fixture-cards` | POS `CardPicker` +2 cards; `scenario-handlers.ts` rows use only outcomes the fixtures can produce |

## Interfaces

- `store.TerminalRepository.Merchant(ctx, tid string) (store.Merchant{MID, Name string}, error)`; `store.ErrUnknownTerminal`.
- `store.TerminalRepository.UpsertFromFixture(ctx, []store.FixtureTerminal) error` (merchant upsert first, then terminal).
- `purchase.Service` / `advtxn.Service` gain a `MerchantResolver` port (the interface above); `purchase.ErrUnknownTerminal` maps to 422 `unknown-terminal` in `internal/api`'s single error-mapping place.
- `store.KeyStoreRepository.EnsureActive(ctx, keyType, kcv, keyUnderLMKHex string) (inserted bool, err error)`: inserts an ACTIVE row only when none is active for that type.
- `cmd/seed`: flags `-gateway-url`, `-issuer-admin-url`, `-database-url`, `-fixture`, `-now` (test clock); stages `upsertMerchants → setLimits → purchase(plan) → cancel(some) → awaitSettled → backdate(plan)`.
- `seedplan.Build(now time.Time, rng *rand.Rand) []seedplan.Txn{CardToken, TerminalID, Amount, At time.Time, Cancel bool}` is pure and deterministic under a fixed seed, so it can be tested without a stack.

## Tests (failing first)

PR 2
- `TestTerminalRepository_merchantForSeededTerminal__MCN_002` (Testcontainers): `00000042` → `GOCPHO000000001`.
- `TestTerminalRepository_unknownTerminal__MCN_002` → `ErrUnknownTerminal`.
- `TestPurchase_usesTerminalsMerchantInDE42AndTranLog__MCN_002` (fake resolver): DE 42 and the persisted `mid` equal the resolved MID, and the WS event's `merchantName` equals the resolved name.
- `TestPostPurchase_unknownTerminalIs422__MCN_002`.
- `TestTranLogRepository_overview_throughputIs24DenseBuckets__MCN_306`: 24 ascending samples 150 s apart, zeros for empty buckets, `tps = count/150`.
- `TestKeyStore_ensureActiveInsertsOnlyWhenNoneActive__MCN_002`.

PR 3
- `TestSeedPlan_todayIsAbout12PercentOverYesterdaysWindow__MCN_002`.
- `TestSeedPlan_everyLastHourBucketHasTransactions__MCN_002`.
- `TestSeedPlan_outcomeMixMatchesCardStates__MCN_002`: every planned 9021 purchase exceeds its balance, every planned 1208 decline exceeds 500 000, and no approval is planned on 3310 or 7765.
- `TestSeedPlan_spendStaysWithinEachApprovingCardsBalance__MCN_002`: per-run approved spend ≤ ⅓ of each approving card's fixture balance.
- `TestBackdate_shiftsTranLogAndHistoryTogether__MCN_002` (Testcontainers): latency unchanged, `created_at` moved.
- Manual verification: `make down -v && make up && make seed`, then `curl` the Overview calls and screenshot the page in Easy and Expert mode against the canvas.

PR 4
- `CardPicker` renders 6 cards; `scenario-handlers` rows satisfy the same card-state rules as `TestSeedPlan_outcomeMixMatchesCardStates`.

## Acceptance

| ID | Criterion | Verified by |
| --- | --- | --- |
| S1 | `make seed` on a fresh stack fills every Overview card except "Chế độ duyệt thay" (G1) | manual run + screenshots |
| S2 | ≥ 5 merchants with Vietnamese names appear in the feed | manual run |
| S3 | Decline reasons show RC 51, 61, 62, 54 | manual run |
| S4 | ≥ 1 REVERSED row | manual run |
| S5 | Day-over-day delta is +10 % to +14 % | `curl /v1/metrics/overview` |
| S6 | All 24 chart buckets are non-zero | `curl /v1/metrics/overview` |
| S7 | An ACTIVE ZPK exists without any rotation | `curl /v1/keys/acquirer` |
| S8 | Three consecutive `make seed` runs keep the same outcome mix (merchants upserted, transactions added per run) | `TestSeedPlan_spendStaysWithinEachApprovingCardsBalance` + manual run |
