package io.mcn.issuer.adapter.logging;

import java.util.regex.Matcher;
import java.util.regex.Pattern;

/** Keeps the first 6 and last 4 digits of standalone 13–19 digit runs (PCI DSS 3.4). */
public final class PanMasker {
  private static final Pattern PAN_LIKE = Pattern.compile("\\b\\d{13,19}\\b");

  private PanMasker() {}

  public static String mask(String text) {
    Matcher m = PAN_LIKE.matcher(text);
    StringBuilder out = new StringBuilder();
    while (m.find()) {
      String pan = m.group();
      m.appendReplacement(
          out,
          pan.substring(0, 6) + "*".repeat(pan.length() - 10) + pan.substring(pan.length() - 4));
    }
    m.appendTail(out);
    return out.toString();
  }
}
