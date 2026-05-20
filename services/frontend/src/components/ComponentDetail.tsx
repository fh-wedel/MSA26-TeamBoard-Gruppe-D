import type { ArchitectureComponent } from "../api/types";

interface ComponentDetailProps {
  component: ArchitectureComponent | null;
}

function Row({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}): JSX.Element {
  return (
    <div className="grid grid-cols-[120px_1fr] gap-2 py-1">
      <div className="text-xs uppercase tracking-wider text-slate-500">{label}</div>
      <div className="text-sm text-slate-200 break-words">{children}</div>
    </div>
  );
}

function relative(iso?: string | null): string {
  if (!iso) return "—";
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return iso;
  const ageS = Math.floor((Date.now() - t) / 1000);
  if (ageS < 60) return `${ageS}s ago`;
  if (ageS < 3600) return `${Math.floor(ageS / 60)}m ago`;
  return new Date(t).toLocaleString();
}

export default function ComponentDetail({ component }: ComponentDetailProps): JSX.Element {
  if (!component) {
    return (
      <div className="panel h-full p-4 text-sm text-slate-400">
        Wähle einen Knoten im Graph, um Details anzuzeigen.
      </div>
    );
  }

  const isPlugin = component.category === "plugin";

  return (
    <div className="panel flex h-full flex-col p-4">
      <div className="mb-3">
        <div className="text-xs uppercase tracking-wider text-slate-500">
          {component.awsService}
        </div>
        <h2 className="text-lg font-semibold text-slate-100">{component.label}</h2>
      </div>

      {component.description && (
        <p className="mb-3 rounded border border-slate-800 bg-slate-950/60 p-3 text-sm text-slate-300">
          {component.description}
        </p>
      )}

      <div className="divide-y divide-slate-800/70">
        <Row label="Category">{component.category}</Row>
        <Row label="Status">
          <span className="inline-flex items-center gap-2">
            <span
              className={`status-dot ${
                component.status === "running"
                  ? "bg-emerald-500"
                  : component.status === "stale"
                    ? "bg-yellow-500"
                    : component.status === "down"
                      ? "bg-rose-500"
                      : "bg-slate-500"
              }`}
            />
            {component.status}
          </span>
        </Row>
        <Row label="Endpoint">
          {component.endpoint ? (
            <code className="text-xs">{component.endpoint}</code>
          ) : (
            <span className="text-slate-500">—</span>
          )}
        </Row>
        {isPlugin && (
          <>
            <Row label="Version">v{component.version ?? "?"}</Row>
            <Row label="Runtime">{component.runtime ?? "—"}</Row>
            <Row label="Last heartbeat">{relative(component.lastHeartbeat)}</Row>
            <Row label="Capabilities">
              <div className="flex flex-wrap gap-1">
                {(component.capabilities ?? []).map((c) => (
                  <span
                    key={c}
                    className="rounded bg-slate-800 px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-slate-300"
                  >
                    {c}
                  </span>
                ))}
                {(component.capabilities ?? []).length === 0 && (
                  <span className="text-slate-500">—</span>
                )}
              </div>
            </Row>
            <Row label="Subscriptions">
              <div className="flex flex-wrap gap-1">
                {(component.eventSubscriptions ?? []).map((c) => (
                  <code
                    key={c}
                    className="rounded bg-slate-800 px-1.5 py-0.5 text-[10px] text-slate-300"
                  >
                    {c}
                  </code>
                ))}
                {(component.eventSubscriptions ?? []).length === 0 && (
                  <span className="text-slate-500">—</span>
                )}
              </div>
            </Row>
          </>
        )}
      </div>
    </div>
  );
}
