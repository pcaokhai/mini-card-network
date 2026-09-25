/** The canvas's live indicator: a solid dot with a ring that expands and fades every 2 s. */
export function PulseDot() {
  return (
    <span aria-hidden className="relative size-2 shrink-0">
      <span className="absolute inset-0 animate-mcn-pulse rounded-full bg-current" />
      <span className="absolute inset-0 rounded-full bg-current" />
    </span>
  );
}
