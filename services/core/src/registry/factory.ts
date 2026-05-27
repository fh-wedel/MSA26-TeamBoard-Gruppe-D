import type { Redis } from "ioredis";
import type { Config } from "../lib/config";
import { logger } from "../lib/logger";
import { DynamoPluginRegistry } from "./dynamo-registry";
import { RedisPluginRegistry, type PluginRegistry } from "./registry";

export function createPluginRegistry(
  config: Config,
  redis: Redis | null,
): PluginRegistry {
  if (config.registryBackend === "dynamodb") {
    logger.info(
      { table: config.pluginRegistryTable },
      "using DynamoDB plugin registry",
    );
    return new DynamoPluginRegistry({
      tableName: config.pluginRegistryTable,
      region: config.awsRegion,
      ttlSeconds: config.pluginRegistryTtl,
      ...(config.dynamoEndpoint ? { endpoint: config.dynamoEndpoint } : {}),
    });
  }
  if (!redis) {
    throw new Error("REGISTRY_BACKEND=redis but no Redis client provided");
  }
  logger.info("using Redis plugin registry");
  return new RedisPluginRegistry(redis, config.pluginRegistryTtl);
}
