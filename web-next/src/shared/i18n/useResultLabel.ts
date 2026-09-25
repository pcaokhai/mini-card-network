import { useTranslations } from "next-intl";
import type { components } from "@/shared/api/generated/schema";

type TransactionStatus = components["schemas"]["TransactionStatus"];

// Only a decision from the issuer is described by its response code; every other state
// (pending, reversed, timed out) is described by what happened to the transaction itself.
const DECIDED: ReadonlySet<TransactionStatus> = new Set(["APPROVED", "DECLINED"]);

/**
 * Plain-language result labels in the console's locale. The gateway's `responseLabel` is
 * English-only, so labels resolve from docs/03 §8's response codes here and fall back to the
 * server text only for a code the table does not know.
 */
export function useResultLabel() {
  const tRc = useTranslations("responseCodes");
  const tStatus = useTranslations("transactionStatus");

  function forCode(responseCode: string | null | undefined, fallback?: string | null): string | null {
    if (responseCode && tRc.has(responseCode)) return tRc(responseCode);
    return fallback ?? null;
  }

  function forTransaction(
    status: TransactionStatus,
    responseCode?: string | null,
    fallback?: string | null,
  ): string {
    if (DECIDED.has(status)) {
      const byCode = forCode(responseCode, fallback);
      if (byCode !== null) return byCode;
    }
    return tStatus(status);
  }

  return { forCode, forTransaction };
}
