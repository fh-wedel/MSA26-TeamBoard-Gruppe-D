import type { FastifyInstance } from "fastify";
import type { EventBus } from "../events/bus";
import type {
  CreateTicketInput,
  TicketRepository,
  UpdateTicketInput,
} from "../db/tickets";

interface TicketIdParams {
  id: string;
}

interface ListQuery {
  boardId?: string;
}

function isCreateInput(body: unknown): body is CreateTicketInput {
  if (!body || typeof body !== "object") return false;
  const b = body as Record<string, unknown>;
  return typeof b.boardId === "number" && typeof b.title === "string";
}

function isUpdateInput(body: unknown): body is UpdateTicketInput {
  if (!body || typeof body !== "object") return false;
  const b = body as Record<string, unknown>;
  const fields: (keyof UpdateTicketInput)[] = [
    "title",
    "description",
    "status",
    "position",
    "boardId",
  ];
  return fields.some((f) => f in b);
}

export async function registerTicketRoutes(
  app: FastifyInstance,
  repo: TicketRepository,
  bus: EventBus,
  pluginId: string,
): Promise<void> {
  app.get<{ Querystring: ListQuery }>("/tickets", async (request) => {
    const boardId = request.query.boardId
      ? Number(request.query.boardId)
      : undefined;
    if (boardId !== undefined && Number.isNaN(boardId)) {
      return { tickets: [] };
    }
    const tickets = await repo.list(boardId);
    return { tickets };
  });

  app.get<{ Params: TicketIdParams }>(
    "/tickets/:id",
    async (request, reply) => {
      const ticket = await repo.get(request.params.id);
      if (!ticket) return reply.status(404).send({ error: "ticket_not_found" });
      return ticket;
    },
  );

  app.post("/tickets", async (request, reply) => {
    if (!isCreateInput(request.body)) {
      return reply.status(400).send({ error: "invalid_payload" });
    }
    const ticket = await repo.create(request.body);
    await bus.publish({
      source: `plugin.${pluginId}`,
      detailType: "ticket.created",
      detail: { ticketId: ticket.id, boardId: ticket.boardId, status: ticket.status },
    });
    return reply.status(201).send(ticket);
  });

  app.put<{ Params: TicketIdParams }>(
    "/tickets/:id",
    async (request, reply) => {
      if (!isUpdateInput(request.body)) {
        return reply.status(400).send({ error: "invalid_payload" });
      }
      const result = await repo.update(request.params.id, request.body);
      if (!result) return reply.status(404).send({ error: "ticket_not_found" });
      if (result.before.status !== result.after.status) {
        await bus.publish({
          source: `plugin.${pluginId}`,
          detailType: "ticket.moved",
          detail: {
            ticketId: result.after.id,
            boardId: result.after.boardId,
            from: result.before.status,
            to: result.after.status,
          },
        });
      }
      return result.after;
    },
  );

  app.delete<{ Params: TicketIdParams }>(
    "/tickets/:id",
    async (request, reply) => {
      const removed = await repo.delete(request.params.id);
      if (!removed) return reply.status(404).send({ error: "ticket_not_found" });
      return reply.status(204).send();
    },
  );
}
