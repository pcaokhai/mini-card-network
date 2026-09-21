"""Unit tests for the reference codec (run: python -m pytest contracts/scripts)."""
import pytest

from mcn87a import CodecError, pack, unpack


def test_secondary_bitmap_is_derived_from_fields_above_64__MCN_003_AC3():
    packed = pack("0800", {7: "0921073300", 11: "000200", 70: "301"})
    assert packed[4:36] == "8220000000000000" + "0400000000000000"


def test_llvar_prefix_counts_characters_and_lllvar_binary_counts_bytes__MCN_003_AC3():
    packed = pack("0200", {2: "9704360000004417", 55: "9F2701"})
    assert "169704360000004417" in packed
    assert packed.endswith("0039F2701")


def test_round_trip_preserves_every_field__MCN_003_AC3():
    fields = {3: "000000", 4: "000000250000", 41: "00000042"}
    assert unpack(pack("0200", fields)) == ("0200", fields)


@pytest.mark.parametrize(
    "fields, code",
    [({4: "25000"}, "INVALID_LENGTH"), ({52: "zz"}, "INVALID_VALUE"), ({63: "X"}, "UNKNOWN_FIELD"), ({1: "0" * 16}, "UNKNOWN_FIELD")],
)
def test_pack_rejects_invalid_fields_with_typed_errors__MCN_003_AC3(fields, code):
    with pytest.raises(CodecError) as err:
        pack("0200", fields)
    assert err.value.code == code
