"use client";

import { useTranslations } from "next-intl";
import { Keypad, type KeypadKey } from "./Keypad";
import type { ResultKind } from "./pos-model";

interface PosDeviceProps {
  merchantName: string;
  terminalId: string;
  label: string;
  amountText: string;
  message: string;
  messageKind: ResultKind | null;
  processing: boolean;
  keypadDisabled: boolean;
  payDisabled: boolean;
  onKey: (key: KeypadKey) => void;
  onPay: () => void;
}

/** The dark terminal from the canvas: merchant line, green screen, keypad and pay button. */
export function PosDevice(props: PosDeviceProps) {
  const t = useTranslations("pos.device");
  return (
    <div className="pos-device">
      <div className="pos-device__meta">
        <span>{props.merchantName}</span>
        <span className="pos-device__tid">{t("terminal", { terminalId: props.terminalId })}</span>
      </div>
      <div role="status" aria-live="polite" className="pos-lcd">
        <div className="pos-lcd__label">{props.label}</div>
        <div className="pos-lcd__amount">{props.amountText} ₫</div>
        <div className="pos-lcd__msg" data-kind={props.messageKind ?? undefined}>
          {props.processing ? t("processing") : props.message}
          {props.processing && (
            <span aria-hidden="true">
              <span className="pos-dot">.</span>
              <span className="pos-dot">.</span>
              <span className="pos-dot">.</span>
            </span>
          )}
        </div>
      </div>
      <Keypad onKey={props.onKey} disabled={props.keypadDisabled} />
      <button type="button" onClick={props.onPay} disabled={props.payDisabled} className="pos-pay">
        {props.processing ? t("paying") : t("pay")}
      </button>
    </div>
  );
}
