import type { Redis } from "ioredis";
import type { Config } from "../lib/config";
import { EventBridgeBus } from "./eventbridge-bus";
import { RedisPubSubBus } from "./redis-bus";
import type { EventBus } from "./types";

export function createEventBus(config: Config, redis: Redis | null): EventBus {
  if (config.eventBus === "eventbridge") {
    return new EventBridgeBus(config.eventBusName, config.awsRegion);
  }
  if (!redis) {
    throw new Error("EVENT_BUS=redis but no Redis client provided");
  }
  return new RedisPubSubBus(redis);
}
