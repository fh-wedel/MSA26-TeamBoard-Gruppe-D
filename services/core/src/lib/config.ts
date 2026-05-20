export interface Config {
  port: number;
  redisUrl: string;
  eventBus: "redis" | "eventbridge";
  pluginRegistryTtl: number;
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
  return {
    port: num("CORE_PORT", 3000),
    redisUrl: process.env.REDIS_URL ?? "redis://localhost:6379",
    eventBus: eventBusRaw,
    pluginRegistryTtl: num("PLUGIN_REGISTRY_TTL", 30),
    awsRegion: process.env.AWS_REGION ?? "eu-central-1",
    eventBusName: process.env.EVENT_BUS_NAME ?? "msa2-bus",
  };
}
