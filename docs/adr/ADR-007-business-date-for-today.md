# ADR-007: "Today" is the acquirer's current business date, rolled at cutover

- Status: Accepted
- Date: 2026-09-25
- Deciders: product owner (the user), GW, WEB
- Related: gap OVW-G7 (`docs/api/overview-page.md` §9); `docs/03-iso8583-interface-spec.md` §7.6 (cutover and business date), DE 15; MCN-306 (Overview), MCN-702 (cutover and 0500 reconciliation), MCN-705 (settlement screen)

## Context

Every per-day figure in the console ("today") uses a different clock:

| Where | Today's rule | Effect in Vietnam (UTC+7) |
| --- | --- | --- |
| Overview KPIs, delta, decline reasons (`gateway-go/internal/store/overview.go`) | `now.Truncate(24h)`: the **UTC** calendar day | The KPIs reset at 07:00 local time, in the middle of the trading day |
| `tran_log.business_date` (`gateway-go/internal/store/tranlog.go`, insert) | Postgres `CURRENT_DATE`: the database's time zone (UTC in the container) | Same as above, stored with every row |
| `Transaction.businessDate` in API responses | The service clock's date (`now.Format`) or `created_at`'s date | Can disagree with the stored column |
| Issuer | `system_state.current_business_date`, advanced by its cutover | The issuer's own day |
| Sidebar date card and settlement screen (web) | The viewer's local calendar day | Can differ from both of the above around midnight |

Card networks don't count days by calendar or UTC. They count by **business date**:
- A transaction belongs to the business date that was current when it was sent. It is carried in DE 15 (settlement date).
- The business date rolls forward only at **cutover**. The acquirer sends 0800/201 with the new DE 15 at the configured cutover time, default 23:59:59 local (docs/03 §7.6).
- Totals, the 0500 reconciliation and the clearing file are all keyed by business date. A "today" that uses any other clock can't be reconciled with the settlement screen or with the issuer.

## Options considered

1. **UTC calendar day** (today's behaviour).
   - Pros: no state, trivially testable.
   - Cons: resets at 07:00 in Vietnam; disagrees with DE 15, settlement and the issuer.
2. **Local calendar day** (Asia/Ho_Chi_Minh midnight).
   - Pros: matches a person's intuition.
   - Cons: still disagrees with the business date between 23:59:59 and 24:00, and whenever a cutover is late or manual. Two clocks remain.
3. **The acquirer's current business date, rolled at cutover.**
   - Pros: one clock for KPIs, stored rows, DE 15, totals and settlement; it is what the settlement screen and the 0500 exchange already assume.
   - Cons: the gateway needs a business-date source before MCN-702 persists cutovers.

## Decision

**Option 3.** "Today" everywhere in the console means **the acquirer's current business date**. It changes only at cutover.

1. **One source in the gateway.** Add a `BusinessCalendar` port (domain), with `Current(now) → date` and `Previous(date) → date`.
   - **Until MCN-702 lands**, a clock-based adapter derives the date from configuration:
     - Config: `CUTOVER_TIME` (default `23:59:59`) and `CUTOVER_TZ` (default `Asia/Ho_Chi_Minh`), typed and validated at startup (root CLAUDE.md §6.11).
     - Rule: the business date is the local date in `CUTOVER_TZ`, **plus one day once the local time is at or past `CUTOVER_TIME`**. This matches docs/03 §7.6: a message sent after the cutover time belongs to the next business date.
   - **When MCN-702 lands**, its cutover (scheduled or manual, 0800/201) persists the current business date, and a stored-state adapter replaces the clock adapter behind the same port. A late or manual cutover then moves "today" exactly when the network does.
2. **Stored.**
   - `tran_log.business_date` is assigned from `BusinessCalendar.Current` when the row is created (not `CURRENT_DATE`).
   - The 0200's DE 15 carries the same date as `MMDD`, and an advice reuses the DE 15 of its original.
   - A message keeps the business date it was sent with, even if it is answered after cutover (docs/03 §7.6).
3. **Overview.**
   - "Today" is `business_date = Current`.
   - The day-over-day delta compares with `business_date = Previous(Current)` over the same elapsed time since each business day opened (the previous cutover instant).
   - The response carries `businessDate` so the UI can label what it counts.
   - The 60-minute throughput chart is not a per-day figure and stays a rolling window.
4. **API.** `Transaction.businessDate` is the stored `business_date`, never a clock read.
5. **Web.** The sidebar date card and the settlement screen show the gateway's business date (`Overview.businessDate`) instead of the viewer's local day. MCN-705 Ruling R11 and gap SET-G6 are resolved by this ADR.

## Consequences

- **Easier:**
  - The Overview, the settlement screen, DE 15, the 0500 totals and the issuer all count the same day.
  - The KPIs reset once, at cutover (23:59:59 local by default), not at 07:00.
- **Harder:**
  - Tests must control both the clock and the calendar. The port makes that a fake, not a global clock.
  - Before MCN-702, a cutover that happens late in reality isn't reflected. The clock adapter assumes the scheduled time. This is documented in `docs/api/overview-page.md` until MCN-702 swaps the adapter.
- **Data:**
  - Rows written before this change keep a UTC-derived `business_date`. The lab seed (`make seed`) regenerates data, so no migration of old rows is needed. Production would need a one-off backfill from `sent_at`, which is out of scope for the lab.
- **Monitoring:**
  - A gateway and issuer business-date mismatch after cutover is a reconciliation risk. MCN-702's 0800/201 exchange is where it gets detected.
- **Reversal:**
  - Reverting to the UTC day means swapping the adapter. The column and the API field are unchanged.
- **Follow-up work:**
  - A contract field: `Overview.businessDate`, added in this PR.
  - GW implementation: the port, the clock adapter, the tran_log insert, DE 15, the overview queries and the API field.
  - WEB: labels and the sidebar card.
  - OVW-G7 and SET-G6 get marked by those PRs.
