package io.mcn.issuer.adapter.http;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.Mockito.RETURNS_SELF;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

import io.javalin.http.BadRequestResponse;
import io.javalin.http.Context;
import io.javalin.http.HttpResponseException;
import io.javalin.http.MethodNotAllowedResponse;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;

class ApiProblemsTest {

  private static Context ctx() {
    Context ctx = mock(Context.class, RETURNS_SELF);
    when(ctx.path()).thenReturn("/v1/cards");
    when(ctx.<String>attribute(anyString())).thenReturn("4bf92f3577b34da6a3ce929d0e0e4736");
    return ctx;
  }

  private static String bodyOf(Context ctx) {
    ArgumentCaptor<String> body = ArgumentCaptor.forClass(String.class);
    verify(ctx).result(body.capture());
    return body.getValue();
  }

  @Test
  @DisplayName("CARDS-G9: Javalin's own 405 keeps its status instead of becoming a 500")
  void should_keepTheStatus_when_javalinThrowsA405() {
    Context ctx = ctx();

    ApiProblems.internal(new MethodNotAllowedResponse(), ctx);

    verify(ctx).status(405);
    assertThat(bodyOf(ctx)).contains("https://mcn.local/problems/validation-error");
  }

  @Test
  @DisplayName("CARDS-G9: Javalin's own 400 maps to validation-error")
  void should_mapToValidationError_when_javalinThrowsA400() {
    Context ctx = ctx();

    ApiProblems.internal(new BadRequestResponse(), ctx);

    verify(ctx).status(400);
    assertThat(bodyOf(ctx)).contains("https://mcn.local/problems/validation-error");
  }

  @Test
  @DisplayName("CARDS-G9: an HttpResponseException with a 5xx status is internal")
  void should_beInternal_when_theHttpResponseExceptionIs5xx() {
    Context ctx = ctx();

    ApiProblems.internal(new HttpResponseException(503, "boom"), ctx);

    verify(ctx).status(500);
    assertThat(bodyOf(ctx)).contains("/problems/internal").doesNotContain("boom");
  }
}
