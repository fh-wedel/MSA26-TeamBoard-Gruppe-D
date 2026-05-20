export interface PluginRegistration {
  pluginId: string;
  version: string;
  capabilities: string[];
  endpoint: string;
  eventSubscriptions: string[];
  healthCheck: string;
  runtime?: string;
  awsService?: string;
  description?: string;
}

export interface RegisteredPlugin extends PluginRegistration {
  registeredAt: string;
  lastHeartbeat: string;
}
