import type { Pool } from "pg";

export interface Ticket {
  id: number;
  boardId: number;
  title: string;
  description: string;
  status: string;
  position: number;
  createdAt: string;
  updatedAt: string;
}

interface TicketRow {
  id: number;
  board_id: number;
  title: string;
  description: string;
  status: string;
  position: number;
  created_at: Date;
  updated_at: Date;
}

function mapRow(row: TicketRow): Ticket {
  return {
    id: row.id,
    boardId: row.board_id,
    title: row.title,
    description: row.description,
    status: row.status,
    position: row.position,
    createdAt: row.created_at.toISOString(),
    updatedAt: row.updated_at.toISOString(),
  };
}

export interface CreateTicketInput {
  boardId: number;
  title: string;
  description?: string;
  status?: string;
  position?: number;
}

export interface UpdateTicketInput {
  title?: string;
  description?: string;
  status?: string;
  position?: number;
  boardId?: number;
}

export class TicketRepository {
  constructor(private readonly pool: Pool) {}

  async list(boardId?: number): Promise<Ticket[]> {
    if (boardId === undefined) {
      const { rows } = await this.pool.query<TicketRow>(
        "SELECT * FROM tickets ORDER BY board_id, status, position, id",
      );
      return rows.map(mapRow);
    }
    const { rows } = await this.pool.query<TicketRow>(
      "SELECT * FROM tickets WHERE board_id = $1 ORDER BY status, position, id",
      [boardId],
    );
    return rows.map(mapRow);
  }

  async get(id: number): Promise<Ticket | null> {
    const { rows } = await this.pool.query<TicketRow>(
      "SELECT * FROM tickets WHERE id = $1",
      [id],
    );
    if (rows.length === 0) return null;
    return mapRow(rows[0]!);
  }

  async create(input: CreateTicketInput): Promise<Ticket> {
    const { rows } = await this.pool.query<TicketRow>(
      `INSERT INTO tickets (board_id, title, description, status, position)
       VALUES ($1, $2, $3, $4, $5)
       RETURNING *`,
      [
        input.boardId,
        input.title,
        input.description ?? "",
        input.status ?? "todo",
        input.position ?? 0,
      ],
    );
    return mapRow(rows[0]!);
  }

  async update(
    id: number,
    input: UpdateTicketInput,
  ): Promise<{ before: Ticket; after: Ticket } | null> {
    const before = await this.get(id);
    if (!before) return null;
    const next = {
      title: input.title ?? before.title,
      description: input.description ?? before.description,
      status: input.status ?? before.status,
      position: input.position ?? before.position,
      boardId: input.boardId ?? before.boardId,
    };
    const { rows } = await this.pool.query<TicketRow>(
      `UPDATE tickets SET title=$1, description=$2, status=$3, position=$4, board_id=$5, updated_at=now()
       WHERE id=$6 RETURNING *`,
      [next.title, next.description, next.status, next.position, next.boardId, id],
    );
    return { before, after: mapRow(rows[0]!) };
  }

  async delete(id: number): Promise<boolean> {
    const { rowCount } = await this.pool.query("DELETE FROM tickets WHERE id = $1", [id]);
    return (rowCount ?? 0) > 0;
  }
}
