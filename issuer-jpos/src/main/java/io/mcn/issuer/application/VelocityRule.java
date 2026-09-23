package io.mcn.issuer.application;

import java.util.Optional;

/**
 * One independent velocity check, evaluated against a card and a candidate transaction amount.
 * Returns the ISO 8583 response code to decline with, or empty to allow. Stateless: a rule reads
 * only its own repository/config, never another rule's intermediate state.
 */
public interface VelocityRule {
  Optional<String> evaluate(long cardId, long amountMinorUnits);
}
