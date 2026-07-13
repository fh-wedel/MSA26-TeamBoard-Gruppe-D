import {
  differenceInCalendarDays, addDays, startOfDay, startOfWeek, endOfWeek,
  eachDayOfInterval, format, isToday, isWeekend, min as dateMin, max as dateMax,
} from 'date-fns'
import clsx from 'clsx'
import { useUIStore } from '../../../stores/uiStore'
import type { BoardViewProps } from './index'
import type { Task } from '../../../api/types'

const DAY_WIDTH = 30   // px per day
const LABEL_WIDTH = 200 // px, frozen left column
const HEADER_H = 44     // px (month row + day row)
const GROUP_H = 30      // px
const ROW_H = 32        // px
const MIN_DAYS = 28

const STATUS_COLOR: Record<string, string> = {
  open: 'bg-border-3', in_progress: 'bg-accent', blocked: 'bg-danger',
  done: 'bg-success', archived: 'bg-text-3',
}
const PRIORITY_COLOR: Record<string, string> = {
  critical: 'bg-danger', high: 'bg-warning', medium: 'bg-amber', low: 'bg-border-3',
}
const PRIORITY_ORDER = ['critical', 'high', 'medium', 'low'] as const
const STATUS_ORDER = ['open', 'in_progress', 'blocked', 'done', 'archived'] as const

