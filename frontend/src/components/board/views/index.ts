import type { ComponentType } from 'react'
import type { Board, Presentation, Task, ViewKind } from '../../../api/types'
import BoardView from './BoardView'
import CalendarView from './CalendarView'
import GanttView from './GanttView'

// Shared contract every board renderer implements. A view receives the board, its
// tasks and the (already resolved) presentation spec, and renders itself. Task
// selection is handled globally via uiStore, so views just call selectTask on click.
export interface BoardViewProps {
  board: Board
  tasks: Task[]
  presentation?: Presentation
}

// The set of built-in renderers the host "advertises". A board type's
// presentation.view selects one of these. This map is the single extension point:
// adding a new paradigm = adding an entry here; a future `remote` kind would load a
// micro-frontend without changing the rest of the model.
const REGISTRY: Record<ViewKind, ComponentType<BoardViewProps>> = {
  board: BoardView,
  calendar: CalendarView,
  timeline: GanttView,
}

export function resolveView(view: ViewKind | string | undefined): {
  Component: ComponentType<BoardViewProps>
  known: boolean
} {
  if (view && view in REGISTRY) {
    return { Component: REGISTRY[view as ViewKind], known: true }
  }
  // Unknown/empty → fall back to the column board.
  return { Component: BoardView, known: !view }
}
