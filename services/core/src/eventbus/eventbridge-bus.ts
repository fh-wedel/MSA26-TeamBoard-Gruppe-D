import {
  EventBridgeClient,
  PutEventsCommand,
} from "@aws-sdk/client-eventbridge";
import type { DomainEvent, EventBus } from "./types";

export class EventBridgeBus implements EventBus {
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
