import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  DndContext,
  PointerSensor,
  useDraggable,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import { api } from "../api/client";
import type { Ticket, TicketStatus } from "../api/types";
import { useWebSocket, type WsStatus } from "../realtime/useWebSocket";

const COLUMNS: { id: TicketStatus; title: string }[] = [
  { id: "todo", title: "To Do" },
  { id: "in-progress", title: "In Progress" },
  { id: "done", title: "Done" },
];

const DEFAULT_BOARD_ID = 1;

interface KanbanBoardProps {
  boardId?: number;
  wsUrl: string;
}

function StatusDot({ status }: { status: WsStatus }): JSX.Element {
  const cls =
    status === "open"
      ? "bg-emerald-500"
      : status === "connecting"
        ? "bg-yellow-500 animate-pulse"
        : "bg-rose-500";
  const label =
    status === "open" ? "Connected" : status === "connecting" ? "Connecting…" : "Disconnected";
  return (
    <span className="inline-flex items-center gap-2 text-xs text-slate-400">
      <span className={`status-dot ${cls}`} aria-hidden />
      {label}
    </span>
  );
}

function TicketCard({ ticket }: { ticket: Ticket }): JSX.Element {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: `ticket:${ticket.id}`,
    data: { ticketId: ticket.id, status: ticket.status },
  });
  const style = transform
    ? {
        transform: `translate3d(${transform.x}px, ${transform.y}px, 0)`,
      }
    : undefined;
  return (
    <div
      ref={setNodeRef}
      style={style}
      {...attributes}
      {...listeners}
      className={`select-none rounded-md border border-slate-800 bg-slate-900 p-3 shadow-sm cursor-grab active:cursor-grabbing ${
        isDragging ? "opacity-60 ring-2 ring-indigo-500" : ""
      }`}
    >
      <div className="text-sm font-medium text-slate-100">{ticket.title}</div>
      {ticket.description && (
        <div className="mt-1 text-xs text-slate-400 line-clamp-3">{ticket.description}</div>
      )}
      <div className="mt-2 flex items-center justify-between text-[10px] text-slate-500">
        <span>#{ticket.id.slice(0, 8)}</span>
        <span>board {ticket.boardId}</span>
      </div>
    </div>
  );
}

interface ColumnProps {
  status: TicketStatus;
  title: string;
  tickets: Ticket[];
  onAdd: (status: TicketStatus) => void;
}

function Column({ status, title, tickets, onAdd }: ColumnProps): JSX.Element {
  const { setNodeRef, isOver } = useDroppable({ id: `col:${status}` });
  return (
    <div
      ref={setNodeRef}
      className={`flex h-full flex-col rounded-lg border ${
        isOver ? "border-indigo-500/70 bg-indigo-500/5" : "border-slate-800 bg-slate-900/40"
      } p-3`}
    >
      <div className="mb-2 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-300">
            {title}
          </h3>
          <span className="rounded-full bg-slate-800 px-2 py-0.5 text-[10px] text-slate-400">
            {tickets.length}
          </span>
        </div>
        <button
          type="button"
          className="btn-ghost text-xs"
          onClick={() => onAdd(status)}
          aria-label={`Neues Ticket in ${title}`}
        >
          + Neu
        </button>
      </div>
      <div className="flex-1 space-y-2 overflow-auto pr-1">
        {tickets.map((t) => (
          <TicketCard key={t.id} ticket={t} />
        ))}
        {tickets.length === 0 && (
          <div className="rounded border border-dashed border-slate-800 p-3 text-center text-xs text-slate-500">
            Hier könnten Tickets stehen.
          </div>
        )}
      </div>
    </div>
  );
}

