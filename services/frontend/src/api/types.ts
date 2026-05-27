export interface RegisteredPlugin {
  pluginId: string;
  version: string;
  capabilities: string[];
  endpoint: string;
  eventSubscriptions: string[];
  healthCheck: string;
  runtime?: string;
  awsService?: string;
  description?: string;
  registeredAt: string;
  lastHeartbeat: string;
}

export interface PluginListResponse {
  plugins: RegisteredPlugin[];
}

export type TicketStatus = "todo" | "in-progress" | "done";

export interface Ticket {
  id: string;
  boardId: number;
  title: string;
  description: string | null;
  status: TicketStatus;
  position: number;
  createdAt: string;
  updatedAt: string;
}

export interface TicketListResponse {
  tickets: Ticket[];
}

export type ArchitectureCategory =
  | "client"
  | "edge"
  | "compute"
  | "messaging"
  | "persistence"
  | "plugin";

export interface ArchitectureComponent {
  id: string;
  label: string;
  awsService: string;
  category: ArchitectureCategory;
  endpoint: string | null;
  status: "static" | "running" | "down" | "stale";
  description?: string;
  // Plugin-only fields:
  capabilities?: string[];
  eventSubscriptions?: string[];
  version?: string;
  lastHeartbeat?: string;
  runtime?: string;
}

export interface ArchitectureEdge {
  from: string;
  to: string;
  label: string;
}

export interface ArchitectureGraph {
  components: ArchitectureComponent[];
  edges: ArchitectureEdge[];
}

export interface StreamEvent {
  receivedAt: string;
  source: string;
  detailType: string;
  detail: Record<string, unknown>;
}
