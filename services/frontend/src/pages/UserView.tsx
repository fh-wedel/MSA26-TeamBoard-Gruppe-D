import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import PluginList from "../components/PluginList";
import KanbanBoard from "../components/KanbanBoard";
import type { RegisteredPlugin } from "../api/types";

const KANBAN_PLUGIN_ID = "kanban-board";

function buildWsUrl(): string {
  const fromEnv = import.meta.env.VITE_BROADCASTER_URL;
  if (typeof fromEnv === "string" && fromEnv.length > 0) {
    return `${fromEnv.replace(/\/$/, "")}/ws`;
  }
  if (typeof window !== "undefined") {
    const proto = window.location.protocol === "https:" ? "wss" : "ws";
    return `${proto}://${window.location.hostname}:3002/ws`;
  }
  return "ws://localhost:3002/ws";
}

function GenericPluginPanel({ plugin }: { plugin: RegisteredPlugin }): JSX.Element {
  return (
    <div className="panel p-4">
      <div className="mb-2 flex items-center justify-between">
        <h2 className="text-base font-semibold text-slate-100">{plugin.pluginId}</h2>
        <span className="text-xs text-slate-500">v{plugin.version}</span>
      </div>
      {plugin.description && (
        <p className="mb-3 text-sm text-slate-300">{plugin.description}</p>
      )}
      <p className="mb-2 text-xs text-slate-500">
        Dieses Plugin hat keine dedizierte UI im Frontend. Es folgen die rohen Metadaten:
      </p>
      <pre className="overflow-auto rounded bg-slate-950/70 p-3 text-xs text-slate-300">
        {JSON.stringify(plugin, null, 2)}
      </pre>
    </div>
  );
}

export default function UserView(): JSX.Element {
  const [plugins, setPlugins] = useState<RegisteredPlugin[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const wsUrl = useMemo(buildWsUrl, []);

  useEffect(() => {
    let cancelled = false;
    const load = async (): Promise<void> => {
      try {
        const res = await api.listPlugins();
        if (cancelled) return;
        setPlugins(res.plugins);
        if (!selected && res.plugins.length > 0) {
          // Prefer kanban if available.
          const kanban = res.plugins.find((p) => p.pluginId === KANBAN_PLUGIN_ID);
          setSelected(kanban?.pluginId ?? res.plugins[0]?.pluginId ?? null);
        }
      } catch {
        // PluginList component surfaces errors.
      }
    };
    void load();
    const t = window.setInterval(load, 5000);
    return (): void => {
      cancelled = true;
      window.clearInterval(t);
    };
    // selected intentionally omitted: we only want to seed once.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const hasKanban = plugins.some((p) => p.pluginId === KANBAN_PLUGIN_ID);
  const selectedPlugin = plugins.find((p) => p.pluginId === selected) ?? null;
  const showKanban = hasKanban && (selected === KANBAN_PLUGIN_ID || selected === null);

  return (
    <div className="flex h-full flex-col">
      <header className="border-b border-slate-800 bg-slate-900/60 px-6 py-3">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold text-slate-100">
              msa2 — Plugin Architecture PoC
            </h1>
            <p className="text-xs text-slate-500">User view · Live plugin demo</p>
          </div>
          <nav className="flex items-center gap-2">
            <Link to="/admin" className="btn-ghost">
              Admin →
            </Link>
          </nav>
        </div>
      </header>

      <main className="grid flex-1 min-h-0 grid-cols-[280px_1fr] gap-4 p-4">
        <aside className="min-h-0">
          <PluginList onSelect={setSelected} selectedPluginId={selected} />
        </aside>

        <section className="min-h-0">
          {showKanban ? (
            <KanbanBoard wsUrl={wsUrl} />
          ) : selectedPlugin ? (
            <GenericPluginPanel plugin={selectedPlugin} />
          ) : (
            <div className="panel p-6 text-sm text-slate-400">
              Keine Plugins registriert. Sobald ein Plugin den Heartbeat sendet, erscheint es
              hier.
            </div>
          )}
        </section>
      </main>
    </div>
  );
}
