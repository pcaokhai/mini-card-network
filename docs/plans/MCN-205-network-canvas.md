# MCN-205 / MCN-804 — Network screen matches the design canvas

Branch `fix/MCN-205-network-canvas`. Lane WEB only. Canvas: `Network.dc.html` (markup, `renderVals()`, keyframes).
Stories: MCN-205 (topology, links table, event timeline, header pill), MCN-804 (switch node, breaker pills, STIP counters, SAF list, "simulate issuer down").

## Interfaces

- `src/components/network/network-model.ts` (pure, no React)
  - `linkTone(status: LinkStatus): Tone` — SIGNED_ON ok, CONNECTED warn, DOWN bad, DISCONNECTED info.
  - `echoAge(link: Link, now: number): EchoAge` — `{ key: "noReply" | "none" | "justNow" | "seconds" | "minutes" | "hours"; n: number }`.
  - `endpointName(id: string): string` — `gateway-a` → `Gateway A`, `issuer` → `Issuer`; `switch` is translated by the caller.
  - `buildTopology(input: { links; switchStatus; terminals }): { nodes: TopologyNode[]; segments: SegmentState[] }` — four nodes POS · acquirer · switch · issuer with tone + instance names; three segments `up | down`.
  - `todaysEvents(events: NetworkEvent[], now: number): NetworkEvent[]` — newest first, local today only, max 30.
- `src/components/network/` — `NetworkTopology`, `LinksPanel`, `EventLog`, `BreakerCard`, `SafCard`, `network.css`.
- `src/shared/api/network-client.ts` — adds `useTerminals()` and `useNetworkLiveUpdates()` (WS `link.status`, `network.event`, `saf.changed`, `switch.status` invalidate the matching query).
- `src/mocks/pages/network.ts` — stateful canvas scenario: four links, switch, SAF, events, terminals, echo, and the `ISSUER_DOWN` chaos toggle.

## Tests

- `network-model.test.ts`: tone per status; echo age buckets and "no reply" after a failed echo; topology marks only the switch→issuer segment down when issuer links are DOWN; direct gateway→issuer link (real stack) drives both acquirer segments; switch node is `unavailable` without a switch status; events sorted newest first, today only, capped at 30.
- `NetworkScreen.test.tsx` (MSW with `networkHandlers`):
  - `MCN-205-AC1` topology shows four nodes and flowing segments in Easy copy.
  - `MCN-205-AC2` links table rows, latency, echo age, "Kiểm tra ngay" echoes and shows "Vừa xong".
  - `MCN-205-AC3` event log newest first, and a WS `network.event` refetches it.
  - `MCN-804-AC1` breaker pills, STIP limit/count and SAF empty state; Expert copy (`SIGNED_ON`, `p99`, `SAF queue`, `depth 0 · dead 0 · oldest —`).
  - `MCN-804-AC2` "Mô phỏng ngân hàng phát hành sập" sets ISSUER_DOWN; the screen shows DOWN rows, OPEN circuit, 37 STIP, three SAF items, the red dashed segment; "Khôi phục" restores and logs the recovery.
  - Real-mode gap: `/v1/network/switch` and `/v1/terminals` 404 → designed unavailable states, no error.
- `NetworkScreen.stories.tsx`: Easy, Expert, IssuerDown, BackendGaps (axe via addon-a11y).

## Rulings

1. **Issuer-down toggle drives the real chaos scenario** (MCN-804-AC2): PUT `/v1/chaos/scenarios/ISSUER_DOWN` via `useSetChaosScenario`; the label follows the scenario's `enabled`. Not exercised against the real stack (it cuts the shared issuer link); verified under dev:mock only.
2. **Only the echo button per row.** The canvas has no sign-on / sign-off buttons; they are dropped from the row (the gateway re-signs-on by itself, MCN-202). `useLinkAction` keeps supporting them.
3. **Addresses have no ports.** `Link` carries `from`/`to` ids, not host:port, so `gateway-a → switch:9000` renders as `gateway-a → switch`. Expert sub-address stays the canvas's `ISO 8583 · 2-byte header` (true for every link, docs/03).
4. **POS node** counts `/v1/terminals` as "máy đã đăng ký" / "terminals registered": the API has no online flag, so the canvas's "đang trực tuyến" / "TLS" would be invented. Without the endpoint (real gateway 404) the node shows an unavailable state.
5. **Switch and STIP on the real stack**: `/v1/network/switch` is 404 (MCN-802 not built). The switch node, breaker pills and STIP tiles show a neutral "chưa có dữ liệu" state; the acquirer→issuer link then colours both segments, since the real gateway talks to the issuer directly.
6. **Motion on transform/opacity only** (docs/02 §7.9): the flowing link translates a striped layer instead of animating `background-position`; tone changes swap colours without the canvas's background-color transitions. Node shake, dot beat and row fade-up are kept.
7. **Event texts come from the gateway**, which writes them in English today; the screen renders them as given (no client-side translation table).
8. **Header pill** (shared `Header.tsx`, not this lane's file) still reads "Mất kết nối"/"Đã kết nối" from links; the canvas's amber "Đang duyệt thay…" pill during STIP needs a Header change — left for a follow-up.
9. **Layout below 1400px**: the canvas's fixed 400px rail leaves the links table no room at 1280px, so the two columns stack there (same breakpoint as Overview).
10. **Breaker pill ink**: the canvas's inactive pill (#6B6D75 on #F0EEE8) is 4.4:1 and fails the axe story test; it uses the muted ink (#5E6068) instead.
11. **Manual echo on the real gateway** answers `ok: true` but does not move `lastEchoAt`, so the row keeps its age instead of the canvas's "Vừa xong". Gateway follow-up; the mock moves it.
12. **Mock chaos**: `GET/PUT /v1/chaos/scenarios` are answered in `mocks/pages/network.ts` so the toggle works under dev:mock; if the Chaos Lab page later mocks them in `chaos.ts`, move them there.

## AC table

| AC | Test |
| --- | --- |
| MCN-205-AC1 topology, colour by status, flowing / red dashed | model topology tests, `MCN-205-AC1`, `MCN-804-AC2` |
| MCN-205-AC2 links table + "Check now" | `MCN-205-AC2` |
| MCN-205-AC3 live event timeline newest first | `MCN-205-AC3`, model events test |
| MCN-205-AC4 header pill | unchanged shared Header (Ruling 8) |
| MCN-804-AC1 switch, breaker pills, STIP, SAF list | `MCN-804-AC1`, `MCN-804-AC2` |
| MCN-804-AC2 simulate issuer down | `MCN-804-AC2` |
