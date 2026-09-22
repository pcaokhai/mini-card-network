import { useTranslations } from "next-intl";
import { formatMoney } from "@/shared/format/money";
import type { JournalEntry } from "@/shared/api/cards-client";

export function LedgerTable({ entries }: { entries: JournalEntry[] }) {
  const t = useTranslations("cards.ledger");
  return (
    <table className="w-full text-sm">
      <caption className="mb-1 text-left text-xs text-muted">{t("title")}</caption>
      <thead>
        <tr className="text-left text-xs text-muted">
          <th className="py-1 font-normal">{t("description")}</th>
          <th className="py-1 font-normal">{t("postings")}</th>
        </tr>
      </thead>
      <tbody>
        {entries.map((entry) => (
          <tr key={entry.journalId} className="border-t border-border">
            <td className="py-2">
              <p>{entry.description}</p>
              <p className="text-xs text-muted">{new Date(entry.occurredAt).toLocaleString()}</p>
            </td>
            <td className="py-2">
              {entry.postings.map((posting, i) => (
                <p key={i} className="font-mono text-xs">
                  {posting.direction === "DEBIT" ? "-" : "+"}
                  {formatMoney(posting.amount)} {posting.account}
                </p>
              ))}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
