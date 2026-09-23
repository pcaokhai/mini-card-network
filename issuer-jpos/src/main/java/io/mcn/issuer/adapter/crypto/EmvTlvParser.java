package io.mcn.issuer.adapter.crypto;

import java.util.HexFormat;
import java.util.LinkedHashMap;
import java.util.Map;

/**
 * Decodes DE 55's simple-TLV EMV data into a tag-keyed map, per docs/03 §11. Recognizes tags with a
 * one- or two-byte tag id (a tag byte with the low 5 bits all set indicates a multi-byte tag,
 * standard BER-TLV convention) and a single-byte length (this lab's tag set never carries a value
 * longer than 127 bytes, so no multi-byte length encoding is needed). Any well-formed tag is parsed
 * into the map even without a named accessor (docs/plans/MCN-602.md Ruling 2); only a structural
 * violation (truncated tag/length, or a declared length exceeding the remaining bytes) throws.
 */
public final class EmvTlvParser {
  private EmvTlvParser() {}

  public static Map<String, byte[]> parse(byte[] tlvBytes) {
    Map<String, byte[]> tags = new LinkedHashMap<>();
    int pos = 0;
    while (pos < tlvBytes.length) {
      if (pos >= tlvBytes.length) {
        throw new MalformedTlvException("truncated tag at offset " + pos);
      }
      int tagStart = pos;
      int firstByte = tlvBytes[pos] & 0xFF;
      pos++;
      if ((firstByte & 0x1F) == 0x1F) {
        if (pos >= tlvBytes.length) {
          throw new MalformedTlvException("truncated multi-byte tag at offset " + tagStart);
        }
        pos++;
      }
      String tag = HexFormat.of().formatHex(tlvBytes, tagStart, pos).toUpperCase();

      if (pos >= tlvBytes.length) {
        throw new MalformedTlvException("truncated length at offset " + pos);
      }
      int length = tlvBytes[pos] & 0xFF;
      pos++;

      if (pos + length > tlvBytes.length) {
        throw new MalformedTlvException("declared length exceeds remaining bytes for tag " + tag);
      }
      byte[] value = new byte[length];
      System.arraycopy(tlvBytes, pos, value, 0, length);
      pos += length;

      tags.put(tag, value);
    }
    return Map.copyOf(tags);
  }

  public static final class MalformedTlvException extends RuntimeException {
    public MalformedTlvException(String message) {
      super(message);
    }
  }
}
