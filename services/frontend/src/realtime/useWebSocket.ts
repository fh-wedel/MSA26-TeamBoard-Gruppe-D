import { useEffect, useRef, useState } from "react";

export type WsStatus = "connecting" | "open" | "closed";

interface UseWebSocketOptions {
  url: string;
  onMessage?: (data: unknown) => void;
  boardId?: number | null;
  reconnectDelayMs?: number;
}

export function useWebSocket(options: UseWebSocketOptions): {
  status: WsStatus;
  send: (data: unknown) => void;
} {
  const { url, onMessage, boardId, reconnectDelayMs = 2000 } = options;
  const [status, setStatus] = useState<WsStatus>("connecting");
  const socketRef = useRef<WebSocket | null>(null);
  const handlerRef = useRef(onMessage);
  const boardIdRef = useRef(boardId);
  handlerRef.current = onMessage;
  boardIdRef.current = boardId;

  useEffect(() => {
    let cancelled = false;
    let reconnectTimer: number | undefined;

    const connect = (): void => {
      if (cancelled) return;
      setStatus("connecting");
      const ws = new WebSocket(url);
      socketRef.current = ws;

      ws.onopen = (): void => {
        if (cancelled) return;
        setStatus("open");
        const bId = boardIdRef.current;
        if (typeof bId === "number") {
          ws.send(JSON.stringify({ boardId: bId }));
        }
      };

      ws.onmessage = (ev: MessageEvent<string>): void => {
        if (!handlerRef.current) return;
        let parsed: unknown;
        try {
          parsed = JSON.parse(ev.data);
        } catch {
          return;
        }
        handlerRef.current(parsed);
      };

      ws.onclose = (): void => {
        if (cancelled) return;
        setStatus("closed");
        socketRef.current = null;
        reconnectTimer = window.setTimeout(connect, reconnectDelayMs);
      };

      ws.onerror = (): void => {
        // Let onclose drive the reconnect.
        ws.close();
      };
    };

    connect();

    return (): void => {
      cancelled = true;
      if (reconnectTimer) window.clearTimeout(reconnectTimer);
      if (socketRef.current) {
        socketRef.current.close();
        socketRef.current = null;
      }
    };
  }, [url, reconnectDelayMs]);

  // When boardId changes after connect, push the new filter.
  useEffect(() => {
    const ws = socketRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({ boardId: boardId ?? null }));
  }, [boardId]);

  return {
    status,
    send: (data: unknown): void => {
      const ws = socketRef.current;
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(typeof data === "string" ? data : JSON.stringify(data));
      }
    },
  };
}
