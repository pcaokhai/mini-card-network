# MCN-104 (canvas pass) — Message Lab matches the design canvas

Story: MCN-104 (docs/06 §E1). Canvas: `project/MessageLab.dc.html`. Motion: docs/02 §7.9 "bitmap diagonal flip on message change".

## Interfaces

- `src/shared/api/lab-client.ts`: `useSampleMessages()` (GET `/v1/lab/messages/samples`) and `useDecodedMessage(raw)` (POST `/v1/lab/messages/decode` as a cached query, previous data kept while the next decodes). Replaces the mutation-based `useDecodeMessage`.
- `src/components/lab/lab-model.ts` (pure): `rawSegments(decoded)`, `isBitOn(decoded, n)`, `bitmapRows(decoded, page)`, `fieldRows(decoded, expert, copy)`, `describeSelection(decoded, key, copy)`, `defaultSelection(decoded)`; `Copy = { t, has }` wraps the `lab` translator.
- `src/components/lab/`: `RawMessage`, `BitmapPanel`, `DetailPanel`, `FieldList`, `lab.css` (canvas values, tones through `data-kind` / `data-on`).
- `MessageLabScreen` owns sample index, bitmap page and selection (component state; `src/shared/state/message-lab.ts` removed because it duplicated server data).
- `src/mocks/pages/lab.ts`: `labHandlers` + `LAB_SAMPLES`, the canvas's four messages.

## Tests

- `lab-model.test.ts`: bitmap segments merge (AC1), bit reading and row hex/bin (AC2), MTI, DE 22, DE 55 and DE 90 breakdowns (AC3), absent and unlisted fields, Easy/Expert row names (AC4).
- `MessageLabScreen.test.tsx`: segment/cell/row selection syncs all views (AC1), 64 cells with hex/bin, secondary page disabled without bit 1 (AC2), MTI breakdown (AC3), Easy/Expert columns and note (AC4), grid remounts with a diagonal delay on message change (AC5).
- `lab-client.test.tsx`: samples and decode through the page handlers.
- Stories `Lab/MessageLabScreen` Easy and Expert with axe.

## Rulings

- R1 — No paste box. The canvas has none; the screen dissects the gateway's sample messages. The decode endpoint is still what renders each sample.
- R2 — Field names and explanations come from the `lab.fields` glossary in messages/*.json (canvas copy), keyed by DE; technical names and formats from `FIELD_SPECS`. The gateway's own names are the fallback for a DE the glossary lacks. DE 15, 43 and 128 (present in the real samples, not in the canvas) got glossary entries in the canvas's voice.
- R3 — Off bits use #6B6D75 instead of the canvas's #8A8C94, which is 3.2:1 on #FAF9F6 and fails the 4.5:1 rule (axe fails the stories).
- R4 — The "Phụ 65–128" tab is always shown as in the canvas but disabled when bit 1 is off, which satisfies AC2 ("appears when bit 1 is set") without changing the layout.
- R5 — Below the canvas's 1440px frame the expert list's fixed columns (376px) crush the name column; a container query narrows the format and value columns under 560px.
- R6 — Sample tab labels stay on one line; the canvas lets "0200 Mua hàng" wrap to two lines inside a 32px button, which reads as an accident.
- R7 — The MTI origin, class, function and version come from small digit glossaries rather than the canvas's fixed "acquirer" text, so a 0810 or issuer-originated message is not mislabelled.
- R8 — JetBrains Mono 500 is imported by the screen: globals.css loads only 400 and the canvas sets bit numbers, hex and values in 500.

## AC table

| AC | Covered by |
| --- | --- |
| MCN-104-AC1 | lab-model "merges both bitmaps…", screen "selecting a raw segment…", "selecting a bitmap cell or a list row…", lab-client test |
| MCN-104-AC2 | lab-model "reads bits…", "gives each bitmap row…", screen "shows hex and binary…" |
| MCN-104-AC3 | lab-model MTI / DE 22 / DE 55 / DE 90 tests, screen "breaks down the MTI's four digits" |
| MCN-104-AC4 | lab-model "names rows…", screen Easy and Expert tests |
| MCN-104-AC5 | screen "re-flips the bitmap cells…"; reduced motion swaps to `lab-fade` in lab.css |
