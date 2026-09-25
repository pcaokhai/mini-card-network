"use client";

// Display-only copy of contracts/fixtures/cards.json - cardToken, last-4, holder name and
// status only. The raw `pan` field must never be embedded in frontend source.
const DISPLAY_CARDS = [
  { cardToken: "tok_normal", last4: "4417", holderName: "Nguyen Minh Anh", status: "ACTIVE" },
  { cardToken: "tok_low", last4: "9021", holderName: "Tran Thu Ha", status: "ACTIVE" },
  { cardToken: "tok_blocked", last4: "3310", holderName: "Le Quoc Bao", status: "BLOCKED" },
  { cardToken: "tok_expired", last4: "7765", holderName: "Pham Gia Huy", status: "ACTIVE" },
  { cardToken: "tok_limit", last4: "1208", holderName: "Vo Thanh Tam", status: "ACTIVE" },
  { cardToken: "tok_second", last4: "5540", holderName: "Dang Ngoc Linh", status: "ACTIVE" },
] as const;

interface CardPickerProps {
  selected: string | null;
  onSelect: (cardToken: string) => void;
}

export function CardPicker({ selected, onSelect }: CardPickerProps) {
  return (
    <div role="radiogroup" aria-label="Test card" className="grid grid-cols-2 gap-3">
      {DISPLAY_CARDS.map((card) => (
        <button
          key={card.cardToken}
          type="button"
          role="radio"
          aria-checked={selected === card.cardToken}
          onClick={() => onSelect(card.cardToken)}
          className={
            selected === card.cardToken
              ? "rounded-card border-2 border-accent bg-accent-soft p-3 text-left"
              : "rounded-card border border-border bg-surface p-3 text-left hover:bg-canvas"
          }
        >
          <div className="font-mono text-sm text-ink">970436 •• •••• {card.last4}</div>
          <div className="text-sm text-muted">{card.holderName}</div>
          <div className="text-xs text-muted">{card.status}</div>
        </button>
      ))}
    </div>
  );
}
