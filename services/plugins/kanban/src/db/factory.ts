import { Pool } from "pg";
import type { KanbanConfig } from "../config";
import { logger } from "../logger";
import { runMigrations } from "./schema";
import {
  DynamoTicketRepository,
  type DynamoTicketRepositoryOptions,
} from "./tickets-dynamo";
import { PostgresTicketRepository, type TicketRepository } from "./tickets";

export interface RepositoryHandle {
  repo: TicketRepository;
  close(): Promise<void>;
}

async function waitForDb(pool: Pool, attempts = 30): Promise<void> {
  for (let i = 1; i <= attempts; i++) {
    try {
      await pool.query("SELECT 1");
      return;
    } catch (err) {
      logger.warn(
        { attempt: i, err: (err as Error).message },
        "waiting for postgres",
      );
      await new Promise((r) => setTimeout(r, Math.min(1000 * i, 3000)));
    }
  }
  throw new Error("postgres unreachable");
}

export async function buildTicketRepository(
  config: KanbanConfig,
): Promise<RepositoryHandle> {
  if (config.storage === "dynamodb") {
    const opts: DynamoTicketRepositoryOptions = {
      tableName: config.ticketsTable,
      region: config.awsRegion,
      ...(config.dynamoEndpoint ? { endpoint: config.dynamoEndpoint } : {}),
    };
    const repo = new DynamoTicketRepository(opts);
    logger.info({ table: opts.tableName }, "using DynamoDB ticket store");
    return {
      repo,
      async close() {
        /* DynamoDBClient cleans up via process exit */
      },
    };
  }

  const pool = new Pool({ connectionString: config.databaseUrl });
  await waitForDb(pool);
  await runMigrations(pool);
  const repo = new PostgresTicketRepository(pool);
  logger.info("using Postgres ticket store");
  return {
    repo,
    async close() {
      await pool.end();
    },
  };
}
