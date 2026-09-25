package io.mcn.issuer.adapter.iso;

import io.mcn.issuer.adapter.persistence.AcquirerLinkRepository;
import org.jpos.iso.ISOException;
import org.jpos.iso.ISOMsg;

/**
 * Sign-on/sign-off/echo per docs/03 §7.1. Financial (01xx/02xx) requests are not this listener's
 * concern as of MCN-302a: {@link #handle} returns {@code null} for them so {@code
 * NetworkManagementListener} declines to consume the message, leaving it for {@code
 * AuthorizationListener} (registered after it in {@code 30_iso_server.xml}).
 */
public final class HandleNetworkManagement {
  private static final String LAB_ACQUIRER_ID = "970499"; // docs/03 §3

  private final AcquirerLinkRepository links;
  private final ReceiveKeyChange receiveKeyChange;

  public HandleNetworkManagement(AcquirerLinkRepository links) {
    this(links, null);
  }

  /** {@code receiveKeyChange} may be {@code null} if key rotation (MCN-504) is not configured. */
  public HandleNetworkManagement(AcquirerLinkRepository links, ReceiveKeyChange receiveKeyChange) {
    this.links = links;
    this.receiveKeyChange = receiveKeyChange;
  }

  public ISOMsg handle(ISOMsg request) throws ISOException {
    String mtiClass = request.getMTI().substring(0, 2);

    if ("08".equals(mtiClass)) {
      ISOMsg response = (ISOMsg) request.clone();
      response.setResponseMTI();
      return handleNetworkManagement(request, response);
    }
    // Owned by the listeners registered after this one in 30_iso_server.xml: AuthorizationListener
    // (01/02) and ReversalListener (04). Claiming 04 here answered every reversal with RC 30
    // before the reversal chain ever saw it.
    if ("01".equals(mtiClass) || "02".equals(mtiClass) || "04".equals(mtiClass)) {
      return null;
    }
    ISOMsg response = (ISOMsg) request.clone();
    response.setResponseMTI();
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
      case "161" -> {
        // key-change advice (MCN-504): the 0810 RC 00 below *is* PARTNER_CONFIRM.
        if (receiveKeyChange == null || !receiveKeyChange.receive(request)) {
          response.set(39, "96");
          return response;
        }
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
