"use client";

import { useTranslations } from "next-intl";
import { TRANSACTION_TYPES, type TransactionType } from "./pos-model";
import { Segmented } from "./Segmented";

interface TransactionTypeSelectorProps {
  value: TransactionType;
  onChange: (type: TransactionType) => void;
}

export function TransactionTypeSelector({ value, onChange }: TransactionTypeSelectorProps) {
  const t = useTranslations("pos.transactionType");
  const options = TRANSACTION_TYPES.map((id) => ({ id, text: t(id) }));
  return <Segmented label={t("label")} options={options} value={value} onChange={onChange} />;
}
