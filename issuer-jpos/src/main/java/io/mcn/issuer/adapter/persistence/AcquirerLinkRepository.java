package io.mcn.issuer.adapter.persistence;

import java.util.Optional;

public interface AcquirerLinkRepository {
  void upsertStatus(String acquirerId, String status);

  Optional<String> findStatus(String acquirerId);
}
