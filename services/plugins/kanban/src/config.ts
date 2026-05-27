export type StorageBackend = "postgres" | "dynamodb";
export type EventBusBackend = "redis" | "eventbridge";

export interface KanbanConfig {
  port: number;
  pluginId: string;
  pluginVersion: string;
  publicEndpoint: string;
  coreUrl: string;
  storage: StorageBackend;
  databaseUrl: string;
  ticketsTable: string;
  dynamoEndpoint?: string;
  eventBus: EventBusBackend;
  redisUrl: string;
  awsRegion: string;
  eventBusName: string;
  heartbeatIntervalMs: number;
}

function num(name: string, fallback: number): number {
  const raw = process.env[name];
  if (!raw) return fallback;
  const parsed = Number(raw);
  if (Number.isNaN(parsed)) throw new Error(`${name} must be a number`);
  return parsed;
}

export function loadConfig(): KanbanConfig {
  const storageRaw = (process.env.STORAGE ?? "postgres").toLowerCase();
  if (storageRaw !== "postgres" && storageRaw !== "dynamodb") {
    throw new Error(`STORAGE must be postgres or dynamodb`);
  }
  const eventBusRaw = (process.env.EVENT_BUS ?? "redis").toLowerCase();
  if (eventBusRaw !== "redis" && eventBusRaw !== "eventbridge") {
    throw new Error(`EVENT_BUS must be redis or eventbridge`);
  }
  return {
    port: num("KANBAN_PORT", 3001),
    pluginId: process.env.KANBAN_PLUGIN_ID ?? "kanban-board",
    pluginVersion: process.env.KANBAN_PLUGIN_VERSION ?? "1.0.0",
    publicEndpoint: process.env.KANBAN_PUBLIC_ENDPOINT ?? "http://plugin-kanban:3001",
    coreUrl: process.env.CORE_URL ?? "http://core:3000",
    storage: storageRaw,
    databaseUrl:
      process.env.DATABASE_URL ?? "postgresql://postgres:postgres@postgres:5432/poc",
    ticketsTable: process.env.TICKETS_TABLE ?? "msa2-tickets",
    ...(process.env.DYNAMO_ENDPOINT
      ? { dynamoEndpoint: process.env.DYNAMO_ENDPOINT }
      : {}),
    eventBus: eventBusRaw,
    redisUrl: process.env.REDIS_URL ?? "redis://redis:6379",
    awsRegion: process.env.AWS_REGION ?? "eu-central-1",
    eventBusName: process.env.EVENT_BUS_NAME ?? "msa2-bus",
    heartbeatIntervalMs: num("HEARTBEAT_INTERVAL_MS", 10_000),
  };
}
