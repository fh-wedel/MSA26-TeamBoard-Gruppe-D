import type {
  ArchitectureGraph,
  PluginListResponse,
  Ticket,
  TicketListResponse,
  TicketStatus,
} from "./types";

async function jsonRequest<T>(input: string, init?: RequestInit): Promise<T> {
  const response = await fetch(input, {
    ...init,
    headers: {
      accept: "application/json",
      ...(init?.body ? { "content-type": "application/json" } : {}),
      ...(init?.headers ?? {}),
    },
  });
  if (!response.ok) {
    const text = await response.text().catch(() => "");
    throw new Error(`${response.status} ${response.statusText} — ${text.slice(0, 200)}`);
  }
  if (response.status === 204) return undefined as T;
  return (await response.json()) as T;
}

export const api = {
  listPlugins: (): Promise<PluginListResponse> => jsonRequest("/api/plugins"),

  listTickets: (boardId?: number): Promise<TicketListResponse> => {
    const qs = boardId !== undefined ? `?boardId=${boardId}` : "";
    return jsonRequest(`/api/plugins/kanban-board/proxy/tickets${qs}`);
  },

  createTicket: (input: {
    boardId: number;
    title: string;
    status?: TicketStatus;
    description?: string | null;
  }): Promise<Ticket> =>
    jsonRequest("/api/plugins/kanban-board/proxy/tickets", {
      method: "POST",
      body: JSON.stringify(input),
    }),

  updateTicket: (
    id: number,
    patch: Partial<Pick<Ticket, "status" | "title" | "description" | "position" | "boardId">>,
  ): Promise<Ticket> =>
    jsonRequest(`/api/plugins/kanban-board/proxy/tickets/${id}`, {
      method: "PUT",
      body: JSON.stringify(patch),
    }),

  deleteTicket: (id: number): Promise<void> =>
    jsonRequest(`/api/plugins/kanban-board/proxy/tickets/${id}`, { method: "DELETE" }),

  getArchitecture: (): Promise<ArchitectureGraph> => jsonRequest("/api/admin/architecture"),
};
