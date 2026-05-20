import { useEffect, useRef, useState } from "react";
import type { StreamEvent } from "../api/types";

interface RawSseEvent {
  receivedAt?: string;
  source?: string;
  detailType?: string;
  detail?: unknown;
}

function coerce(raw: unknown): StreamEvent | null {
  if (!raw || typeof raw !== "object") return null;
  const r = raw as RawSseEvent;
  if (typeof r.source !== "string" || typeof r.detailType !== "string") return null;
  return {
    receivedAt: typeof r.receivedAt === "string" ? r.receivedAt : new Date().toISOString(),
    source: r.source,
    detailType: r.detailType,
    detail:
      r.detail && typeof r.detail === "object"
        ? (r.detail as Record<string, unknown>)
        : {},
  };
}

export interface EventStreamState {
  events: StreamEvent[];
  status: "connecting" | "open" | "closed";
}

/**
 * Subscribe to the admin SSE stream. Returns the last `windowMs` of events
 * plus a status indicator.
 */
export function useEventStream(url: string, windowMs = 30_000): EventStreamState {
  const [state, setState] = useState<EventStreamState>({
    events: [],
    status: "connecting",
  });
  const bufferRef = useRef<StreamEvent[]>([]);

  useEffect(() => {
    let cancelled = false;
    const source = new EventSource(url);

    const prune = (events: StreamEvent[]): StreamEvent[] => {
      const cutoff = Date.now() - windowMs;
      return events.filter((e) => {
        const t = Date.parse(e.receivedAt);
        return Number.isNaN(t) || t >= cutoff;
      });
    };

    source.onopen = (): void => {
      if (cancelled) return;
      setState((prev) => ({ ...prev, status: "open" }));
    };

    source.onerror = (): void => {
      if (cancelled) return;
      setState((prev) => ({ ...prev, status: "closed" }));
      // EventSource auto-reconnects; UI will reflect transitions.
    };

    source.onmessage = (ev: MessageEvent<string>): void => {
      if (cancelled) return;
      let parsed: unknown;
      try {
        parsed = JSON.parse(ev.data);
      } catch {
        return;
      }
      const evt = coerce(parsed);
      if (!evt) return;
      bufferRef.current = prune([evt, ...bufferRef.current]).slice(0, 200);
      setState({ status: "open", events: bufferRef.current });
    };

    const pruneTimer = window.setInterval(() => {
      if (cancelled) return;
      const next = prune(bufferRef.current);
      if (next.length !== bufferRef.current.length) {
        bufferRef.current = next;
        setState((prev) => ({ ...prev, events: next }));
      }
    }, 2000);

    return (): void => {
      cancelled = true;
      window.clearInterval(pruneTimer);
      source.close();
    };
  }, [url, windowMs]);

  return state;
}
