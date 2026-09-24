import { useEffect, useRef, useState } from "react";

/** Wire envelope from contracts/ws-events.schema.json. */
export interface WsEnvelope {
  id: string;
  type: string;
  occurredAt: string;
  data: unknown;
}

const BASE_BACKOFF_MS = 1000;
const MAX_BACKOFF_MS = 30_000;

/**
 * MCN-306: first live WS consumer in web-next. Opens one socket to
 * gateway-go's hub (internal/ws/hub.go, mounted at /v1/stream), filters by
 * event type, and reconnects with exponential backoff on close.
 */
export function useWsEvents(eventTypes: string[], onEvent: (event: WsEnvelope) => void): { connected: boolean } {
  const onEventRef = useRef(onEvent);
  const [connected, setConnected] = useState(false);

  useEffect(() => {
    onEventRef.current = onEvent;
  }, [onEvent]);

  useEffect(() => {
    let socket: WebSocket | null = null;
    let closed = false;
    let attempt = 0;
    let timeoutId: ReturnType<typeof setTimeout> | undefined;

    function connect() {
      if (closed) return;
      // NEXT_PUBLIC_WS_URL is a workaround for web-next's missing BFF proxy layer (CLAUDE.md
      // documents Route Handlers under src/app/api/ that were never built) - it points the
      // browser directly at the gateway. Falls back to same-origin, the intended topology
      // once the proxy exists.
      const url =
        process.env.NEXT_PUBLIC_WS_URL ??
        `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/v1/stream`;
      socket = new WebSocket(url);
      socket.onopen = () => {
        attempt = 0;
        setConnected(true);
      };
      socket.onmessage = (event) => {
        const envelope: WsEnvelope = JSON.parse(event.data);
        if (eventTypes.includes(envelope.type)) onEventRef.current(envelope);
      };
      socket.onclose = () => {
        setConnected(false);
        if (closed) return;
        const delay = Math.min(MAX_BACKOFF_MS, BASE_BACKOFF_MS * 2 ** attempt);
        attempt += 1;
        timeoutId = setTimeout(connect, delay);
      };
    }
    connect();

    return () => {
      closed = true;
      clearTimeout(timeoutId);
      socket?.close();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [eventTypes.join(",")]);

  return { connected };
}
