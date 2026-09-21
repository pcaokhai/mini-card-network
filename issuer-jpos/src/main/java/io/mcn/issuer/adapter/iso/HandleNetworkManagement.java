package io.mcn.issuer.adapter.iso;

import io.mcn.issuer.adapter.persistence.AcquirerLinkRepository;
import org.jpos.iso.ISOException;
import org.jpos.iso.ISOMsg;

/**
 * Sign-on/sign-off/echo per docs/03 §7.1. Financial (01xx/02xx) requests get RC 91 unconditionally
 * this sprint: no authorization participant exists yet (MCN-302 replaces this).
 */
public final class HandleNetworkManagement {
  private static final String LAB_ACQUIRER_ID = "970499"; // docs/03 §3

  private final AcquirerLinkRepository links;

  public HandleNetworkManagement(AcquirerLinkRepository links) {
    this.links = links;
  }

  public ISOMsg handle(ISOMsg request) throws ISOException {
    String mtiClass = request.getMTI().substring(0, 2);
    ISOMsg response = (ISOMsg) request.clone();
    response.setResponseMTI();

    if ("08".equals(mtiClass)) {
      return handleNetworkManagement(request, response);
    }
    if ("01".equals(mtiClass) || "02".equals(mtiClass)) {
      response.set(39, "91");
      return response;
    }
    response.set(39, "30");
    return response;
  }

  private ISOMsg handleNetworkManagement(ISOMsg request, ISOMsg response) throws ISOException {
    String de70 = request.getString(70);
    switch (de70) {
      case "001" -> links.upsertStatus(LAB_ACQUIRER_ID, "SIGNED_ON");
      case "002" -> links.upsertStatus(LAB_ACQUIRER_ID, "DISCONNECTED");
      case "301" -> {
        // echo: no state change, just RC 00
      }
      default -> {
        response.set(39, "30"); // unsupported network management code
        return response;
      }
    }
    response.set(39, "00");
    return response;
  }
}
