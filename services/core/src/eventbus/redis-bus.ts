import type { Redis } from "ioredis";
import type { DomainEvent, EventBus } from "./types";

export const REDIS_EVENT_CHANNEL = "events.global";

export type EventTapHandler = (event: DomainEvent) => void;

export class RedisPubSubBus implements EventBus {
  private subscriber: Redis | null = null;
  private subscribePromise: Promise<void> | null = null;
  private readonly tapHandlers = new Set<EventTapHandler>();

  constructor(private readonly redis: Redis) {}

  async publish(event: DomainEvent): Promise<void> {
    await this.redis.publish(REDIS_EVENT_CHANNEL, JSON.stringify(event));
  }

  async close(): Promise<void> {
    if (this.subscriber) {
      try {
        await this.subscriber.quit();
      } catch {
        // best effort during shutdown
      }
      this.subscriber = null;
    }
    this.tapHandlers.clear();
  }

  /**
   * Subscribe to every event flowing through the bus. Returns an unsubscribe
   * function. The first call lazily creates a dedicated subscriber connection.
   * Not part of the EventBus interface — implementation detail used by the
   * admin SSE endpoint.
   */
  tap(handler: EventTapHandler): () => void {
    this.tapHandlers.add(handler);
    void this.ensureSubscribed();
    return () => {
      this.tapHandlers.delete(handler);
    };
  }

  private async ensureSubscribed(): Promise<void> {
    if (this.subscriber) return;
    if (!this.subscribePromise) {
      const client = this.redis.duplicate();
      this.subscriber = client;
      client.on("error", () => {
        // surfaced via the parent client's error handler
      });
      client.on("message", (_channel, message) => {
        let event: DomainEvent;
        try {
          event = JSON.parse(message) as DomainEvent;
        } catch {
          return;
        }
        for (const handler of this.tapHandlers) {
          try {
            handler(event);
          } catch {
            // isolate handler failures
          }
        }
      });
      this.subscribePromise = client.subscribe(REDIS_EVENT_CHANNEL).then(() => undefined);
    }
    await this.subscribePromise;
  }
}
