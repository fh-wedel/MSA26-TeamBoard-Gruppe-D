import {
  EventBridgeClient,
  PutEventsCommand,
} from "@aws-sdk/client-eventbridge";
import type { Redis } from "ioredis";
import type { KanbanConfig } from "../config";

export interface DomainEvent {
  source: string;
  detailType: string;
  detail: Record<string, unknown>;
}

export interface EventBus {
  publish(event: DomainEvent): Promise<void>;
  close(): Promise<void>;
}

export const REDIS_EVENT_CHANNEL = "events.global";

class RedisBus implements EventBus {
  constructor(private readonly redis: Redis) {}
  async publish(event: DomainEvent): Promise<void> {
    await this.redis.publish(REDIS_EVENT_CHANNEL, JSON.stringify(event));
  }
  async close(): Promise<void> {
    /* owned by caller */
  }
}

class EventBridgeBus implements EventBus {
  private readonly client: EventBridgeClient;
  constructor(
    private readonly busName: string,
    region: string,
  ) {
    this.client = new EventBridgeClient({ region });
  }
  async publish(event: DomainEvent): Promise<void> {
    await this.client.send(
      new PutEventsCommand({
        Entries: [
          {
            EventBusName: this.busName,
            Source: event.source,
            DetailType: event.detailType,
            Detail: JSON.stringify(event.detail),
          },
        ],
      }),
    );
  }
  async close(): Promise<void> {
    this.client.destroy();
  }
}

export function createEventBus(config: KanbanConfig, redis: Redis): EventBus {
  if (config.eventBus === "eventbridge") {
    return new EventBridgeBus(config.eventBusName, config.awsRegion);
  }
  return new RedisBus(redis);
}
