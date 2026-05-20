import { useMemo } from "react";
import { useEventStream } from "../realtime/useEventStream";

interface EventStreamProps {
  url: string;
}

function formatTime(iso: string): string {
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return iso;
  return new Date(t).toLocaleTimeString();
}

export default function EventStream({ url }: EventStreamProps): JSX.Element {
  const { events, status } = useEventStream(url, 30_000);

  const statusInfo = useMemo(() => {
    if (status === "open") return { color: "bg-emerald-500", text: "live" };
    if (status === "connecting") return { color: "bg-yellow-500 animate-pulse", text: "connecting" };
    return { color: "bg-rose-500", text: "disconnected" };
  }, [status]);

  return (
    <div className="panel flex h-full flex-col">
      <div className="flex items-center justify-between border-b border-slate-800 px-4 py-2">
        <div className="flex items-center gap-2">
          <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-300">
            Live Event Stream
          </h3>
          <span className="text-[10px] text-slate-500">last 30s</span>
        </div>
        <span className="inline-flex items-center gap-2 text-xs text-slate-400">
          <span className={`status-dot ${statusInfo.color}`} />
          {statusInfo.text}
        </span>
      </div>
      <div className="flex-1 overflow-auto px-2 py-1 font-mono text-xs">
        {events.length === 0 ? (
          <p className="p-3 text-slate-500">Keine Events. Erzeuge welche im User-Frontend.</p>
        ) : (
          <table className="w-full">
            <thead>
              <tr className="text-left text-[10px] uppercase tracking-wider text-slate-500">
                <th className="px-2 py-1 w-24">Time</th>
                <th className="px-2 py-1 w-44">Source</th>
                <th className="px-2 py-1 w-40">Detail-Type</th>
                <th className="px-2 py-1">Detail</th>
              </tr>
            </thead>
            <tbody>
              {events.map((e, i) => (
                <tr
                  key={`${e.receivedAt}-${i}`}
                  className="border-t border-slate-800/50 align-top text-slate-300"
                >
                  <td className="px-2 py-1 text-slate-400">{formatTime(e.receivedAt)}</td>
                  <td className="px-2 py-1">{e.source}</td>
                  <td className="px-2 py-1 text-indigo-300">{e.detailType}</td>
                  <td className="px-2 py-1 text-slate-400">
                    <code className="break-all">{JSON.stringify(e.detail)}</code>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
