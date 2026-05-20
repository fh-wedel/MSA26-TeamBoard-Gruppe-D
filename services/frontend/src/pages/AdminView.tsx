import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import ArchitectureGraph from "../components/ArchitectureGraph";
import ComponentDetail from "../components/ComponentDetail";
import EventStream from "../components/EventStream";
import type { ArchitectureComponent } from "../api/types";

function buildSseUrl(): string {
  const fromEnv = import.meta.env.VITE_CORE_URL;
  const base =
    typeof fromEnv === "string" && fromEnv.length > 0
      ? fromEnv.replace(/\/$/, "")
      : `${window.location.protocol}//${window.location.hostname}:3000`;
  return `${base}/api/admin/events/stream`;
}

export default function AdminView(): JSX.Element {
  const [selected, setSelected] = useState<ArchitectureComponent | null>(null);
  const sseUrl = useMemo(buildSseUrl, []);

  return (
    <div className="flex h-full flex-col">
      <header className="border-b border-slate-800 bg-slate-900/60 px-6 py-3">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold text-slate-100">
              Admin — Architecture Overview
            </h1>
            <p className="text-xs text-slate-500">
              Live topology of running components · auto-refresh 5s
            </p>
          </div>
          <nav className="flex items-center gap-2">
            <Link to="/" className="btn-ghost">
              ← User View
            </Link>
          </nav>
        </div>
      </header>

      <main className="grid flex-1 min-h-0 grid-rows-[1fr_240px] grid-cols-[1fr_360px] gap-4 p-4">
        <section className="row-span-1 col-span-1 panel overflow-hidden">
          <ArchitectureGraph
            onSelect={setSelected}
            selectedId={selected?.id ?? null}
          />
        </section>
        <aside className="row-span-1 col-span-1 min-h-0">
          <ComponentDetail component={selected} />
        </aside>
        <section className="row-span-1 col-span-2 min-h-0">
          <EventStream url={sseUrl} />
        </section>
      </main>
    </div>
  );
}
