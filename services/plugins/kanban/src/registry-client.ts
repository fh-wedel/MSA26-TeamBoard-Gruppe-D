import { request } from "undici";
import type { KanbanConfig } from "./config";
import { logger } from "./logger";

interface RegistrationPayload {
  pluginId: string;
  version: string;
  capabilities: string[];
  endpoint: string;
  eventSubscriptions: string[];
  healthCheck: string;
  runtime: string;
  awsService: string;
  description: string;
}

export class RegistryClient {
  private heartbeatTimer: NodeJS.Timeout | null = null;
  private stopped = false;

  constructor(private readonly config: KanbanConfig) {}

  private get registrationPayload(): RegistrationPayload {
    return {
      pluginId: this.config.pluginId,
      version: this.config.pluginVersion,
      capabilities: ["read", "write", "realtime"],
      endpoint: this.config.publicEndpoint,
      eventSubscriptions: ["ticket.moved", "ticket.assigned"],
      healthCheck: "/health",
      runtime: process.env.PLUGIN_RUNTIME ?? "ecs-fargate",
      awsService: process.env.PLUGIN_AWS_SERVICE ?? "ECS Fargate",
      description:
        process.env.PLUGIN_DESCRIPTION ??
        "Kanban Board Plugin — Tickets in Postgres, Echtzeit via EventBridge",
    };
  }

  async register(): Promise<void> {
    const url = `${this.config.coreUrl}/internal/registry/register`;
    const res = await request(url, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(this.registrationPayload),
    });
    if (res.statusCode >= 300) {
      const body = await res.body.text();
      throw new Error(
        `registration failed: HTTP ${res.statusCode} body=${body.slice(0, 200)}`,
      );
    }
    await res.body.dump();
    logger.info({ pluginId: this.config.pluginId }, "registered with core");
  }

  async heartbeat(): Promise<void> {
    const url = `${this.config.coreUrl}/internal/registry/heartbeat/${encodeURIComponent(
      this.config.pluginId,
    )}`;
    const res = await request(url, { method: "PUT" });
    if (res.statusCode === 404) {
      await res.body.dump();
      logger.warn("heartbeat returned 404 — re-registering");
      await this.register();
      return;
    }
    if (res.statusCode >= 300) {
      const body = await res.body.text();
      throw new Error(`heartbeat failed: HTTP ${res.statusCode} body=${body.slice(0, 200)}`);
    }
    await res.body.dump();
  }

  async deregister(): Promise<void> {
    this.stopped = true;
    if (this.heartbeatTimer) clearInterval(this.heartbeatTimer);
    const url = `${this.config.coreUrl}/internal/registry/deregister/${encodeURIComponent(
      this.config.pluginId,
    )}`;
    try {
      const res = await request(url, { method: "DELETE" });
      await res.body.dump();
      logger.info("deregistered from core");
    } catch (err) {
      logger.warn({ err }, "deregistration failed (ignored)");
    }
  }

  async startWithRetry(maxAttempts = 30): Promise<void> {
    for (let attempt = 1; attempt <= maxAttempts; attempt++) {
      if (this.stopped) return;
      try {
        await this.register();
        this.heartbeatTimer = setInterval(() => {
          this.heartbeat().catch((err) =>
            logger.error({ err }, "heartbeat error"),
          );
        }, this.config.heartbeatIntervalMs);
        return;
      } catch (err) {
        logger.warn(
          { attempt, err: (err as Error).message },
          "registration attempt failed, retrying",
        );
        await new Promise((r) => setTimeout(r, Math.min(1000 * attempt, 5000)));
      }
    }
    throw new Error(`could not register with core after ${maxAttempts} attempts`);
  }
}
