import type { FastifyInstance } from "fastify";
import type { Config } from "../lib/config";
import { RedisPubSubBus } from "../eventbus/redis-bus";
import type { EventBus, DomainEvent } from "../eventbus/types";

export interface ArchitectureComponent {
  id: string;
  label: string;
  awsService: string;
  category:
    | "client"
    | "edge"
    | "compute"
    | "messaging"
    | "persistence";
  endpoint: string | null;
  status: "static" | "running" | "down";
  description?: string;
}

export interface ArchitectureEdge {
  from: string;
  to: string;
  label: string;
}

export interface ArchitectureGraph {
  components: ArchitectureComponent[];
  edges: ArchitectureEdge[];
}

function buildArchitecture(config: Config): ArchitectureGraph {
  const broadcasterEndpoint =
    process.env.BROADCASTER_URL ?? "http://ws-broadcaster:3002";
  const usingDynamo = config.registryBackend === "dynamodb";
  const usingEventBridge = config.eventBus === "eventbridge";

  const components: ArchitectureComponent[] = [
    {
      id: "client",
      label: "Client (Browser)",
      awsService: "—",
      category: "client",
      endpoint: null,
      status: "static",
      description: "User browser running the Vite + React frontend.",
    },
    {
      id: "api-gateway-rest",
      label: "API Gateway REST",
      awsService: "API Gateway",
      category: "edge",
      endpoint: null,
      status: "static",
      description: "Public REST entry point in AWS. Locally bypassed; client calls Core directly.",
    },
    {
      id: "api-gateway-ws",
      label: "API Gateway WebSocket",
      awsService: "API Gateway WebSocket API",
      category: "edge",
      endpoint: null,
      status: "static",
      description: "WebSocket entry point in AWS. Locally the WS Broadcaster serves /ws directly.",
    },
    {
      id: "core",
      label: "Core API",
      awsService: "ECS Fargate",
      category: "compute",
      endpoint: "http://core:3000",
      status: "running",
      description: "Plugin registry, reverse proxy, event bus owner.",
    },
    {
      id: "eventbus",
      label: "Event Bus",
      awsService: usingEventBridge ? "Amazon EventBridge" : "Redis Pub/Sub (lokal)",
      category: "messaging",
      endpoint: null,
      status: "running",
      description: usingEventBridge
        ? "Domain events on the msa2-bus event bus. Pub/Sub via EventBridge rules."
        : "Local mode: Redis Pub/Sub channel events.global.",
    },
    {
      id: "broadcaster",
      label: "WebSocket Broadcaster",
      awsService: "AWS Lambda",
      category: "compute",
      endpoint: broadcasterEndpoint,
      status: "running",
      description: "Fans out events to connected WebSocket clients.",
    },
    {
      id: "registry-store",
      label: "Plugin Registry",
      awsService: usingDynamo ? "DynamoDB (TTL)" : "Redis (TTL)",
      category: "persistence",
      endpoint: usingDynamo ? null : "redis:6379",
      status: "running",
      description: usingDynamo
        ? "DynamoDB table holds active plugin registrations; TTL attribute drops stale entries automatically."
        : "Local mode: Redis stores registry entries with EX-based TTL.",
    },
    {
      id: "tickets-store",
      label: "Kanban Tickets",
      awsService: usingDynamo ? "DynamoDB" : "PostgreSQL (lokal)",
      category: "persistence",
      endpoint: usingDynamo ? null : "postgres:5432",
      status: "running",
      description: usingDynamo
        ? "DynamoDB table msa2-tickets, partition key pk=BOARD#<id>, sort key sk=TICKET#<uuid>."
        : "Local mode: Postgres tickets table.",
    },
    {
      id: "connections",
      label: "Connection State",
      awsService: "DynamoDB",
      category: "persistence",
      endpoint: usingDynamo ? null : "dynamodb-local:8000",
      status: "running",
      description: "WebSocket connection lookup table for the broadcaster.",
    },
    {
      id: "s3",
      label: "Blob Storage",
      awsService: "S3 (MinIO lokal)",
      category: "persistence",
      endpoint: "minio:9000",
      status: "running",
      description: "Object storage for plugin attachments.",
    },
  ];

  const edges: ArchitectureEdge[] = [
    { from: "client", to: "api-gateway-rest", label: "HTTPS" },
    { from: "client", to: "api-gateway-ws", label: "WSS" },
    { from: "api-gateway-rest", to: "core", label: "REST" },
    { from: "api-gateway-ws", to: "broadcaster", label: "WS" },
    { from: "core", to: "registry-store", label: "register / heartbeat" },
    { from: "core", to: "eventbus", label: "publish" },
    { from: "eventbus", to: "broadcaster", label: "ticket.*" },
    { from: "broadcaster", to: "connections", label: "lookup" },
    { from: "broadcaster", to: "api-gateway-ws", label: "@connections" },
    { from: "core", to: "tickets-store", label: "(via kanban proxy)" },
  ];

  return { components, edges };
}

interface SseEvent {
  receivedAt: string;
  source: string;
  detailType: string;
  detail: Record<string, unknown>;
}

function isRedisBus(bus: EventBus): bus is RedisPubSubBus {
  return bus instanceof RedisPubSubBus;
}

export async function registerAdminRoutes(
  app: FastifyInstance,
  bus: EventBus,
  config: Config,
): Promise<void> {
  app.get("/api/admin/architecture", async () => buildArchitecture(config));

  app.get("/api/admin/events/stream", (request, reply) => {
    if (!isRedisBus(bus)) {
      void reply.status(503).send({
        error: "event_stream_not_available",
        reason:
          "Live event SSE is only wired for the Redis bus. In AWS, events flow through EventBridge to Lambda — set up an EventBridge → Lambda relay to stream them back here.",
      });
      return;
    }

    const raw = reply.raw;
    raw.writeHead(200, {
      "content-type": "text/event-stream",
      "cache-control": "no-cache, no-transform",
      connection: "keep-alive",
      "x-accel-buffering": "no",
    });
    raw.write(`retry: 5000\n\n`);

    const send = (event: DomainEvent): void => {
      const payload: SseEvent = {
        receivedAt: new Date().toISOString(),
        source: event.source,
        detailType: event.detailType,
        detail: event.detail ?? {},
      };
      try {
        raw.write(`data: ${JSON.stringify(payload)}\n\n`);
      } catch {
        // client gone; cleanup handled by close listener
      }
    };

    const unsubscribe = bus.tap(send);
    const keepalive = setInterval(() => {
      try {
        raw.write(`: keepalive ${Date.now()}\n\n`);
      } catch {
        // ignored
      }
    }, 15_000);

    const cleanup = (): void => {
      clearInterval(keepalive);
      unsubscribe();
    };

    request.raw.on("close", cleanup);
    request.raw.on("error", cleanup);
  });
}
