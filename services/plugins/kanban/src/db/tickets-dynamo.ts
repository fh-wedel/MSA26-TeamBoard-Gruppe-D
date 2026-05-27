import { randomUUID } from "node:crypto";
import { DynamoDBClient } from "@aws-sdk/client-dynamodb";
import {
  DynamoDBDocumentClient,
  DeleteCommand,
  PutCommand,
  QueryCommand,
  ScanCommand,
} from "@aws-sdk/lib-dynamodb";
import type {
  CreateTicketInput,
  Ticket,
  TicketRepository,
  UpdateTicketInput,
} from "./tickets";

interface DynamoTicketItem {
  pk: string;
  sk: string;
  boardId: number;
  ticketId: string;
  title: string;
  description: string;
  status: string;
  position: number;
  createdAt: string;
  updatedAt: string;
}

function pk(boardId: number): string {
  return `BOARD#${boardId}`;
}

function sk(ticketId: string): string {
  return `TICKET#${ticketId}`;
}

function toTicket(item: DynamoTicketItem): Ticket {
  return {
    id: item.ticketId,
    boardId: item.boardId,
    title: item.title,
    description: item.description,
    status: item.status,
    position: item.position,
    createdAt: item.createdAt,
    updatedAt: item.updatedAt,
  };
}

export interface DynamoTicketRepositoryOptions {
  tableName: string;
  region: string;
  endpoint?: string;
}

export class DynamoTicketRepository implements TicketRepository {
  private readonly doc: DynamoDBDocumentClient;
  private readonly tableName: string;

  constructor(opts: DynamoTicketRepositoryOptions) {
    const client = new DynamoDBClient({
      region: opts.region,
      ...(opts.endpoint ? { endpoint: opts.endpoint } : {}),
    });
    this.doc = DynamoDBDocumentClient.from(client);
    this.tableName = opts.tableName;
  }

  async list(boardId?: number): Promise<Ticket[]> {
    if (boardId === undefined) {
      // Cross-board listing — scan. Acceptable for PoC scale; for prod we'd
      // add a GSI on a constant pk like "ALL" to keep this query-able.
      const out = await this.doc.send(
        new ScanCommand({ TableName: this.tableName }),
      );
      const items = (out.Items ?? []) as DynamoTicketItem[];
      items.sort(
        (a, b) =>
          a.boardId - b.boardId ||
          a.status.localeCompare(b.status) ||
          a.position - b.position ||
          a.ticketId.localeCompare(b.ticketId),
      );
      return items.map(toTicket);
    }
    const out = await this.doc.send(
      new QueryCommand({
        TableName: this.tableName,
        KeyConditionExpression: "pk = :pk",
        ExpressionAttributeValues: { ":pk": pk(boardId) },
      }),
    );
    const items = (out.Items ?? []) as DynamoTicketItem[];
    items.sort(
      (a, b) =>
        a.status.localeCompare(b.status) ||
        a.position - b.position ||
        a.ticketId.localeCompare(b.ticketId),
    );
    return items.map(toTicket);
  }

  async get(id: string): Promise<Ticket | null> {
    // We don't know the board from id alone — scan with a filter. Fine for
    // PoC; in prod we'd require boardId in the API or add a GSI on ticketId.
    const out = await this.doc.send(
      new ScanCommand({
        TableName: this.tableName,
        FilterExpression: "ticketId = :id",
        ExpressionAttributeValues: { ":id": id },
        Limit: 1,
      }),
    );
    const item = (out.Items ?? [])[0] as DynamoTicketItem | undefined;
    return item ? toTicket(item) : null;
  }

  async create(input: CreateTicketInput): Promise<Ticket> {
    const ticketId = randomUUID();
    const now = new Date().toISOString();
    const item: DynamoTicketItem = {
      pk: pk(input.boardId),
      sk: sk(ticketId),
      boardId: input.boardId,
      ticketId,
      title: input.title,
      description: input.description ?? "",
      status: input.status ?? "todo",
      position: input.position ?? 0,
      createdAt: now,
      updatedAt: now,
    };
    await this.doc.send(
      new PutCommand({ TableName: this.tableName, Item: item }),
    );
    return toTicket(item);
  }

  async update(
    id: string,
    input: UpdateTicketInput,
  ): Promise<{ before: Ticket; after: Ticket } | null> {
    const before = await this.get(id);
    if (!before) return null;
    const nextBoardId = input.boardId ?? before.boardId;
    const next: DynamoTicketItem = {
      pk: pk(nextBoardId),
      sk: sk(before.id),
      boardId: nextBoardId,
      ticketId: before.id,
      title: input.title ?? before.title,
      description: input.description ?? before.description,
      status: input.status ?? before.status,
      position: input.position ?? before.position,
      createdAt: before.createdAt,
      updatedAt: new Date().toISOString(),
    };
    if (nextBoardId !== before.boardId) {
      await this.doc.send(
        new DeleteCommand({
          TableName: this.tableName,
          Key: { pk: pk(before.boardId), sk: sk(before.id) },
        }),
      );
    }
    await this.doc.send(
      new PutCommand({ TableName: this.tableName, Item: next }),
    );
    return { before, after: toTicket(next) };
  }

  async delete(id: string): Promise<boolean> {
    const existing = await this.get(id);
    if (!existing) return false;
    await this.doc.send(
      new DeleteCommand({
        TableName: this.tableName,
        Key: { pk: pk(existing.boardId), sk: sk(existing.id) },
      }),
    );
    return true;
  }
}
