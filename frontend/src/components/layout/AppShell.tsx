import { Outlet } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import Sidebar from './Sidebar'
import TopBar from './TopBar'
import { useWebSocket } from '../../hooks/useWebSocket'
import { useAuthStore } from '../../stores/authStore'

export default function AppShell() {
  const qc = useQueryClient()
  const { user } = useAuthStore()

  // Board types are global runtime data from the Board Registry. Subscribe to the
  // shared board-types channel app-wide so a type registered/updated/removed by
  // anyone refreshes every client's create-board dialog without a reload.
  useWebSocket((msg) => {
    if (msg.type !== 'event') return
    const eventType: string | undefined = msg.data?.event_type
    if (eventType?.startsWith('boardtype.')) {
      qc.invalidateQueries({ queryKey: ['board-types'] })
    }
  }, !!user, ['boardtypes:all'])

  return (
    <div className="flex h-screen overflow-hidden bg-bg-1">
      <Sidebar />
      <div className="flex flex-col flex-1 min-w-0">
        <TopBar />
        <main className="flex-1 overflow-auto">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
