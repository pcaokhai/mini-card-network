package io.mcn.issuer.adapter.txn;

import com.zaxxer.hikari.HikariDataSource;
import io.mcn.issuer.adapter.persistence.AccountLockRepository;
import io.mcn.issuer.adapter.persistence.AccountRow;
import io.mcn.issuer.adapter.persistence.LedgerRepository;
import io.mcn.issuer.adapter.persistence.VelocityCounterRepository;
import java.io.Serializable;
import java.sql.Connection;
import java.sql.SQLException;
import java.time.LocalDate;
import javax.sql.DataSource;
import org.jpos.core.Configurable;
import org.jpos.core.Configuration;
import org.jpos.core.ConfigurationException;
import org.jpos.transaction.Context;
import org.jpos.transaction.TransactionParticipant;
import org.jpos.util.Destroyable;

/**
 * Real approval (MCN-302b, replacing MCN-302a's RC-96 placeholder): row-locks the card's account,
 * approves (RC 00 + DE 38 auth code) and posts a balanced double-entry journal when funds are
 * sufficient, or declines RC 51 otherwise. Never touches the account when an earlier participant
 * already set {@code RESPONSE_CODE} (a decline, or a replayed duplicate).
 *
 * <p><b>Task 0 finding:</b> jPOS 3.0.1's {@code TransactionManager} hands every participant in a
 * transaction the same {@code Context}, but never a shared JDBC {@code Connection} - confirmed both
 * from {@code TransactionManager}'s source (each participant is {@code Configurable} and wires up
 * its own resources in {@code setConfiguration}; the manager only ever calls {@code prepare}/{@code
 * commit}/{@code abort} with the {@code Context}) and from every participant already merged in this
 * codebase (CheckCard, CheckLimits, Deduplicate, LogAndOutbox each build their own {@link
 * HikariDataSource} from Q2 config via {@link TxnDataSource}). A row lock taken on one
 * participant's connection would therefore be silently released the moment that connection is
 * returned to its pool - long before a later participant's {@code commit()} runs. So the row lock,
 * the debit and the journal insert all happen here, in {@link #prepare}, inside one explicit JDBC
 * transaction on one connection this class opens and closes itself; nothing is deferred to {@code
 * commit()}/{@code abort()} (this participant does not implement {@code AbortParticipant} - there
 * is nothing left to compensate for once {@code prepare()} has already committed or rolled back its
 * own transaction).
 *
 * <p>Needs {@code LogAndOutbox} to have already run (it precedes this participant in {@code
 * 40_txn_manager.xml}) so {@code TxnContextKeys.TRAN_ID} names an already-committed {@code
 * tran_log} row - {@code journal_entry.tran_id} has a real, non-deferred FK to it.
 */
public class Authorize implements TransactionParticipant, Configurable, Destroyable {

  private static final String CURRENCY = "704";
  private static final String PURCHASE_TRAN_TYPE = "PURCHASE";

  private AccountLockRepository lockRepository;
  private LedgerRepository ledgerRepository;
  private VelocityCounterRepository velocityCounterRepository;
  private final AuthCodeGenerator authCodeGenerator;
  private DataSource dataSource;
  private HikariDataSource ownedDataSource;

  /** No-arg constructor for Q2's {@code QFactory.newInstance}; see {@link #setConfiguration}. */
  public Authorize() {
    this(null, null, null, new AuthCodeGenerator(), null);
  }

  public Authorize(
      AccountLockRepository lockRepository,
      LedgerRepository ledgerRepository,
      VelocityCounterRepository velocityCounterRepository,
      AuthCodeGenerator authCodeGenerator) {
    this(lockRepository, ledgerRepository, velocityCounterRepository, authCodeGenerator, null);
  }

  public Authorize(
      AccountLockRepository lockRepository,
      LedgerRepository ledgerRepository,
      VelocityCounterRepository velocityCounterRepository,
      AuthCodeGenerator authCodeGenerator,
      DataSource dataSource) {
    this.lockRepository = lockRepository;
    this.ledgerRepository = ledgerRepository;
    this.velocityCounterRepository = velocityCounterRepository;
    this.authCodeGenerator = authCodeGenerator;
    this.dataSource = dataSource;
  }

  @Override
  public void setConfiguration(Configuration cfg) throws ConfigurationException {
    this.ownedDataSource = TxnDataSource.fromConfig(cfg);
    this.dataSource = ownedDataSource;
    this.lockRepository = new AccountLockRepository(this.dataSource);
    this.ledgerRepository = new LedgerRepository();
    this.velocityCounterRepository = new VelocityCounterRepository(this.dataSource);
  }

  @Override
  public void destroy() {
    if (ownedDataSource != null) {
      ownedDataSource.close();
    }
  }

  @Override
  public int prepare(long id, Serializable context) {
    Context ctx = (Context) context;
    if (ctx.<String>get(TxnContextKeys.RESPONSE_CODE) != null) {
      return PREPARED;
    }

    long accountId = ctx.get(TxnContextKeys.ACCOUNT_ID);
    long amount = ctx.get(TxnContextKeys.AMOUNT);

    try (Connection conn = dataSource.getConnection()) {
      conn.setAutoCommit(false);
      AccountRow account = lockRepository.lockAndGet(conn, accountId);

      if (account.availableBalance() < amount) {
        conn.rollback();
        ctx.put(TxnContextKeys.RESPONSE_CODE, "51");
        ctx.put(TxnContextKeys.DECLINE_REASON, "insufficient funds");
        return PREPARED;
      }

      boolean debited = lockRepository.debit(conn, accountId, amount, account.version());
      if (!debited) {
        // ponytail: the row lock (FOR UPDATE) serializes every writer on this account within one
        // DB transaction - a failed optimistic-version check here means the lock isn't actually
        // being held, which is a real bug, not a retryable race. Fail loudly rather than looping.
        throw new IllegalStateException(
            "account " + accountId + " version changed under a held row lock");
      }

      String authCode = authCodeGenerator.generate();
      Long tranId = ctx.get(TxnContextKeys.TRAN_ID);
      LocalDate businessDate = ctx.get(TxnContextKeys.BUSINESS_DATE);
      LocalDate effectiveBusinessDate = businessDate == null ? LocalDate.now() : businessDate;
      ledgerRepository.postPurchase(
          conn, tranId == null ? 0L : tranId, effectiveBusinessDate, accountId, amount, CURRENCY);

      Long cardId = ctx.get(TxnContextKeys.CARD_ID);
      if (cardId != null) {
        velocityCounterRepository.incrementDaily(
            conn, cardId, PURCHASE_TRAN_TYPE, effectiveBusinessDate, amount);
      }

      conn.commit();
      ctx.put(TxnContextKeys.RESPONSE_CODE, "00");
      ctx.put(TxnContextKeys.AUTH_CODE, authCode);
      return PREPARED;
    } catch (SQLException e) {
      throw new IllegalStateException("authorize failed", e);
    }
  }
}
