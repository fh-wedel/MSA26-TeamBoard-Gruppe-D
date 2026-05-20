import type { FastifyInstance } from "fastify";
import type { PluginRegistry } from "../registry/registry";
import type { PluginRegistration } from "../registry/types";

interface PluginIdParams {
  pluginId: string;
}

function isOptionalString(value: unknown): boolean {
  return value === undefined || typeof value === "string";
}

function isValidRegistration(body: unknown): body is PluginRegistration {
  if (!body || typeof body !== "object") return false;
  const b = body as Record<string, unknown>;
  return (
    typeof b.pluginId === "string" &&
    typeof b.version === "string" &&
    Array.isArray(b.capabilities) &&
    typeof b.endpoint === "string" &&
    Array.isArray(b.eventSubscriptions) &&
    typeof b.healthCheck === "string" &&
    isOptionalString(b.runtime) &&
    isOptionalString(b.awsService) &&
    isOptionalString(b.description)
  );
}

function sanitizeRegistration(body: PluginRegistration): PluginRegistration {
  const out: PluginRegistration = {
    pluginId: body.pluginId,
    version: body.version,
    capabilities: body.capabilities,
    endpoint: body.endpoint,
    eventSubscriptions: body.eventSubscriptions,
    healthCheck: body.healthCheck,
  };
  if (body.runtime !== undefined) out.runtime = body.runtime;
  if (body.awsService !== undefined) out.awsService = body.awsService;
  if (body.description !== undefined) out.description = body.description;
  return out;
}

export async function registerInternalRegistryRoutes(
  app: FastifyInstance,
  registry: PluginRegistry,
): Promise<void> {
  app.post("/internal/registry/register", async (request, reply) => {
    if (!isValidRegistration(request.body)) {
      return reply.status(400).send({ error: "invalid_registration_payload" });
    }
    const stored = await registry.register(sanitizeRegistration(request.body));
    request.log.info({ pluginId: stored.pluginId }, "plugin registered");
    return reply.status(201).send(stored);
  });

  app.put<{ Params: PluginIdParams }>(
    "/internal/registry/heartbeat/:pluginId",
    async (request, reply) => {
      const updated = await registry.heartbeat(request.params.pluginId);
      if (!updated) {
        return reply.status(404).send({ error: "plugin_not_registered" });
      }
      return reply.send(updated);
    },
  );

  app.delete<{ Params: PluginIdParams }>(
    "/internal/registry/deregister/:pluginId",
    async (request, reply) => {
      const removed = await registry.deregister(request.params.pluginId);
      if (!removed) {
        return reply.status(404).send({ error: "plugin_not_registered" });
      }
      request.log.info(
        { pluginId: request.params.pluginId },
        "plugin deregistered",
      );
      return reply.status(204).send();
    },
  );
}

export async function registerPublicPluginRoutes(
  app: FastifyInstance,
  registry: PluginRegistry,
): Promise<void> {
  app.get("/api/plugins", async () => {
    const plugins = await registry.list();
    return { plugins };
  });

  app.get<{ Params: PluginIdParams }>(
    "/api/plugins/:pluginId",
    async (request, reply) => {
      const plugin = await registry.get(request.params.pluginId);
      if (!plugin) {
        return reply.status(404).send({ error: "plugin_not_found" });
      }
      return plugin;
    },
  );
}
