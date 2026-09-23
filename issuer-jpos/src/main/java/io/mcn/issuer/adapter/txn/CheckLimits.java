package io.mcn.issuer.adapter.txn;

import com.zaxxer.hikari.HikariDataSource;
import io.mcn.issuer.adapter.persistence.CardLimitRepository;
import io.mcn.issuer.adapter.persistence.VelocityCounterRepository;
import io.mcn.issuer.application.VelocityRule;
import java.io.Serializable;
import java.lang.reflect.Method;
import java.util.ArrayList;
import java.util.List;
import org.jpos.core.Configurable;
import org.jpos.core.Configuration;
import org.jpos.core.ConfigurationException;
import org.jpos.transaction.Context;
import org.jpos.transaction.TransactionParticipant;
import org.jpos.util.Destroyable;

/**
 * Runs the configured {@link VelocityRule} strategies in order, aborting with whichever RC the
 * first declining rule returns (RC 61 amount ceilings, RC 65 frequency ceilings - docs/03 §8).
 * Account balance (RC 51) is Authorize's scope. The rule set itself is config-driven (MCN-803-AC2,
 * see {@link #setConfiguration}) - this class holds no rule-kind-specific branching.
 */
public class CheckLimits implements TransactionParticipant, Configurable, Destroyable {

  private List<VelocityRule> rules;
  private HikariDataSource dataSource;

  /** No-arg constructor for Q2's {@code QFactory.newInstance}; see {@link #setConfiguration}. */
  public CheckLimits() {}

  public CheckLimits(List<VelocityRule> rules) {
    this.rules = rules;
  }

  @Override
  public void setConfiguration(Configuration cfg) throws ConfigurationException {
    this.dataSource = TxnDataSource.fromConfig(cfg);
    CardLimitRepository cardLimitRepository = new CardLimitRepository(dataSource);
    VelocityCounterRepository velocityCounterRepository = new VelocityCounterRepository(dataSource);
    this.rules = resolveRules(cfg.get("rules", ""), cardLimitRepository, velocityCounterRepository);
  }

  /** One class name per {@link VelocityRule}, each exposing a static {@code create} factory. */
  private static List<VelocityRule> resolveRules(
      String commaSeparatedClassNames,
      CardLimitRepository cardLimitRepository,
      VelocityCounterRepository velocityCounterRepository)
      throws ConfigurationException {
    List<VelocityRule> resolved = new ArrayList<>();
    for (String className : commaSeparatedClassNames.split(",")) {
      String trimmed = className.trim();
      if (trimmed.isEmpty()) continue;
      try {
        Class<?> clazz = Class.forName(trimmed);
        Method factory =
            clazz.getMethod("create", CardLimitRepository.class, VelocityCounterRepository.class);
        resolved.add(
            (VelocityRule) factory.invoke(null, cardLimitRepository, velocityCounterRepository));
      } catch (ReflectiveOperationException e) {
        throw new ConfigurationException("failed to load velocity rule " + trimmed, e);
      }
    }
    return resolved;
  }

  @Override
  public void destroy() {
    if (dataSource != null) {
      dataSource.close();
    }
  }

  @Override
  public int prepare(long id, Serializable context) {
    Context ctx = (Context) context;
    long cardId = ctx.get(TxnContextKeys.CARD_ID);
    long amount = ctx.get(TxnContextKeys.AMOUNT);

    for (VelocityRule rule : rules) {
      var declineRc = rule.evaluate(cardId, amount);
      if (declineRc.isPresent()) {
        ctx.put(TxnContextKeys.RESPONSE_CODE, declineRc.get());
        return ABORTED;
      }
    }
    return PREPARED;
  }
}
