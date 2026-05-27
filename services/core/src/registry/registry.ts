import type { Redis } from "ioredis";
import type { PluginRegistration, RegisteredPlugin } from "./types";

export interface PluginRegistry {
  register(reg: PluginRegistration): Promise<RegisteredPlugin>;
  heartbeat(pluginId: string): Promise<RegisteredPlugin | null>;
  deregister(pluginId: string): Promise<boolean>;
  get(pluginId: string): Promise<RegisteredPlugin | null>;
  list(): Promise<RegisteredPlugin[]>;
}

const REGISTRY_KEY_PREFIX = "plugin:registry:";
const REGISTRY_SET_KEY = "plugin:registry:index";

export class RedisPluginRegistry implements PluginRegistry {
  constructor(
    private readonly redis: Redis,
    private readonly ttlSeconds: number,
  ) {}

  private key(pluginId: string): string {
    return `${REGISTRY_KEY_PREFIX}${pluginId}`;
  }

  async register(reg: PluginRegistration): Promise<RegisteredPlugin> {
    const now = new Date().toISOString();
    const stored: RegisteredPlugin = {
      ...reg,
      registeredAt: now,
      lastHeartbeat: now,
    };
    const key = this.key(reg.pluginId);
    await this.redis
      .multi()
      .set(key, JSON.stringify(stored), "EX", this.ttlSeconds)
      .sadd(REGISTRY_SET_KEY, reg.pluginId)
      .exec();
    return stored;
  }

  async heartbeat(pluginId: string): Promise<RegisteredPlugin | null> {
    const key = this.key(pluginId);
    const raw = await this.redis.get(key);
    if (!raw) return null;
    const plugin = JSON.parse(raw) as RegisteredPlugin;
    plugin.lastHeartbeat = new Date().toISOString();
    await this.redis.set(key, JSON.stringify(plugin), "EX", this.ttlSeconds);
    return plugin;
  }

  async deregister(pluginId: string): Promise<boolean> {
    const key = this.key(pluginId);
    const result = await this.redis
      .multi()
      .del(key)
      .srem(REGISTRY_SET_KEY, pluginId)
      .exec();
    if (!result) return false;
    const deleted = Number(result[0]?.[1] ?? 0);
    return deleted > 0;
  }

  async get(pluginId: string): Promise<RegisteredPlugin | null> {
    const raw = await this.redis.get(this.key(pluginId));
    if (!raw) return null;
    return JSON.parse(raw) as RegisteredPlugin;
  }

  async list(): Promise<RegisteredPlugin[]> {
    const ids = await this.redis.smembers(REGISTRY_SET_KEY);
    if (ids.length === 0) return [];
    const keys = ids.map((id) => this.key(id));
    const values = await this.redis.mget(...keys);
    const plugins: RegisteredPlugin[] = [];
    const expired: string[] = [];
    values.forEach((raw, i) => {
      if (raw) {
        plugins.push(JSON.parse(raw) as RegisteredPlugin);
      } else {
        const id = ids[i];
        if (id) expired.push(id);
      }
    });
    if (expired.length > 0) {
      await this.redis.srem(REGISTRY_SET_KEY, ...expired);
    }
    return plugins;
  }
}
