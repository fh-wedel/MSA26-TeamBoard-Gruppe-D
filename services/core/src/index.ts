import Fastify from "fastify";
import cors from "@fastify/cors";
import Redis from "ioredis";
import { registerAdminRoutes } from "./api/admin-routes";
import { registerInternalRegistryRoutes, registerPublicPluginRoutes } from "./api/registry-routes";
import { createEventBus } from "./eventbus/factory";
import { loadConfig } from "./lib/config";
import { logger } from "./lib/logger";
import { registerProxyRoute } from "./proxy/proxy";
import { PluginRegistry } from "./registry/registry";

async function main(): Promise<void> {
  const config = loadConfig();
  const redis = new Redis(config.redisUrl, { lazyConnect: false, maxRetriesPerRequest: null });

  redis.on("error", (err) => logger.error({ err }, "redis error"));

  const registry = new PluginRegistry(redis, config.pluginRegistryTtl);
  const eventBus = createEventBus(config, redis);

  const isDev = process.env.NODE_ENV !== "production";
  const app = Fastify({
    logger: {
      level: process.env.LOG_LEVEL ?? "info",
      base: { service: "core" },
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

  await app.register(cors, {
    origin: (origin, cb) => {
      // Allow same-origin (no Origin header) and known dev origins.
      if (!origin) return cb(null, true);
      const allow = [
        "http://localhost:5173",
        "http://127.0.0.1:5173",
        "http://localhost:3000",
        "http://127.0.0.1:3000",
      ];
      if (allow.includes(origin)) return cb(null, true);
      // Permit any localhost origin during development to avoid friction.
      if (isDev && /^http:\/\/(localhost|127\.0\.0\.1)(:\d+)?$/.test(origin)) {
        return cb(null, true);
      }
      cb(new Error(`origin ${origin} not allowed`), false);
    },
    credentials: true,
  });

  app.get("/health", async () => ({
    status: "ok",
    service: "core",
    eventBus: config.eventBus,
    uptimeSeconds: Math.round(process.uptime()),
  }));

  await registerInternalRegistryRoutes(app, registry);
  await registerPublicPluginRoutes(app, registry);
  await registerAdminRoutes(app, eventBus);
  await registerProxyRoute(app, registry);

  app.decorate("eventBus", eventBus);

  const shutdown = async (signal: string): Promise<void> => {
    logger.info({ signal }, "shutting down");
    try {
      await app.close();
      await eventBus.close();
      await redis.quit();
    } catch (err) {
      logger.error({ err }, "error during shutdown");
    } finally {
      process.exit(0);
    }
  };
  process.on("SIGTERM", () => void shutdown("SIGTERM"));
  process.on("SIGINT", () => void shutdown("SIGINT"));

  await app.listen({ port: config.port, host: "0.0.0.0" });
  logger.info({ port: config.port, eventBus: config.eventBus }, "core service ready");
}

main().catch((err) => {
  logger.fatal({ err }, "core service failed to start");
  process.exit(1);
});
