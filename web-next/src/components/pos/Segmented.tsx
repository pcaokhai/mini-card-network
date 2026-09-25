"use client";

interface SegmentedProps<T extends string> {
  label: string;
  options: readonly { id: T; text: string }[];
  value: T;
  onChange: (id: T) => void;
}

/** The canvas's on/off segmented control, styled like the header's Easy/Expert toggle. */
export function Segmented<T extends string>({ label, options, value, onChange }: SegmentedProps<T>) {
  return (
    <div role="group" aria-label={label} className="pos-seg">
      {options.map((option) => (
        <button
          key={option.id}
          type="button"
          aria-pressed={value === option.id}
          onClick={() => onChange(option.id)}
          className="pos-seg__btn"
        >
          {option.text}
        </button>
      ))}
    </div>
  );
}
