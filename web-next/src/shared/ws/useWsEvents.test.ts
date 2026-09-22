import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useWsEvents } from "./useWsEvents";

class FakeWebSocket {
  static instances: FakeWebSocket[] = [];
  onopen: (() => void) | null = null;
  onmessage: ((e: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  readyState = 0;
  constructor(public url: string) {
    FakeWebSocket.instances.push(this);
  }
  close() {
    this.onclose?.();
  }
}

beforeEach(() => {
  FakeWebSocket.instances = [];
  vi.stubGlobal("WebSocket", FakeWebSocket);
});
afterEach(() => vi.unstubAllGlobals());

describe("useWsEvents", () => {
  it("calls onEvent only for subscribed event types", () => {
    const onEvent = vi.fn();
    renderHook(() => useWsEvents(["transaction.created"], onEvent));
    const ws = FakeWebSocket.instances[0];

    act(() => {
      ws.onmessage?.({
        data: JSON.stringify({ id: "1", type: "transaction.created", occurredAt: "now", data: { rrn: "x" } }),
      });
      ws.onmessage?.({ data: JSON.stringify({ id: "2", type: "link.status", occurredAt: "now", data: {} }) });
    });

    expect(onEvent).toHaveBeenCalledTimes(1);
    expect(onEvent).toHaveBeenCalledWith(expect.objectContaining({ type: "transaction.created" }));
  });

  it("cleans up the socket on unmount", () => {
    const { unmount } = renderHook(() => useWsEvents(["transaction.created"], vi.fn()));
    const ws = FakeWebSocket.instances[0];
    const closeSpy = vi.spyOn(ws, "close");
    unmount();
    expect(closeSpy).toHaveBeenCalled();
  });
});
