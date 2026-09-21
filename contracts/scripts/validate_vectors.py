#!/usr/bin/env python3
"""MCN-003-AC3: every golden vector round-trips; every invalid vector fails with its expected error code."""
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from mcn87a import CodecError, load_spec, pack, unpack  # noqa: E402

ROOT = Path(__file__).resolve().parent.parent / "iso8583" / "vectors"


def main() -> int:
    spec = load_spec()
    failures = 0
    valid = sorted(ROOT.glob("*.json"))
    invalid = sorted((ROOT / "invalid").glob("*.json"))
    for path in valid:
        v = json.loads(path.read_text())
        fields = {int(k): val for k, val in v["fields"].items()}
        try:
            packed = pack(v["mti"], fields, spec)
            mti, back = unpack(v["packed"], spec)
        except CodecError as e:
            print(f"FAIL {path.name}: {e}")
            failures += 1
            continue
        if packed != v["packed"]:
            print(f"FAIL {path.name}: pack mismatch\n  expected {v['packed']}\n  actual   {packed}")
            failures += 1
        elif mti != v["mti"] or back != fields:
            print(f"FAIL {path.name}: unpack mismatch")
            failures += 1
        else:
            print(f"ok   {path.name}")
    for path in invalid:
        v = json.loads(path.read_text())
        try:
            unpack(v["packed"], spec)
            print(f"FAIL {path.name}: expected {v['expectedError']}, got success")
            failures += 1
        except CodecError as e:
            if e.code != v["expectedError"]:
                print(f"FAIL {path.name}: expected {v['expectedError']}, got {e.code}")
                failures += 1
            else:
                print(f"ok   {path.name} ({e.code})")
    if not valid:
        print("FAIL no vectors found")
        failures += 1
    print(f"{len(valid)} valid, {len(invalid)} invalid, {failures} failures")
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