// Timeline / Gantt renderer: one bar per task spanning a configurable start..end
// date field (defaults: start_date → due_date, falling back to created_at when a
// task has no explicit start). Tasks are grouped (default: by column). The chart
// has a frozen task-name column, a month+day axis, weekend shading and a today line.
export default function GanttView({ board, tasks, presentation }: BoardViewProps) {
  const { selectTask } = useUIStore()
  const vc = presentation?.view_config ?? {}
  const startField = (vc.start_field as keyof Task) ?? 'start_date'
  const endField = (vc.end_field as keyof Task) ?? 'due_date'
  const colorBy = vc.color_by ?? 'status'
  const groupBy = vc.group_by ?? 'column'

  function field(t: Task, f: keyof Task): Date | null {
    const raw = t[f] as unknown as string | undefined
    if (!raw) return null
    const d = new Date(raw)
    return isNaN(d.getTime()) ? null : d
  }
  function span(t: Task): { start: Date; end: Date } {
    const start = startOfDay(field(t, startField) ?? field(t, 'created_at') ?? new Date())
    let end = field(t, endField)
    end = end ? startOfDay(end) : addDays(start, 1)
    if (end < start) end = start
    return { start, end }
  }
  function barColor(t: Task): string {
    if (colorBy === 'priority') return PRIORITY_COLOR[t.priority] ?? 'bg-border-3'
    return STATUS_COLOR[t.status] ?? 'bg-border-3'
  }

  if (tasks.length === 0) {
    return <div className="flex-1 flex items-center justify-center text-sm text-text-3">No tasks to chart.</div>
  }

  // Date range, padded to whole weeks (Mon start) with a minimum span.
  const spans = tasks.map(span)
  let rangeStart = startOfWeek(addDays(dateMin(spans.map((s) => s.start)), -2), { weekStartsOn: 1 })
  let rangeEnd = endOfWeek(addDays(dateMax(spans.map((s) => s.end)), 2), { weekStartsOn: 1 })
  if (differenceInCalendarDays(rangeEnd, rangeStart) + 1 < MIN_DAYS) {
    rangeEnd = addDays(rangeStart, MIN_DAYS - 1)
  }
  const days = eachDayOfInterval({ start: rangeStart, end: rangeEnd })
  const chartWidth = days.length * DAY_WIDTH

  // Month header segments.
  const months: { label: string; left: number; width: number }[] = []
  days.forEach((d, i) => {
    const label = format(d, 'MMMM yyyy')
    const last = months[months.length - 1]
    if (last && last.label === label) last.width += DAY_WIDTH
    else months.push({ label, left: i * DAY_WIDTH, width: DAY_WIDTH })
  })

  // Group tasks.
  const columns = [...(board.columns ?? [])].sort((a, b) => a.position - b.position)
  type Group = { key: string; label: string; tasks: Task[] }
  let groups: Group[]
  if (groupBy === 'column' && columns.length > 0) {
    groups = columns.map((c) => ({ key: c.id, label: c.name, tasks: tasks.filter((t) => t.column_id === c.id) }))
    const ungrouped = tasks.filter((t) => !t.column_id || !columns.some((c) => c.id === t.column_id))
    if (ungrouped.length > 0) groups.push({ key: '__none__', label: 'Ungrouped', tasks: ungrouped })
  } else {
    groups = [{ key: '__all__', label: 'All tasks', tasks }]
  }
  groups = groups.filter((g) => g.tasks.length > 0)

  // Legend explaining the bar colors (priority or status).
  const legend =
    colorBy === 'priority'
      ? PRIORITY_ORDER.map((k) => ({ key: k, label: k, color: PRIORITY_COLOR[k] }))
      : STATUS_ORDER.map((k) => ({ key: k, label: k.replace('_', ' '), color: STATUS_COLOR[k] }))

  return (
    <div className="flex-1 flex flex-col min-h-0">
      <div className="flex items-center gap-3 px-3 py-1.5 border-b border-border-1 text-[10px] text-text-3">
        <span className="uppercase tracking-wide">{colorBy}</span>
        {legend.map((l) => (
          <span key={l.key} className="flex items-center gap-1 capitalize">
            <span className={clsx('inline-block w-2.5 h-2.5 rounded-sm', l.color)} />
            {l.label}
          </span>
        ))}
      </div>
      <div className="flex-1 overflow-auto">
      <div className="flex" style={{ width: LABEL_WIDTH + chartWidth }}>
        {/* Frozen task-name column */}
        <div className="sticky left-0 z-20 bg-bg-0 flex-shrink-0 border-r border-border-1" style={{ width: LABEL_WIDTH }}>
          <div style={{ height: HEADER_H }} className="border-b border-border-1 flex items-end px-3 pb-1 text-[11px] text-text-3">
            Task
          </div>
          {groups.map((g) => (
            <div key={g.key}>
              <div style={{ height: GROUP_H }} className="flex items-center px-3 text-[11px] font-medium text-text-2 bg-bg-1">
                {g.label}
              </div>
              {g.tasks.map((t) => (
                <div key={t.id} style={{ height: ROW_H }}
                  className="flex items-center px-3 text-xs text-text-1 truncate cursor-pointer hover:bg-bg-3"
                  onClick={() => selectTask(t.id)}>
                  <span className="truncate">{t.title}</span>
                </div>
              ))}
            </div>
          ))}
        </div>

        {/* Chart area */}
        <div className="relative" style={{ width: chartWidth }}>
          {/* Background grid: weekend shading + day separators */}
          <div className="absolute inset-0 flex" style={{ top: HEADER_H, pointerEvents: 'none' }}>
            {days.map((d, i) => (
              <div key={i} style={{ width: DAY_WIDTH }}
                className={clsx('h-full border-r border-border-1/40', isWeekend(d) && 'bg-bg-1/60')} />
            ))}
          </div>
          {/* Today line */}
          {days.some((d) => isToday(d)) && (
            <div className="absolute w-px bg-accent/70 z-10" style={{
              top: HEADER_H,
              bottom: 0,
              left: differenceInCalendarDays(startOfDay(new Date()), rangeStart) * DAY_WIDTH + DAY_WIDTH / 2,
            }} />
          )}

          {/* Header: month row + day row */}
          <div className="sticky top-0 z-10 bg-bg-0 border-b border-border-1" style={{ height: HEADER_H }}>
            <div className="relative" style={{ height: 22 }}>
              {months.map((m, i) => (
                <div key={i} className="absolute top-0 h-full flex items-center px-1.5 text-[11px] font-medium text-text-1 border-l border-border-1 whitespace-nowrap"
                  style={{ left: m.left, width: m.width }}>
                  {m.label}
                </div>
              ))}
            </div>
            <div className="relative" style={{ height: 22 }}>
              {days.map((d, i) => (
                <div key={i} className={clsx('absolute top-0 h-full flex items-center justify-center text-[10px] tabular-nums',
                  isToday(d) ? 'text-accent font-semibold' : isWeekend(d) ? 'text-text-3' : 'text-text-2')}
                  style={{ left: i * DAY_WIDTH, width: DAY_WIDTH }}>
                  {format(d, 'd')}
                </div>
              ))}
            </div>
          </div>

          {/* Rows with bars */}
          {groups.map((g) => (
            <div key={g.key}>
              <div style={{ height: GROUP_H }} className="bg-bg-1/40" />
              {g.tasks.map((t) => {
                const { start, end } = span(t)
                const left = differenceInCalendarDays(start, rangeStart) * DAY_WIDTH
                const width = (differenceInCalendarDays(end, start) + 1) * DAY_WIDTH
                return (
                  <div key={t.id} style={{ height: ROW_H }} className="relative">
                    <button onClick={() => selectTask(t.id)}
                      title={`${t.title} · ${format(start, 'MMM d')} – ${format(end, 'MMM d')}`}
                      className={clsx('absolute top-1.5 bottom-1.5 rounded px-2 flex items-center text-[10px] text-white/95 truncate shadow-sm hover:brightness-110 transition', barColor(t))}
                      style={{ left: left + 2, width: Math.max(width - 4, DAY_WIDTH - 4) }}>
                      <span className="truncate">{t.title}</span>
                    </button>
                  </div>
                )
              })}
            </div>
          ))}
        </div>
      </div>
      </div>
    </div>
  )
}
