import { useCallback, useEffect, useMemo, useState } from "react";
import ReactFlow, {
  Background,
  Controls,
  type Edge,
  type Node,
  type NodeMouseHandler,
  type NodeProps,
  Handle,
  Position,
} from "reactflow";
import { api } from "../api/client";
import type {
  ArchitectureCategory,
  ArchitectureComponent,
  ArchitectureGraph as ArchGraph,
  RegisteredPlugin,
} from "../api/types";

interface ArchitectureGraphProps {
  onSelect: (component: ArchitectureComponent | null) => void;
  selectedId: string | null;
}

interface PositionedLayout {
  [id: string]: { x: number; y: number };
}

const LAYOUT: PositionedLayout = {
  client: { x: 60, y: 40 },
  "api-gateway-rest": { x: 60, y: 170 },
  "api-gateway-ws": { x: 60, y: 300 },
  core: { x: 380, y: 170 },
  eventbridge: { x: 700, y: 170 },
  broadcaster: { x: 380, y: 300 },
  rds: { x: 700, y: 50 },
  redis: { x: 700, y: 290 },
  dynamodb: { x: 700, y: 410 },
  s3: { x: 700, y: 530 },
};

const PLUGIN_LAYOUT_ORIGIN = { x: 1050, y: 80 };
const PLUGIN_LAYOUT_GAP = 130;

const CATEGORY_STYLE: Record<
  ArchitectureCategory,
  { ring: string; badge: string; icon: string }
> = {
  client: { ring: "ring-slate-500", badge: "bg-slate-700 text-slate-200", icon: "C" },
  edge: { ring: "ring-sky-500", badge: "bg-sky-900/50 text-sky-200", icon: "G" },
  compute: { ring: "ring-indigo-500", badge: "bg-indigo-900/50 text-indigo-200", icon: "F" },
  messaging: { ring: "ring-amber-500", badge: "bg-amber-900/40 text-amber-200", icon: "E" },
  persistence: {
    ring: "ring-emerald-500",
    badge: "bg-emerald-900/40 text-emerald-200",
    icon: "D",
  },
  plugin: { ring: "ring-fuchsia-500", badge: "bg-fuchsia-900/40 text-fuchsia-200", icon: "P" },
};

function statusColor(status: ArchitectureComponent["status"]): string {
  switch (status) {
    case "running":
      return "bg-emerald-500";
    case "stale":
      return "bg-yellow-500";
    case "down":
      return "bg-rose-500";
    case "static":
    default:
      return "bg-slate-500";
  }
}

interface CardData {
  component: ArchitectureComponent;
  selected: boolean;
}

function ComponentNode({ data }: NodeProps<CardData>): JSX.Element {
  const { component, selected } = data;
  const style = CATEGORY_STYLE[component.category];
  return (
    <div
      className={`min-w-[200px] rounded-lg border border-slate-800 bg-slate-900/95 px-3 py-2 shadow-md ring-2 ${
        selected ? "ring-indigo-400" : style.ring
      }`}
    >
      <Handle type="target" position={Position.Left} />
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <span
            className={`inline-flex h-6 w-6 items-center justify-center rounded text-[10px] font-bold ${style.badge}`}
            aria-hidden
          >
            {style.icon}
          </span>
          <div className="text-sm font-medium text-slate-100">{component.label}</div>
        </div>
        <span
          className={`status-dot ${statusColor(component.status)}`}
          title={component.status}
        />
      </div>
      <div className="mt-1 text-[10px] uppercase tracking-wider text-slate-500">
        {component.awsService}
      </div>
      <Handle type="source" position={Position.Right} />
    </div>
  );
}

const nodeTypes = { component: ComponentNode };

function ageSeconds(iso: string): number {
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return Number.POSITIVE_INFINITY;
  return Math.floor((Date.now() - t) / 1000);
}

function pluginStatus(plugin: RegisteredPlugin): ArchitectureComponent["status"] {
  const age = ageSeconds(plugin.lastHeartbeat);
  if (age < 15) return "running";
  if (age < 30) return "stale";
  return "down";
}

