import Fastify from "fastify";
import websocketPlugin from "@fastify/websocket";
import Redis from "ioredis";
import type { WebSocket } from "ws";
import { DynamoDBClient } from "@aws-sdk/client-dynamodb";
import { DynamoConnectionStore, ensureTable } from "./connections";
import { logger } from "./logger";

const REDIS_EVENT_CHANNEL = "events.global";

interface LocalConfig {
  port: number;
  redisUrl: string;
  dynamoEndpoint: string;
  tableName: string;
  awsRegion: string;
}

function loadConfig(): LocalConfig {
  return {
    port: Number(process.env.BROADCASTER_PORT ?? 3002),
    redisUrl: process.env.REDIS_URL ?? "redis://redis:6379",
    dynamoEndpoint: process.env.DYNAMODB_ENDPOINT ?? "http://dynamodb-local:8000",
    tableName: process.env.CONNECTIONS_TABLE ?? "ws-connections",
    awsRegion: process.env.AWS_REGION ?? "eu-central-1",
  };
}

interface DomainEvent {
  source: string;
  detailType: string;
  detail: Record<string, unknown>;
}

interface DeliveredEvent {
  receivedAt: string;
  source: string;
  detailType: string;
  detail: Record<string, unknown>;
  recipientCount: number;
}

interface ClientState {
  socket: WebSocket;
  boardId: number | null;
}

async function waitForDynamo(client: DynamoDBClient, attempts = 30): Promise<void> {
  for (let i = 1; i <= attempts; i++) {
    try {
      await ensureTable(client, process.env.CONNECTIONS_TABLE ?? "ws-connections");
      return;
    } catch (err) {
      logger.warn({ attempt: i, err: (err as Error).message }, "waiting for dynamodb");
      await new Promise((r) => setTimeout(r, Math.min(1000 * i, 3000)));
    }
  }
  throw new Error("dynamodb-local unreachable");
}

function extractBoardId(detail: Record<string, unknown>): number | null {
  const v = detail.boardId;
  return typeof v === "number" ? v : null;
}

async function main(): Promise<void> {
  const cfg = loadConfig();

  const dynamoClient = new DynamoDBClient({
    region: cfg.awsRegion,
    endpoint: cfg.dynamoEndpoint,
    credentials: { accessKeyId: "local", secretAccessKey: "local" },
  });
  await waitForDynamo(dynamoClient);
  const store = new DynamoConnectionStore(dynamoClient, cfg.tableName);

  const subscriber = new Redis(cfg.redisUrl, { maxRetriesPerRequest: null });
  subscriber.on("error", (err) => logger.error({ err }, "redis subscriber error"));

  const recentEvents: DeliveredEvent[] = [];
  const MAX_RECENT = 100;
  const clients = new Set<ClientState>();

  const isDev = process.env.NODE_ENV !== "production";
  const app = Fastify({
    logger: {
      level: process.env.LOG_LEVEL ?? "info",
      base: { service: "ws-broadcaster" },
      ...(isDev
        ? {
            transport: {
              target: "pino-pretty",
              options: { colorize: true, translateTime: "SYS:standard" },
            },
          }
        : {}),
    },
  });

  await app.register(websocketPlugin);

  await subscriber.subscribe(REDIS_EVENT_CHANNEL);
  subscriber.on("message", async (_channel, message) => {
    let event: DomainEvent;
    try {
      event = JSON.parse(message) as DomainEvent;
    } catch (err) {
      logger.warn({ err, message }, "invalid event payload");
      return;
    }
    const detail = (event.detail ?? {}) as Record<string, unknown>;
    const boardId = extractBoardId(detail);

    let recipients = 0;
    if (boardId !== null) {
      try {
        const conns = await store.byBoard(boardId);
        recipients = conns.length;
      } catch (err) {
        logger.error({ err }, "failed to query connections");
      }
    }

    const delivered: DeliveredEvent = {
      receivedAt: new Date().toISOString(),
      source: event.source,
      detailType: event.detailType,
      detail,
      recipientCount: recipients,
    };
    recentEvents.unshift(delivered);
    if (recentEvents.length > MAX_RECENT) recentEvents.length = MAX_RECENT;

    let wsRecipients = 0;
    const payload = JSON.stringify({
      type: "event",
      receivedAt: delivered.receivedAt,
      source: event.source,
      detailType: event.detailType,
      detail,
    });
    for (const client of clients) {
      if (client.socket.readyState !== client.socket.OPEN) continue;
      if (client.boardId !== null && boardId !== null && client.boardId !== boardId) {
        continue;
      }
      try {
        client.socket.send(payload);
        wsRecipients++;
      } catch (err) {
        logger.warn({ err }, "ws send failed");
      }
    }

    logger.info(
      {
        source: event.source,
        detailType: event.detailType,
        dynamoRecipients: recipients,
        wsRecipients,
      },
      "event received",
    );
  });

  app.get("/health", async () => ({
    status: "ok",
    service: "ws-broadcaster",
    subscribedTo: REDIS_EVENT_CHANNEL,
    wsClients: clients.size,
    uptimeSeconds: Math.round(process.uptime()),
  }));

  app.get("/events/recent", async () => ({ events: recentEvents }));

  app.get("/ws", { websocket: true }, (socket, request) => {
    const state: ClientState = { socket, boardId: null };
    clients.add(state);
    request.log.info({ clientCount: clients.size }, "ws client connected");

    try {
      socket.send(
        JSON.stringify({
          type: "hello",
          service: "ws-broadcaster",
          time: new Date().toISOString(),
        }),
      );
    } catch (err) {
      request.log.warn({ err }, "ws initial send failed");
    }

    socket.on("message", (raw: Buffer) => {
      let parsed: unknown;
      try {
        parsed = JSON.parse(raw.toString("utf8"));
      } catch {
        return;
      }
      if (parsed && typeof parsed === "object") {
        const obj = parsed as Record<string, unknown>;
        if (typeof obj.boardId === "number") {
          state.boardId = obj.boardId;
          request.log.info({ boardId: state.boardId }, "client board filter set");
        } else if (obj.boardId === null) {
          state.boardId = null;
        }
      }
    });

    const onClose = (): void => {
      clients.delete(state);
      request.log.info({ clientCount: clients.size }, "ws client disconnected");
    };
    socket.on("close", onClose);
    socket.on("error", (err: Error) => {
      request.log.warn({ err }, "ws client error");
      onClose();
    });
  });

  const shutdown = async (signal: string): Promise<void> => {
    logger.info({ signal }, "shutting down");
    try {
      for (const client of clients) {
        try {
          client.socket.close();
        } catch {
          // ignore
        }
      }
      clients.clear();
      await app.close();
      await subscriber.quit();
      dynamoClient.destroy();
    } catch (err) {
      logger.error({ err }, "error during shutdown");
    } finally {
      process.exit(0);
    }
  };
  process.on("SIGTERM", () => void shutdown("SIGTERM"));
  process.on("SIGINT", () => void shutdown("SIGINT"));

  await app.listen({ port: cfg.port, host: "0.0.0.0" });
  logger.info({ port: cfg.port }, "ws-broadcaster (local) ready");
}

main().catch((err) => {
  logger.fatal({ err }, "ws-broadcaster failed to start");
  process.exit(1);
});
