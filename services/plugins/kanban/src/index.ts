import Fastify from "fastify";
import Redis from "ioredis";
import { Pool } from "pg";
import { loadConfig } from "./config";
import { runMigrations } from "./db/schema";
import { TicketRepository } from "./db/tickets";
import { createEventBus } from "./events/bus";
import { logger } from "./logger";
import { RegistryClient } from "./registry-client";
import { registerTicketRoutes } from "./routes/tickets";

async function waitForDb(pool: Pool, attempts = 30): Promise<void> {
  for (let i = 1; i <= attempts; i++) {
    try {
      await pool.query("SELECT 1");
      return;
    } catch (err) {
      logger.warn(
        { attempt: i, err: (err as Error).message },
        "waiting for postgres",
      );
      await new Promise((r) => setTimeout(r, Math.min(1000 * i, 3000)));
    }
  }
  throw new Error("postgres unreachable");
}

async function main(): Promise<void> {
  const config = loadConfig();

  const pool = new Pool({ connectionString: config.databaseUrl });
  await waitForDb(pool);
  await runMigrations(pool);

  const redis = new Redis(config.redisUrl, { lazyConnect: false, maxRetriesPerRequest: null });
  redis.on("error", (err) => logger.error({ err }, "redis error"));

  const repo = new TicketRepository(pool);
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
    uptimeSeconds: Math.round(process.uptime()),
  }));

  await registerTicketRoutes(app, repo, bus, config.pluginId);

  await app.listen({ port: config.port, host: "0.0.0.0" });
  logger.info({ port: config.port }, "kanban plugin ready");

  const registry = new RegistryClient(config);
  // Fire and forget — service stays up even if registration is slow.
  registry.startWithRetry().catch((err) =>
    logger.error({ err }, "fatal registration failure"),
  );

  const shutdown = async (signal: string): Promise<void> => {
    logger.info({ signal }, "shutting down");
    try {
      await registry.deregister();
      await app.close();
      await bus.close();
      await redis.quit();
      await pool.end();
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
