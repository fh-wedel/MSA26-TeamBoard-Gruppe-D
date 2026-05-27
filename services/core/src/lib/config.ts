export type RegistryBackend = "redis" | "dynamodb";
export type EventBusBackend = "redis" | "eventbridge";

export interface Config {
  port: number;
  redisUrl: string;
  eventBus: EventBusBackend;
  registryBackend: RegistryBackend;
  pluginRegistryTtl: number;
  pluginRegistryTable: string;
  dynamoEndpoint?: string;
  awsRegion: string;
  eventBusName: string;
}

function num(name: string, fallback: number): number {
  const raw = process.env[name];
  if (!raw) return fallback;
  const parsed = Number(raw);
  if (Number.isNaN(parsed)) {
    throw new Error(`Env var ${name} must be a number, got ${raw}`);
  }
  return parsed;
}

export function loadConfig(): Config {
  const eventBusRaw = (process.env.EVENT_BUS ?? "redis").toLowerCase();
  if (eventBusRaw !== "redis" && eventBusRaw !== "eventbridge") {
    throw new Error(`EVENT_BUS must be "redis" or "eventbridge", got: ${eventBusRaw}`);
  }
  const registryRaw = (process.env.REGISTRY_BACKEND ?? "redis").toLowerCase();
  if (registryRaw !== "redis" && registryRaw !== "dynamodb") {
    throw new Error(`REGISTRY_BACKEND must be "redis" or "dynamodb", got: ${registryRaw}`);
  }
  return {
    port: num("CORE_PORT", 3000),
    redisUrl: process.env.REDIS_URL ?? "redis://localhost:6379",
    eventBus: eventBusRaw,
    registryBackend: registryRaw,
    pluginRegistryTtl: num("PLUGIN_REGISTRY_TTL", 30),
    pluginRegistryTable: process.env.PLUGIN_REGISTRY_TABLE ?? "msa2-plugin-registry",
    ...(process.env.DYNAMO_ENDPOINT
      ? { dynamoEndpoint: process.env.DYNAMO_ENDPOINT }
      : {}),
    awsRegion: process.env.AWS_REGION ?? "eu-central-1",
    eventBusName: process.env.EVENT_BUS_NAME ?? "msa2-bus",
  };
}
