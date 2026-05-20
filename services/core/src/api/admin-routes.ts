import type { FastifyInstance } from "fastify";
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

function buildArchitecture(): ArchitectureGraph {
  const broadcasterEndpoint =
    process.env.BROADCASTER_URL ?? "http://ws-broadcaster:3002";
  return {
    components: [
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
        id: "eventbridge",
        label: "Event Bus",
        awsService: "Amazon EventBridge (Redis Pub/Sub lokal)",
        category: "messaging",
        endpoint: null,
        status: "running",
        description: "Domain events flow through here. Local: Redis Pub/Sub channel events.global.",
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
        id: "rds",
        label: "Primary DB",
        awsService: "RDS PostgreSQL (Aurora Serverless v2)",
        category: "persistence",
        endpoint: "postgres:5432",
        status: "running",
        description: "Per-plugin schemas; kanban tickets live here.",
      },
      {
        id: "redis",
        label: "Cache / Registry",
        awsService: "ElastiCache Redis",
        category: "persistence",
        endpoint: "redis:6379",
        status: "running",
        description: "Plugin registry storage and local Pub/Sub bus.",
      },
      {
        id: "dynamodb",
        label: "Connection State",
        awsService: "DynamoDB",
        category: "persistence",
        endpoint: "dynamodb-local:8000",
        status: "running",
        description: "WebSocket connection lookup table.",
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
    ],
    edges: [
      { from: "client", to: "api-gateway-rest", label: "HTTPS" },
      { from: "client", to: "api-gateway-ws", label: "WSS" },
      { from: "api-gateway-rest", to: "core", label: "REST" },
      { from: "api-gateway-ws", to: "broadcaster", label: "WS" },
      { from: "core", to: "redis", label: "registry" },
      { from: "core", to: "eventbridge", label: "publish/subscribe" },
      { from: "eventbridge", to: "broadcaster", label: "ticket.*" },
      { from: "broadcaster", to: "dynamodb", label: "lookup" },
      { from: "broadcaster", to: "api-gateway-ws", label: "@connections" },
      { from: "core", to: "rds", label: "metadata" },
    ],
  };
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
): Promise<void> {
  app.get("/api/admin/architecture", async () => buildArchitecture());

  app.get("/api/admin/events/stream", (request, reply) => {
    if (!isRedisBus(bus)) {
      void reply.status(503).send({ error: "event_stream_not_available" });
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
