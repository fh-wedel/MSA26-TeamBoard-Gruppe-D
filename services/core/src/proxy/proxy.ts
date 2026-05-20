import { request as undiciRequest } from "undici";
import type { FastifyInstance, FastifyReply, FastifyRequest } from "fastify";
import type { PluginRegistry } from "../registry/registry";

interface ProxyParams {
  pluginId: string;
  "*": string;
}

const HOP_BY_HOP = new Set([
  "connection",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "te",
  "trailer",
  "transfer-encoding",
  "upgrade",
  "host",
  "content-length",
]);

function filterHeaders(
  headers: Record<string, string | string[] | undefined>,
): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(headers)) {
    if (v === undefined) continue;
    if (HOP_BY_HOP.has(k.toLowerCase())) continue;
    out[k] = Array.isArray(v) ? v.join(", ") : v;
  }
  return out;
}

export async function registerProxyRoute(
  app: FastifyInstance,
  registry: PluginRegistry,
): Promise<void> {
  const handler = async (
    request: FastifyRequest<{ Params: ProxyParams }>,
    reply: FastifyReply,
  ): Promise<void> => {
    const { pluginId } = request.params;
    const remainder = request.params["*"] ?? "";
    const plugin = await registry.get(pluginId);
    if (!plugin) {
      reply.status(502).send({ error: "plugin_unavailable", pluginId });
      return;
    }
    const upstream = plugin.endpoint.replace(/\/$/, "");
    const search = request.url.includes("?")
      ? request.url.slice(request.url.indexOf("?"))
      : "";
    const target = `${upstream}/${remainder}${search}`;

    const headers = filterHeaders(
      request.headers as Record<string, string | string[] | undefined>,
    );
    headers["x-forwarded-host"] = request.hostname;
    headers["x-forwarded-proto"] = request.protocol;
    headers["x-forwarded-for"] = request.ip;
    headers["x-plugin-id"] = pluginId;

    const body =
      request.method === "GET" || request.method === "HEAD"
        ? undefined
        : (request.body as unknown);

    const upstreamResponse = await undiciRequest(target, {
      method: request.method as
        | "GET"
        | "POST"
        | "PUT"
        | "DELETE"
        | "PATCH"
        | "HEAD"
        | "OPTIONS",
      headers,
      body:
        body === undefined
          ? undefined
          : typeof body === "string" || Buffer.isBuffer(body)
            ? (body as string | Buffer)
            : JSON.stringify(body),
    });

    reply.status(upstreamResponse.statusCode);
    for (const [k, v] of Object.entries(upstreamResponse.headers)) {
      if (v === undefined) continue;
      if (HOP_BY_HOP.has(k.toLowerCase())) continue;
      reply.header(k, v as string | string[]);
    }
    // Buffer the upstream body so Fastify sets a correct content-length.
    // Plugin payloads are small; if we later need to stream, switch to
    // reply.raw + pipe.
    const buf = Buffer.from(await upstreamResponse.body.arrayBuffer());
    reply.send(buf);
  };

  app.route<{ Params: ProxyParams }>({
    method: ["GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"],
    url: "/api/plugins/:pluginId/proxy/*",
    handler,
  });
}
