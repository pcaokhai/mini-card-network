import { act, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderWithIntl } from "@/test/render";
import { LiveFeed } from "./LiveFeed";

vi.mock("@/shared/ws/useWsEvents", () => ({
  useWsEvents: (types: string[], onEvent: (e: unknown) => void) => {
    (globalThis as { __emit?: (e: unknown) => void }).__emit = onEvent;
    return { connected: true };
  },
}));

describe("LiveFeed", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  function emit(event: unknown) {
    (globalThis as { __emit?: (e: unknown) => void }).__emit?.(event);
  }

  it("renders a new row immediately below the 5 events/s threshold", () => {
    renderWithIntl(<LiveFeed />);
    act(() => {
      emit({ type: "transaction.created", data: { rrn: "a", status: "APPROVED" }, occurredAt: "now" });
    });
    expect(screen.getByText("a")).toBeInTheDocument();
  });

  it("caps the feed at 30 rows, dropping the oldest", () => {
    renderWithIntl(<LiveFeed />);
    act(() => {
      for (let i = 0; i < 35; i++) {
        emit({ type: "transaction.created", data: { rrn: `r${i}`, status: "APPROVED" }, occurredAt: "now" });
      }
    });
    // this burst exceeds the 5/s threshold, so the tail is batched - flush it.
    act(() => {
      vi.advanceTimersByTime(500);
    });
    expect(screen.getAllByRole("listitem")).toHaveLength(30);
    expect(screen.queryByText("r0")).not.toBeInTheDocument();
    expect(screen.getByText("r34")).toBeInTheDocument();
  });

  it("batches updates into one render per 500ms once the rate exceeds 5/s", () => {
    renderWithIntl(<LiveFeed />);
    act(() => {
      for (let i = 0; i < 10; i++) {
        emit({ type: "transaction.created", data: { rrn: `burst${i}`, status: "APPROVED" }, occurredAt: "now" });
      }
    });
    expect(screen.queryByText("burst9")).not.toBeInTheDocument();
    act(() => {
      vi.advanceTimersByTime(500);
    });
    expect(screen.getByText("burst9")).toBeInTheDocument();
  });

  it("MCN-306-AC3: Expert mode shows the RC code beside the result; Easy mode does not", () => {
    const declined = {
      type: "transaction.created",
      data: { rrn: "626514000122", status: "DECLINED", responseCode: "51", responseLabel: "Không đủ tiền" },
      occurredAt: "now",
    };
    const { unmount } = renderWithIntl(<LiveFeed expertMode />);
    act(() => emit(declined));
    expect(screen.getByTestId("feed-status")).toHaveTextContent("Không đủ tiền · RC 51");
    unmount();

    renderWithIntl(<LiveFeed />);
    act(() => emit(declined));
    expect(screen.getByTestId("feed-status")).toHaveTextContent(/^Không đủ tiền$/);
  });

  it("MCN-306-AC2: new transactions slide in at the top with a highlight", () => {
    renderWithIntl(<LiveFeed />);
    act(() => emit({ type: "transaction.created", data: { rrn: "older", status: "APPROVED" }, occurredAt: "now" }));
    act(() => emit({ type: "transaction.created", data: { rrn: "newer", status: "APPROVED" }, occurredAt: "now" }));
    const [first] = screen.getAllByRole("listitem");
    expect(first).toHaveTextContent("newer");
    expect(first).toHaveAttribute("data-new", "true");
  });

  it("an update replaces the row for the same RRN instead of adding a duplicate", () => {
    renderWithIntl(<LiveFeed />);
    act(() => emit({ type: "transaction.created", data: { rrn: "same", status: "SENT" }, occurredAt: "now" }));
    act(() => emit({ type: "transaction.updated", data: { rrn: "same", status: "REVERSED", responseCode: "91" }, occurredAt: "now" }));
    expect(screen.getAllByRole("listitem")).toHaveLength(1);
    expect(screen.getByTestId("feed-status")).toHaveTextContent("Đã tự hủy");
  });
});
