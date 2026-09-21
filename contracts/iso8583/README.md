# contracts/iso8583

- `packager-spec.yaml` — MCN-87A field definitions. Both codecs are generated from it; never edit generated packagers.
- `../scripts/mcn87a.py` — the executable reference of this spec (MCN-003). It only validates vectors
  in `make contracts`; each service still generates and owns its own production codec from
  `packager-spec.yaml` directly.
- `vectors/*.json` — golden vectors `{ name, spec, mti, fields{DE: value}, packed }`. `packed` excludes the 2-byte length header.
  Both codecs MUST pack `fields` to exactly `packed` and unpack `packed` to exactly `fields` (`make contracts`).
- `vectors/invalid/*.json` — negative vectors `{ name, spec, packed, expectedError }`. `expectedError` is one of:
  `TRUNCATED`, `UNKNOWN_FIELD`, `INVALID_LENGTH`, `INVALID_VALUE`, `INVALID_BITMAP`, `INVALID_MTI`, `TRAILING_DATA`.
- Adding a message type or field: add/extend the spec, add at least one positive vector and one negative vector
  (`vectors/invalid/*.json` with `{ packed, expectedError }`), then implement.
- `vectors/mac/` (added by MCN-502) holds MAC vectors with lab-only ZAK keys.
- Values use test data only: BIN 970436, acquirer 970499, TID 00000042.