function pluginToComponent(plugin: RegisteredPlugin): ArchitectureComponent {
  return {
    id: `plugin:${plugin.pluginId}`,
    label: plugin.pluginId,
    awsService: plugin.awsService ?? "ECS Fargate",
    category: "plugin",
    endpoint: plugin.endpoint,
    status: pluginStatus(plugin),
    description: plugin.description,
    capabilities: plugin.capabilities,
    eventSubscriptions: plugin.eventSubscriptions,
    version: plugin.version,
    lastHeartbeat: plugin.lastHeartbeat,
    runtime: plugin.runtime,
  };
}

export default function ArchitectureGraph(props: ArchitectureGraphProps): JSX.Element {
  const [arch, setArch] = useState<ArchGraph | null>(null);
  const [plugins, setPlugins] = useState<RegisteredPlugin[]>([]);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async (): Promise<void> => {
    try {
      const [a, p] = await Promise.all([api.getArchitecture(), api.listPlugins()]);
      setArch(a);
      setPlugins(p.plugins);
      setError(null);
    } catch (err) {
      setError((err as Error).message);
    }
  }, []);

  useEffect(() => {
    void refresh();
    const t = window.setInterval(refresh, 5000);
    return (): void => window.clearInterval(t);
  }, [refresh]);

  const { nodes, edges, components } = useMemo(() => {
    const baseComponents: ArchitectureComponent[] = arch?.components ?? [];
    const pluginComponents = plugins.map(pluginToComponent);
    const allComponents: ArchitectureComponent[] = [...baseComponents, ...pluginComponents];

    const builtNodes: Node<CardData>[] = allComponents.map((c, i) => {
      let pos = LAYOUT[c.id];
      if (!pos) {
        const pluginIndex = pluginComponents.findIndex((p) => p.id === c.id);
        if (pluginIndex >= 0) {
          pos = {
            x: PLUGIN_LAYOUT_ORIGIN.x,
            y: PLUGIN_LAYOUT_ORIGIN.y + pluginIndex * PLUGIN_LAYOUT_GAP,
          };
        } else {
          pos = { x: 1050, y: 50 + i * 100 };
        }
      }
      return {
        id: c.id,
        type: "component",
        position: pos,
        data: { component: c, selected: props.selectedId === c.id },
      };
    });

    const baseEdges: Edge[] = (arch?.edges ?? []).map((e, i) => ({
      id: `e-${i}-${e.from}-${e.to}`,
      source: e.from,
      target: e.to,
      label: e.label,
      animated: e.from === "eventbridge" || e.to === "broadcaster",
    }));

    // Plugins: core -> plugin (proxy + registry), plugin -> eventbridge (publish)
    const pluginEdges: Edge[] = pluginComponents.flatMap((p) => [
      {
        id: `pe-core-${p.id}`,
        source: "core",
        target: p.id,
        label: "proxy",
      },
      {
        id: `pe-${p.id}-bus`,
        source: p.id,
        target: "eventbridge",
        label: "publish",
        animated: true,
      },
    ]);

    return {
      nodes: builtNodes,
      edges: [...baseEdges, ...pluginEdges],
      components: allComponents,
    };
  }, [arch, plugins, props.selectedId]);

  const onNodeClick: NodeMouseHandler = useCallback(
    (_e, node) => {
      const c = components.find((x) => x.id === node.id);
      props.onSelect(c ?? null);
    },
    [components, props],
  );

  const onPaneClick = useCallback((): void => {
    props.onSelect(null);
  }, [props]);

  return (
    <div className="relative h-full w-full">
      {error && (
        <div className="absolute left-3 top-3 z-10 rounded border border-rose-700/40 bg-rose-950/60 px-3 py-1.5 text-xs text-rose-200">
          {error}
        </div>
      )}
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        onNodeClick={onNodeClick}
        onPaneClick={onPaneClick}
        fitView
        proOptions={{ hideAttribution: true }}
        nodesDraggable
        nodesConnectable={false}
        elementsSelectable
      >
        <Background gap={20} size={1} color="#1e293b" />
        <Controls position="bottom-right" />
      </ReactFlow>
    </div>
  );
}
