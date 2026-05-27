import { DynamoDBClient } from "@aws-sdk/client-dynamodb";
import {
  DynamoDBDocumentClient,
  DeleteCommand,
  GetCommand,
  PutCommand,
  ScanCommand,
} from "@aws-sdk/lib-dynamodb";
import type { PluginRegistry } from "./registry";
import type { PluginRegistration, RegisteredPlugin } from "./types";

interface RegistryItem extends RegisteredPlugin {
  pk: string;
  ttl: number;
}

const PK = "PLUGIN";

function nowSeconds(): number {
  return Math.floor(Date.now() / 1000);
}

export interface DynamoPluginRegistryOptions {
  tableName: string;
  region: string;
  ttlSeconds: number;
  endpoint?: string;
}

/**
 * Plugin registry backed by DynamoDB with native TTL.
 * Item shape: { pk: "PLUGIN", pluginId, ...registration, ttl }.
 * DynamoDB sweeps expired items via the table's TTL attribute, so missed
 * heartbeats drop the plugin automatically (lazy, typically within minutes).
 * `list()` also filters in-app to avoid returning items past their TTL even
 * before the sweeper notices.
 */
export class DynamoPluginRegistry implements PluginRegistry {
  private readonly doc: DynamoDBDocumentClient;
  private readonly tableName: string;
  private readonly ttlSeconds: number;

  constructor(opts: DynamoPluginRegistryOptions) {
    const client = new DynamoDBClient({
      region: opts.region,
      ...(opts.endpoint ? { endpoint: opts.endpoint } : {}),
    });
    this.doc = DynamoDBDocumentClient.from(client);
    this.tableName = opts.tableName;
    this.ttlSeconds = opts.ttlSeconds;
  }

  async register(reg: PluginRegistration): Promise<RegisteredPlugin> {
    const now = new Date().toISOString();
    const stored: RegisteredPlugin = {
      ...reg,
      registeredAt: now,
      lastHeartbeat: now,
    };
    const item: RegistryItem = {
      pk: PK,
      ...stored,
      ttl: nowSeconds() + this.ttlSeconds,
    };
    await this.doc.send(
      new PutCommand({ TableName: this.tableName, Item: item }),
    );
    return stored;
  }

  async heartbeat(pluginId: string): Promise<RegisteredPlugin | null> {
    const existing = await this.get(pluginId);
    if (!existing) return null;
    existing.lastHeartbeat = new Date().toISOString();
    const item: RegistryItem = {
      pk: PK,
      ...existing,
      ttl: nowSeconds() + this.ttlSeconds,
    };
    await this.doc.send(
      new PutCommand({ TableName: this.tableName, Item: item }),
    );
    return existing;
  }

  async deregister(pluginId: string): Promise<boolean> {
    const existing = await this.get(pluginId);
    if (!existing) return false;
    await this.doc.send(
      new DeleteCommand({
        TableName: this.tableName,
        Key: { pk: PK, pluginId },
      }),
    );
    return true;
  }

  async get(pluginId: string): Promise<RegisteredPlugin | null> {
    const out = await this.doc.send(
      new GetCommand({
        TableName: this.tableName,
        Key: { pk: PK, pluginId },
      }),
    );
    if (!out.Item) return null;
    const item = out.Item as RegistryItem;
    if (item.ttl && item.ttl < nowSeconds()) return null;
    return stripInternal(item);
  }

  async list(): Promise<RegisteredPlugin[]> {
    const out = await this.doc.send(
      new ScanCommand({
        TableName: this.tableName,
        FilterExpression: "pk = :pk",
        ExpressionAttributeValues: { ":pk": PK },
      }),
    );
    const items = (out.Items ?? []) as RegistryItem[];
    const cutoff = nowSeconds();
    return items.filter((i) => !i.ttl || i.ttl >= cutoff).map(stripInternal);
  }
}

function stripInternal(item: RegistryItem): RegisteredPlugin {
  const { pk: _pk, ttl: _ttl, ...rest } = item;
  return rest;
}
