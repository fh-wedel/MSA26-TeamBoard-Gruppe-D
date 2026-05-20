import type { Pool } from "pg";

export const SCHEMA_SQL = `
CREATE TABLE IF NOT EXISTS tickets (
  id           SERIAL PRIMARY KEY,
  board_id     INTEGER NOT NULL,
  title        TEXT NOT NULL,
  description  TEXT NOT NULL DEFAULT '',
  status       TEXT NOT NULL DEFAULT 'todo',
  position     INTEGER NOT NULL DEFAULT 0,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS tickets_board_idx ON tickets (board_id);
CREATE INDEX IF NOT EXISTS tickets_status_idx ON tickets (status);
`;

export async function runMigrations(pool: Pool): Promise<void> {
  await pool.query(SCHEMA_SQL);
}
