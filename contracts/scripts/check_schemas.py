#!/usr/bin/env python3
"""MCN-003-AC4: WS and Kafka JSON Schemas are valid Draft 2020-12 and accept their fixtures."""
import json
import sys
from pathlib import Path

from jsonschema import Draft202012Validator

ROOT = Path(__file__).resolve().parent.parent


def main() -> int:
    failures = 0
    schemas = [ROOT / "ws-events.schema.json", *sorted((ROOT / "events").glob("*.schema.json"))]
    for path in schemas:
        try:
            Draft202012Validator.check_schema(json.loads(path.read_text()))
            print(f"ok   {path.relative_to(ROOT)}")
        except Exception as e:  # noqa: BLE001
            print(f"FAIL {path.relative_to(ROOT)}: {e}")
            failures += 1
    event_schema = json.loads((ROOT / "events" / "transaction-event.schema.json").read_text())
    validator = Draft202012Validator(event_schema)
    for fixture in sorted((ROOT / "fixtures" / "events").glob("*.json")):
        errors = list(validator.iter_errors(json.loads(fixture.read_text())))
        if errors:
            print(f"FAIL {fixture.relative_to(ROOT)}: {errors[0].message}")
            failures += 1
        else:
            print(f"ok   {fixture.relative_to(ROOT)}")
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
