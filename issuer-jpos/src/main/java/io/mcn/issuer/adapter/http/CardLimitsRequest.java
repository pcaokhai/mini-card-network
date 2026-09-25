package io.mcn.issuer.adapter.http;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.mcn.issuer.adapter.http.ApiProblems.FieldError;
import java.util.ArrayList;
import java.util.List;

/**
 * A validated {@code PUT /v1/cards/{cardRef}/limits} body (CARDS-G7): amounts are positive integer
 * minor units in the card's currency, {@code dailyCount} is null or positive, and the
 * per-transaction ceiling never exceeds the daily one. Positive, not ≥ 0: {@code card_limit} checks
 * {@code max_amount > 0} and {@code max_count > 0} (a zero ceiling is a block, which has its own
 * endpoint).
 */
record CardLimitsRequest(long perTransactionAmount, long dailyAmount, Integer dailyCount) {
  private static final ObjectMapper MAPPER = new ObjectMapper();

  sealed interface Parsed permits Valid, Invalid {}

  record Valid(CardLimitsRequest request) implements Parsed {}

  record Invalid(List<FieldError> errors) implements Parsed {}

  static Parsed parse(String body, String cardCurrency) {
    JsonNode root;
    try {
      root = MAPPER.readTree(body);
    } catch (Exception e) {
      root = null;
    }
    if (root == null || !root.isObject()) {
      return new Invalid(List.of(new FieldError("body", "must be a JSON object")));
    }
    List<FieldError> errors = new ArrayList<>();
    Long perTxn = amount(root, "perTransactionAmount", cardCurrency, errors);
    Long daily = amount(root, "dailyAmount", cardCurrency, errors);
    Integer dailyCount = dailyCount(root.path("dailyCount"), errors);
    if (perTxn != null && daily != null && perTxn > daily) {
      errors.add(
          new FieldError("perTransactionAmount.amount", "must not exceed dailyAmount.amount"));
    }
    return errors.isEmpty()
        ? new Valid(new CardLimitsRequest(perTxn, daily, dailyCount))
        : new Invalid(List.copyOf(errors));
  }

  private static Long amount(
      JsonNode root, String field, String cardCurrency, List<FieldError> errors) {
    JsonNode money = root.path(field);
    if (!cardCurrency.equals(money.path("currency").asText(null))) {
      errors.add(
          new FieldError(field + ".currency", "must be the card's currency " + cardCurrency));
    }
    JsonNode amount = money.path("amount");
    if (amount.isIntegralNumber() && amount.canConvertToLong() && amount.asLong() > 0) {
      return amount.asLong();
    }
    errors.add(new FieldError(field + ".amount", "must be a positive integer in minor units"));
    return null;
  }

  private static Integer dailyCount(JsonNode node, List<FieldError> errors) {
    if (node.isMissingNode() || node.isNull()) return null;
    if (node.isIntegralNumber() && node.canConvertToInt() && node.asInt() > 0) return node.asInt();
    errors.add(new FieldError("dailyCount", "must be null or a positive integer"));
    return null;
  }
}
