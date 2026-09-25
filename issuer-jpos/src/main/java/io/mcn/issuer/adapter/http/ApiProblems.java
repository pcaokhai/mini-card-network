package io.mcn.issuer.adapter.http;

import com.fasterxml.jackson.databind.ObjectMapper;
import io.javalin.http.Context;
import io.javalin.http.HttpResponseException;
import io.mcn.issuer.adapter.logging.PanMasker;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import java.util.regex.Matcher;
import java.util.regex.Pattern;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * The issuer Admin API's one mapping to RFC 9457 problems (docs/04 §3): full-URI {@code type},
 * {@code instance}, and {@code traceId} = the request's W3C trace id, which every response also
 * echoes as {@code X-Trace-Id}.
 */
final class ApiProblems {
  private static final Logger LOG = LoggerFactory.getLogger(ApiProblems.class);
  private static final String PROBLEM_JSON = "application/problem+json";
  private static final String TYPE_BASE = "https://mcn.local/problems/";
  private static final String TRACE_ID_ATTRIBUTE = "mcn.traceId";
  private static final Pattern TRACEPARENT =
      Pattern.compile("^[0-9a-f]{2}-([0-9a-f]{32})-[0-9a-f]{16}-[0-9a-f]{2}$");
  private static final String INVALID_TRACE_ID = "0".repeat(32);
  private static final ObjectMapper MAPPER = new ObjectMapper();

  record FieldError(String field, String message) {}

  private ApiProblems() {}

  /** Before-handler: adopts the caller's trace id from {@code traceparent}, else starts one. */
  static void assignTraceId(Context ctx) {
    String traceId = traceIdFrom(ctx.header("traceparent"));
    ctx.attribute(TRACE_ID_ATTRIBUTE, traceId);
    ctx.header("X-Trace-Id", traceId);
  }

  private static String traceIdFrom(String traceparent) {
    Matcher m = TRACEPARENT.matcher(traceparent == null ? "" : traceparent.trim());
    if (m.matches() && !m.group(1).equals(INVALID_TRACE_ID)) return m.group(1);
    return UUID.randomUUID().toString().replace("-", "");
  }

  private static String traceId(Context ctx) {
    String traceId = ctx.attribute(TRACE_ID_ATTRIBUTE);
    if (traceId == null) {
      assignTraceId(ctx);
      traceId = ctx.attribute(TRACE_ID_ATTRIBUTE);
    }
    return traceId;
  }

  static void send(Context ctx, int status, String slug, String title, String detail) {
    send(ctx, status, slug, title, detail, List.of());
  }

  static void validation(Context ctx, List<FieldError> errors) {
    send(
        ctx,
        400,
        "validation-error",
        "Request invalid",
        "One or more fields are invalid; see errors[].",
        errors);
  }

  /**
   * Exception handler. Javalin's own client errors (404 no such route, 400, 405…) arrive here as
   * {@link HttpResponseException} and keep their status. Anything else is unexpected: logged
   * against the trace id, answered 500 with only the trace.
   */
  static void internal(Exception e, Context ctx) {
    if (e instanceof HttpResponseException http && http.getStatus() < 500) {
      int status = http.getStatus();
      String slug = status == 404 ? "not-found" : "validation-error";
      send(ctx, status, slug, "Request not served", "No " + ctx.method() + " at " + ctx.path());
      return;
    }
    LOG.error(
        "admin api request failed trace_id={} path={}",
        traceId(ctx),
        PanMasker.mask(ctx.path()),
        e);
    send(ctx, 500, "internal", "Internal error", null);
  }

  /**
   * 404 error handler: Javalin answers an unmatched route with its own plain text that echoes the
   * raw path. A 404 a handler already wrote as a problem is left alone.
   */
  static void unmatchedRoute(Context ctx) {
    String contentType = ctx.contentType();
    if (contentType != null && contentType.startsWith(PROBLEM_JSON)) return;
    send(ctx, 404, "not-found", "Not found", "No " + ctx.method() + " at " + ctx.path());
  }

  private static void send(
      Context ctx, int status, String slug, String title, String detail, List<FieldError> errors) {
    Map<String, Object> body = new LinkedHashMap<>();
    body.put("type", TYPE_BASE + slug);
    body.put("title", title);
    body.put("status", status);
    // The path and anything built from it are caller input: mask a PAN typed into the URL.
    if (detail != null) body.put("detail", PanMasker.mask(detail));
    body.put("instance", PanMasker.mask(ctx.path()));
    body.put("traceId", traceId(ctx));
    if (!errors.isEmpty()) body.put("errors", errors);
    try {
      // result() then contentType(): ctx.json() would reset the type to application/json.
      ctx.status(status).result(MAPPER.writeValueAsString(body));
    } catch (Exception e) {
      throw new IllegalStateException("serialize problem failed", e);
    }
    ctx.contentType(PROBLEM_JSON);
  }
}
