import { useState } from 'react'
import {
  startOfMonth, endOfMonth, startOfWeek, endOfWeek, eachDayOfInterval,
  format, isSameMonth, isSameDay, addMonths, subMonths,
} from 'date-fns'
import { ChevronLeft, ChevronRight, Plus } from 'lucide-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'
import { useUIStore } from '../../../stores/uiStore'
import { tasksApi } from '../../../api/tasks'
import type { BoardViewProps } from './index'
import type { Task } from '../../../api/types'

const PRIORITY_DOT: Record<string, string> = {
  critical: 'bg-danger', high: 'bg-warning', medium: 'bg-amber', low: 'bg-border-3',
}

// Calendar renderer: places tasks on a month grid by a configurable date field
// (presentation.view_config.date_field, default "due_date"). Tasks without that
// date go into an "Unscheduled" tray. week_start controls the first weekday.
// Clicking a day's "+" quick-adds a task with that date (in the board's first
// column, which the calendar otherwise ignores).
export default function CalendarView({ board, tasks, presentation }: BoardViewProps) {
  const { selectTask } = useUIStore()
  const qc = useQueryClient()
  const [month, setMonth] = useState(() => startOfMonth(new Date()))
  const [addingDay, setAddingDay] = useState<string | null>(null)
  const [title, setTitle] = useState('')

  const vc = presentation?.view_config ?? {}
  const dateField = (vc.date_field as keyof Task) ?? 'due_date'
  const weekStartsOn = vc.week_start === 'sunday' ? 0 : 1
  const firstColumnId = board.columns?.[0]?.id

  const addTask = useMutation({
    mutationFn: ({ day }: { day: Date }) =>
      tasksApi.create(board.id, {
        title,
        column_id: firstColumnId,
        ...(dateField === 'due_date' ? { due_date: day.toISOString() } : {}),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['tasks', board.id] })
      setTitle(''); setAddingDay(null)
    },
  })

  function taskDate(t: Task): Date | null {
    const raw = t[dateField] as unknown as string | undefined
    if (!raw) return null
    const d = new Date(raw)
    return isNaN(d.getTime()) ? null : d
  }

  const scheduled = tasks.filter((t) => taskDate(t) !== null)
  const unscheduled = tasks.filter((t) => taskDate(t) === null)

  const gridStart = startOfWeek(startOfMonth(month), { weekStartsOn })
  const gridEnd = endOfWeek(endOfMonth(month), { weekStartsOn })
  const days = eachDayOfInterval({ start: gridStart, end: gridEnd })

  const weekdays = Array.from({ length: 7 }, (_, i) =>
    format(eachDayOfInterval({ start: gridStart, end: endOfWeek(gridStart, { weekStartsOn }) })[i], 'EEE'))

  function tasksOn(day: Date) {
    return scheduled.filter((t) => isSameDay(taskDate(t)!, day))
  }

  return (
    <div className="flex flex-col h-full p-6 overflow-hidden">
      {/* Toolbar */}
      <div className="flex items-center gap-3 mb-4 flex-shrink-0">
        <h2 className="text-sm font-semibold text-text-0 w-40">{format(month, 'MMMM yyyy')}</h2>
        <div className="flex items-center gap-1">
          <button onClick={() => setMonth(subMonths(month, 1))} className="btn-ghost p-1.5" aria-label="Previous month">
            <ChevronLeft size={15} />
          </button>
          <button onClick={() => setMonth(startOfMonth(new Date()))} className="btn-ghost text-xs px-2 py-1">Today</button>
          <button onClick={() => setMonth(addMonths(month, 1))} className="btn-ghost p-1.5" aria-label="Next month">
            <ChevronRight size={15} />
          </button>
        </div>
        <span className="text-xs text-text-3 ml-auto">by {String(dateField).replace('_', ' ')}</span>
      </div>

      {/* Weekday header */}
      <div className="grid grid-cols-7 gap-px mb-px flex-shrink-0">
        {weekdays.map((d) => (
          <div key={d} className="text-[11px] font-medium text-text-3 px-2 py-1">{d}</div>
        ))}
      </div>

      {/* Day grid */}
      <div className="grid grid-cols-7 gap-px bg-border-1 flex-1 overflow-y-auto rounded overflow-hidden auto-rows-fr">
        {days.map((day) => {
          const dayTasks = tasksOn(day)
          const inMonth = isSameMonth(day, month)
          const today = isSameDay(day, new Date())
          const dayKey = day.toISOString()
          const canAdd = dateField === 'due_date' && !!firstColumnId
          return (
            <div key={dayKey}
              className={clsx('group bg-bg-1 min-h-24 p-1.5 flex flex-col gap-1', !inMonth && 'opacity-40')}>
              <div className="flex items-center justify-between">
                {canAdd ? (
                  <button onClick={() => { setAddingDay(dayKey); setTitle('') }}
                    className="text-text-3 hover:text-accent transition-colors" aria-label="Add task">
                    <Plus size={12} />
                  </button>
                ) : <span />}
                <span className={clsx('text-[11px] px-1 rounded',
                  today ? 'bg-accent text-white font-semibold' : 'text-text-3')}>
                  {format(day, 'd')}
                </span>
              </div>
              {addingDay === dayKey && (
                <input autoFocus value={title} onChange={(e) => setTitle(e.target.value)}
                  placeholder="Task title…"
                  className="input-base w-full text-[11px] py-0.5 px-1"
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && title.trim()) addTask.mutate({ day })
                    if (e.key === 'Escape') { setAddingDay(null); setTitle('') }
                  }}
                  onBlur={() => { if (!title.trim()) setAddingDay(null) }} />
              )}
              {dayTasks.slice(0, 4).map((t) => (
                <button key={t.id} onClick={() => selectTask(t.id)}
                  className="flex items-center gap-1 text-left text-[11px] text-text-1 bg-bg-3 hover:bg-bg-2 border border-border-1 rounded px-1.5 py-0.5 truncate">
                  <span className={clsx('w-1.5 h-1.5 rounded-full flex-shrink-0', PRIORITY_DOT[t.priority])} />
                  <span className="truncate">{t.title}</span>
                </button>
              ))}
              {dayTasks.length > 4 && (
                <span className="text-[10px] text-text-3 px-1">+{dayTasks.length - 4} more</span>
              )}
            </div>
          )
        })}
      </div>

      {/* Unscheduled tray */}
      {unscheduled.length > 0 && (
        <div className="flex-shrink-0 mt-3">
          <p className="label mb-1.5">Unscheduled ({unscheduled.length})</p>
          <div className="flex gap-2 flex-wrap">
            {unscheduled.map((t) => (
              <button key={t.id} onClick={() => selectTask(t.id)}
                className="flex items-center gap-1.5 text-xs text-text-1 bg-bg-3 hover:bg-bg-2 border border-border-1 rounded px-2 py-1">
                <span className={clsx('w-1.5 h-1.5 rounded-full', PRIORITY_DOT[t.priority])} />
                {t.title}
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
