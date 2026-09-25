# MCN-505 — Security and keys screen matches the design canvas

Story: MCN-505 (docs/06). Motion: docs/02 §7.9 ("KCV flip after rotation; PIN block rows revealing XOR").
Canvas: `Security.dc.html` (markup, `renderVals()`, keyframes `mcnFlipA`, `mcnPop`, `mcnFadeUpA/B`).
Lane WEB only. No contract, BFF or shared-UI changes.

## Interfaces

- `components/security/security-model.ts`
  - `keyRows(keys: KeyInfo[]): KeyRow[]`: canvas order (ZMK, ZPK, ZAK, TPK, TAK, CVK, PVK); `percent` clamped 0–100;
    `status` = `rotateSoon` when ACTIVE and under 20 % of lifetime left, as in the canvas; PENDING and RETIRED are neutral.
  - `rotationSteps(rotation?)`: the four contract steps, PENDING until the resource says otherwise.
  - `pinBlockRows(pin, zpkKcv)`: the five canvas rows (masked PIN field, PAN field, clear block, "under TPK", "under ZPK").
  - `isValidPin`, `ILLUSTRATION_PAN`, `ILLUSTRATION_PAN_MASKED`.
- `components/security/useZpkRotation.ts`: `{ rotation, phase: idle|running|completed|failed, startFailed, lostTrack, start, reset }`.
- `KeyTable`, `RotationPanel`, `PinBlockVisualiser`, `PciNeverDoList`: presentational, `expert: boolean` prop.
- `shared/api/security-client.ts`: `useStartRotation` seeds the rotation query with the 202 body; `useRotation`
  invalidates the key list when the rotation is COMPLETED (so the new KCV arrives and flips); `useIssuerKeys` removed.
- `mocks/pages/security.ts`: the six canvas keys; a rotation that completes one step every 1.2 s and then serves
  ZPK KCV `7D02B1`; `resetSecurityMock()`, `mockSecurityKeys()`.

## Tests

- `security-model.test.ts`: ordering, rotate-soon threshold, clamping, format-0 rows worked by hand
  (`041234FFFFFFFFFF ⊕ 0000436123456789 = 0412779EDCBA9876`), 12-digit length nibble `C`, ZPK row changes with the ZPK KCV.
- `SecurityScreen.test.tsx` (MCN-505-AC1..AC3): key rows in both modes; rotation walks four steps, the new KCV flips in,
  "Hoàn tất · làm lại" resets; refused start and unreadable rotation both show an alert and re-enable the button;
  PIN block default rows, inline error, Expert labels; PCI list in both modes.
- `SecurityScreen.stories.tsx`: Easy and Expert, axe on.

## Rulings

1. **Keys shown are the gateway's (`/v1/keys/acquirer`).** On the real stack that is ZPK and ZAK only; the canvas's
   six keys come from dev:mock. `/v1/keys/issuer` is dropped from this screen: the BFF sends it to the gateway, which
   has no such route (404), and the Issuer Admin API returns `[]`.
2. **Rotation is driven by the rotation resource, not a "Bước tiếp" button.** The canvas steps manually; the product
   polls `GET /v1/keys/acquirer/rotations/{id}` (AC1). While running the button reads "Đang xoay…" and is disabled.
   "Hoàn tất · làm lại" returns to the idle view as in the canvas; it never starts a second rotation.
3. **Only the ZPK is rotatable here**, as in the canvas ("Xoay khóa mã hóa PIN", ZPK row only), although the API
   also accepts ZAK.
4. **The Expert step details show the rotation's `newKcv`**, or `······` until the resource reports one. The canvas
   hard-codes `7D02B1`.
5. **The PIN block uses a made-up card, `9704 36•• •••• 7890`**, labelled "Số thẻ minh họa". The previous
   visualiser (and the canvas) used the fixture card `…4417` in client code. The new number is not in
   `contracts/fixtures/cards.json` and is not Luhn-valid.
6. **TPK note reads "Dùng chung cho các máy POS"**: the canvas's "42 máy POS" is a number the product does not have.
7. **KCV flip, step pop and PIN rows use transform/opacity only.** The canvas's width transition on the lifetime bar
   becomes `scaleX`.
8. **Below 1400 px the PCI card moves under the other two.** The canvas is 1440 px wide; at 1280 px the three fixed
   columns would leave the PIN rows about 250 px, which is too narrow.

## AC

| AC | Evidence |
| --- | --- |
| AC1 key table, lifetime bar, near-expiry warning, rotation stepper from the resource, KCV flip | `SecurityScreen.test.tsx` AC1 tests, `security-model.test.ts` |
| AC2 PIN block in the browser, labelled illustration, 4–12 digits with inline error | AC2 tests |
| AC3 PCI "never do" list, Easy/Expert | AC3 test |