export default function KanbanBoard(props: KanbanBoardProps): JSX.Element {
  const boardId = props.boardId ?? DEFAULT_BOARD_ID;
  const [tickets, setTickets] = useState<Ticket[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }));
  const reloadTokenRef = useRef(0);

  const reload = useCallback(async (): Promise<void> => {
    const token = ++reloadTokenRef.current;
    try {
      const res = await api.listTickets(boardId);
      if (token !== reloadTokenRef.current) return;
      setTickets(res.tickets);
      setError(null);
    } catch (err) {
      if (token !== reloadTokenRef.current) return;
      setError((err as Error).message);
    } finally {
      if (token === reloadTokenRef.current) setLoading(false);
    }
  }, [boardId]);

  useEffect(() => {
    void reload();
  }, [reload]);

  const handleWsMessage = useCallback(
    (data: unknown): void => {
      if (!data || typeof data !== "object") return;
      const msg = data as Record<string, unknown>;
      if (msg.type !== "event") return;
      const detailType = typeof msg.detailType === "string" ? msg.detailType : "";
      if (!detailType.startsWith("ticket.")) return;
      const detail = (msg.detail ?? {}) as Record<string, unknown>;
      // Only react to events for our board (broadcaster already filters when
      // a boardId was set, but be defensive).
      if (typeof detail.boardId === "number" && detail.boardId !== boardId) return;
      void reload();
    },
    [boardId, reload],
  );

  const { status: wsStatus } = useWebSocket({
    url: props.wsUrl,
    boardId,
    onMessage: handleWsMessage,
  });

  const byColumn = useMemo(() => {
    const map: Record<TicketStatus, Ticket[]> = {
      todo: [],
      "in-progress": [],
      done: [],
    };
    for (const t of tickets) {
      const s = (t.status as TicketStatus) ?? "todo";
      if (s in map) map[s].push(t);
      else map.todo.push(t);
    }
    return map;
  }, [tickets]);

  const onDragEnd = useCallback(
    async (e: DragEndEvent) => {
      const overId = e.over?.id;
      if (typeof overId !== "string" || !overId.startsWith("col:")) return;
      const newStatus = overId.slice("col:".length) as TicketStatus;
      const data = e.active.data.current as { ticketId?: string; status?: TicketStatus } | undefined;
      if (!data?.ticketId || data.status === newStatus) return;

      // Optimistic update
      setTickets((prev) =>
        prev.map((t) => (t.id === data.ticketId ? { ...t, status: newStatus } : t)),
      );
      try {
        await api.updateTicket(data.ticketId, { status: newStatus });
      } catch (err) {
        setError((err as Error).message);
        void reload();
      }
    },
    [reload],
  );

  const onAdd = useCallback(
    async (status: TicketStatus): Promise<void> => {
      const title = window.prompt(`Titel für neues Ticket in "${status}":`);
      if (!title) return;
      try {
        await api.createTicket({ boardId, title, status });
        await reload();
      } catch (err) {
        setError((err as Error).message);
      }
    },
    [boardId, reload],
  );

  return (
    <div className="flex h-full flex-col">
      <div className="mb-3 flex items-center justify-between">
        <div>
          <h2 className="text-base font-semibold text-slate-100">Kanban Board</h2>
          <p className="text-xs text-slate-500">
            Plugin: <code className="text-slate-400">kanban-board</code> · Board #{boardId}
          </p>
        </div>
        <StatusDot status={wsStatus} />
      </div>

      {loading && tickets.length === 0 && (
        <p className="text-sm text-slate-400">Tickets werden geladen…</p>
      )}
      {error && (
        <p className="mb-2 rounded border border-rose-700/40 bg-rose-950/40 p-2 text-xs text-rose-300">
          {error}
        </p>
      )}

      <DndContext sensors={sensors} onDragEnd={onDragEnd}>
        <div className="grid flex-1 grid-cols-1 gap-4 md:grid-cols-3 min-h-0">
          {COLUMNS.map((col) => (
            <Column
              key={col.id}
              status={col.id}
              title={col.title}
              tickets={byColumn[col.id]}
              onAdd={onAdd}
            />
          ))}
        </div>
      </DndContext>
    </div>
  );
}
