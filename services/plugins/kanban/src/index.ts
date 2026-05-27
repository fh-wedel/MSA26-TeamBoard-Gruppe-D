import Fastify from "fastify";
import Redis from "ioredis";
import { loadConfig } from "./config";
import { buildTicketRepository } from "./db/factory";
import { createEventBus } from "./events/bus";
import { logger } from "./logger";
import { RegistryClient } from "./registry-client";
import { registerTicketRoutes } from "./routes/tickets";

async function main(): Promise<void> {
  const config = loadConfig();

  const { repo, close: closeRepo } = await buildTicketRepository(config);

  const redis =
    config.eventBus === "redis"
      ? new Redis(config.redisUrl, { lazyConnect: false, maxRetriesPerRequest: null })
      : null;
  redis?.on("error", (err) => logger.error({ err }, "redis error"));

  const bus = createEventBus(config, redis);

  const isDev = process.env.NODE_ENV !== "production";
  const app = Fastify({
    logger: {
      level: process.env.LOG_LEVEL ?? "info",
      base: { service: "plugin-kanban" },
      ...(isDev
        ? {
            transport: {
              target: "pino-pretty",
              options: { colorize: true, translateTime: "SYS:standard" },
            },
          }
        : {}),
    },
    trustProxy: true,
  });

  app.get("/health", async () => ({
    status: "ok",
    pluginId: config.pluginId,
    version: config.pluginVersion,
    storage: config.storage,
    eventBus: config.eventBus,
    uptimeSeconds: Math.round(process.uptime()),
  }));

  await registerTicketRoutes(app, repo, bus, config.pluginId);

  await app.listen({ port: config.port, host: "0.0.0.0" });
  logger.info(
    { port: config.port, storage: config.storage, eventBus: config.eventBus },
    "kanban plugin ready",
  );

  const registry = new RegistryClient(config);
  registry.startWithRetry().catch((err) =>
    logger.error({ err }, "fatal registration failure"),
  );

  const shutdown = async (signal: string): Promise<void> => {
    logger.info({ signal }, "shutting down");
    try {
      await registry.deregister();
      await app.close();
      await bus.close();
      if (redis) await redis.quit();
      await closeRepo();
    } catch (err) {
      logger.error({ err }, "error during shutdown");
    } finally {
      process.exit(0);
    }
  };
  process.on("SIGTERM", () => void shutdown("SIGTERM"));
  process.on("SIGINT", () => void shutdown("SIGINT"));
}

main().catch((err) => {
  logger.fatal({ err }, "kanban plugin failed to start");
  process.exit(1);
});
