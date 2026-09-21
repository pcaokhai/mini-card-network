"""Executable reference of the MCN-87A packing rules (docs/03 §2, contracts/iso8583/packager-spec.yaml).

Used only to validate golden vectors; production codecs are generated separately in each service.
"""
from __future__ import annotations

import re
from dataclasses import dataclass
from pathlib import Path

import yaml

SPEC_PATH = Path(__file__).resolve().parent.parent / "iso8583" / "packager-spec.yaml"
_HEX = re.compile(r"^[0-9A-F]*$")


class CodecError(Exception):
    def __init__(self, code: str, detail: str):
        super().__init__(f"{code}: {detail}")
        self.code = code


@dataclass(frozen=True)
class FieldSpec:
    number: int
    type: str
    length: int
    prefix: str | None


def load_spec(path: Path = SPEC_PATH) -> dict[int, FieldSpec]:
    raw = yaml.safe_load(path.read_text())
    return {int(n): FieldSpec(int(n), f["type"], int(f["length"]), f.get("prefix")) for n, f in raw["fields"].items()}


def _bitmap_hex(numbers: set[int], page: int) -> str:
    value = 0
    for i in range(64):
        if page * 64 + i + 1 in numbers:
            value |= 1 << (63 - i)
    return format(value, "016X")


def _data_len(spec: FieldSpec, value: str) -> int:
    return len(value) // 2 if spec.type == "b" else len(value)


def _check_value(spec: FieldSpec, value: str) -> None:
    if spec.type == "b" and (len(value) % 2 or not _HEX.match(value)):
        raise CodecError("INVALID_VALUE", f"DE {spec.number} must be uppercase hex")
    if spec.type == "n" and not value.isdigit():
        raise CodecError("INVALID_VALUE", f"DE {spec.number} must be numeric")
    size = _data_len(spec, value)
    if spec.prefix is None and size != spec.length:
        raise CodecError("INVALID_LENGTH", f"DE {spec.number} fixed length {spec.length}, got {size}")
    if spec.prefix is not None and size > spec.length:
        raise CodecError("INVALID_LENGTH", f"DE {spec.number} max {spec.length}, got {size}")


def pack(mti: str, fields: dict[int, str], spec: dict[int, FieldSpec] | None = None) -> str:
    spec = spec or load_spec()
    if not re.fullmatch(r"\d{4}", mti):
        raise CodecError("INVALID_MTI", mti)
    numbers = set(fields)
    if 1 in numbers:
        raise CodecError("UNKNOWN_FIELD", "DE 1 is derived, do not set it")
    secondary = any(n > 64 for n in numbers)
    bits = numbers | ({1} if secondary else set())
    out = [mti, _bitmap_hex(bits, 0)]
    if secondary:
        out.append(_bitmap_hex(bits, 1))
    for n in sorted(numbers):
        if n not in spec:
            raise CodecError("UNKNOWN_FIELD", f"DE {n}")
        s, v = spec[n], fields[n]
        _check_value(s, v)
        if s.prefix == "LL":
            out.append(f"{_data_len(s, v):02d}")
        elif s.prefix == "LLL":
            out.append(f"{_data_len(s, v):03d}")
        out.append(v)
    return "".join(out)


def unpack(packed: str, spec: dict[int, FieldSpec] | None = None) -> tuple[str, dict[int, str]]:
    spec = spec or load_spec()
    pos = 0

    def take(count: int, what: str) -> str:
        nonlocal pos
        if pos + count > len(packed):
            raise CodecError("TRUNCATED", f"{what} needs {count} chars at {pos}")
        chunk = packed[pos:pos + count]
        pos += count
        return chunk

    mti = take(4, "MTI")
    if not mti.isdigit():
        raise CodecError("INVALID_MTI", mti)
    primary = take(16, "primary bitmap")
    if not _HEX.match(primary):
        raise CodecError("INVALID_BITMAP", primary)
    bits = int(primary, 16)
    numbers = [i + 1 for i in range(64) if bits & (1 << (63 - i))]
    if 1 in numbers:
        secondary = take(16, "secondary bitmap")
        if not _HEX.match(secondary):
            raise CodecError("INVALID_BITMAP", secondary)
        sbits = int(secondary, 16)
        numbers += [65 + i for i in range(64) if sbits & (1 << (63 - i))]
    fields: dict[int, str] = {}
    for n in numbers:
        if n == 1:
            continue
        if n not in spec:
            raise CodecError("UNKNOWN_FIELD", f"DE {n}")
        s = spec[n]
        if s.prefix:
            digits = 2 if s.prefix == "LL" else 3
            raw_len = take(digits, f"DE {n} length")
            if not raw_len.isdigit():
                raise CodecError("INVALID_LENGTH", f"DE {n} prefix '{raw_len}'")
            size = int(raw_len)
            if size > s.length:
                raise CodecError("INVALID_LENGTH", f"DE {n} max {s.length}, got {size}")
        else:
            size = s.length
        value = take(size * 2 if s.type == "b" else size, f"DE {n}")
        _check_value(s, value)
        fields[n] = value
    if pos != len(packed):
        raise CodecError("TRAILING_DATA", f"{len(packed) - pos} chars after last field")
    return mti, fields
