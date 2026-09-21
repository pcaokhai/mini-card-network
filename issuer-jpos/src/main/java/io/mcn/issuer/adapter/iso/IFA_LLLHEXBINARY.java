package io.mcn.issuer.adapter.iso;

import org.jpos.iso.AsciiHexInterpreter;
import org.jpos.iso.AsciiPrefixer;
import org.jpos.iso.ISOBinaryFieldPackager;

/**
 * LLL-prefixed binary field packager for ASCII wire formats, where the payload is carried as
 * literal hex-ASCII text (e.g. ICC/EMV data). jPOS ships no built-in equivalent: its LLL-prefixed
 * binary classes (IFA_LLLBINARY, IFB_LLLHEX) all use LiteralBinaryInterpreter, which packs raw
 * bytes instead of hex text, corrupting an ASCII wire message. Confirmed via schema probe
 * (MCN-102) against jPOS 3.0.1.
 */
public class IFA_LLLHEXBINARY extends ISOBinaryFieldPackager {
  public IFA_LLLHEXBINARY() {
    super(AsciiHexInterpreter.INSTANCE, AsciiPrefixer.LLL);
  }
}
