export interface DomainEvent {
  source: string;
  detailType: string;
  detail: Record<string, unknown>;
}

export interface EventBus {
  publish(event: DomainEvent): Promise<void>;
  close(): Promise<void>;
}
