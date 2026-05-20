import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { RegisteredPlugin } from "../api/types";

interface PluginListProps {
  onSelect?: (pluginId: string) => void;
  selectedPluginId?: string | null;
}

function ageSeconds(iso: string): number {
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return Number.POSITIVE_INFINITY;
  return Math.floor((Date.now() - t) / 1000);
}

function statusFor(plugin: RegisteredPlugin): {
  color: string;
  label: string;
} {
  const age = ageSeconds(plugin.lastHeartbeat);
  if (age < 15) return { color: "bg-emerald-500", label: "healthy" };
  if (age < 30) return { color: "bg-yellow-500", label: "stale" };
  return { color: "bg-rose-500", label: "down" };
}

export default function PluginList(props: PluginListProps): JSX.Element {
  const [plugins, setPlugins] = useState<RegisteredPlugin[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    const refresh = async (): Promise<void> => {
      try {
        const res = await api.listPlugins();
        if (cancelled) return;
        setPlugins(res.plugins);
        setError(null);
      } catch (err) {
        if (cancelled) return;
        setError((err as Error).message);
      } finally {
        if (!cancelled) setLoading(false);
      }
    };
    void refresh();
    const timer = window.setInterval(refresh, 5000);
    return (): void => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, []);

  return (
    <div className="panel p-4 h-full overflow-auto">
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-sm font-semibold uppercase tracking-wider text-slate-300">
          Registered Plugins
        </h2>
        <span className="text-xs text-slate-500">auto-refresh 5s</span>
      </div>

      {loading && plugins.length === 0 && (
        <p className="text-sm text-slate-400">Loading…</p>
      )}
      {error && (
        <p className="text-sm text-rose-400 mb-2" title={error}>
          Failed to load plugins.
        </p>
      )}
      {!loading && plugins.length === 0 && !error && (
        <p className="text-sm text-slate-500">No plugins registered yet.</p>
      )}

      <ul className="space-y-2">
        {plugins.map((p) => {
          const s = statusFor(p);
          const selected = props.selectedPluginId === p.pluginId;
          return (
            <li key={p.pluginId}>
              <button
                type="button"
                onClick={() => props.onSelect?.(p.pluginId)}
                className={`w-full text-left rounded-md px-3 py-2 border transition-colors ${
                  selected
                    ? "border-indigo-500 bg-indigo-500/10"
                    : "border-slate-800 hover:border-slate-700 hover:bg-slate-800/60"
                }`}
              >
                <div className="flex items-center justify-between">
                  <span className="font-medium text-slate-100">{p.pluginId}</span>
                  <span
                    className={`status-dot ${s.color}`}
                    title={`${s.label} (heartbeat ${ageSeconds(p.lastHeartbeat)}s ago)`}
                  />
                </div>
                <div className="mt-1 text-xs text-slate-400">v{p.version}</div>
                {p.description && (
                  <div className="mt-1 text-xs text-slate-500 line-clamp-2">
                    {p.description}
                  </div>
                )}
                <div className="mt-1 flex flex-wrap gap-1">
                  {p.capabilities.map((c) => (
                    <span
                      key={c}
                      className="inline-flex text-[10px] uppercase tracking-wide rounded bg-slate-800 px-1.5 py-0.5 text-slate-300"
                    >
                      {c}
                    </span>
                  ))}
                </div>
              </button>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
