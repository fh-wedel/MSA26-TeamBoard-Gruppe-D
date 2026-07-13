import { useState, useRef, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Bell, Check, X, UserPlus, CheckCircle, XCircle } from 'lucide-react'
import { notificationsApi } from '../../api/notifications'
import { invitationsApi } from '../../api/invitations'
import { useWebSocket } from '../../hooks/useWebSocket'
import { useAuthStore } from '../../stores/authStore'
import { formatDistanceToNow } from 'date-fns'
import clsx from 'clsx'

export default function NotificationBell() {
  const [open, setOpen] = useState(false)
  const [pulse, setPulse] = useState(false)
  const pulseTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const ref = useRef<HTMLDivElement>(null)
  const qc = useQueryClient()
  const { user } = useAuthStore()

  const { data: countData } = useQuery({
    queryKey: ['notifications', 'count'],
    queryFn: () => notificationsApi.unreadCount(),
    refetchInterval: 30_000,
  })

  const { data: listData } = useQuery({
    queryKey: ['notifications', 'list'],
    queryFn: () => notificationsApi.list(),
    enabled: open,
  })

  // Live push: a new notification refreshes the unread count (and the open list)
  // immediately and briefly pulses the bell so the new item is noticed.
  useWebSocket((msg) => {
    if (msg.type === 'notification') {
      qc.invalidateQueries({ queryKey: ['notifications'] })
      setPulse(true)
      if (pulseTimer.current) clearTimeout(pulseTimer.current)
      pulseTimer.current = setTimeout(() => setPulse(false), 2500)
    }
  }, !!user)

  useEffect(() => () => { if (pulseTimer.current) clearTimeout(pulseTimer.current) }, [])

  const markRead = useMutation({
    mutationFn: (id: string) => notificationsApi.markRead(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['notifications'] }),
  })

  const markAll = useMutation({
    mutationFn: () => notificationsApi.markAllRead(),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['notifications'] }),
  })

  const acceptInvitation = useMutation({
    mutationFn: ({ notifId, token }: { notifId: string; token: string }) =>
      invitationsApi.accept(token).then(() => notificationsApi.markRead(notifId)),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['notifications'] })
      qc.invalidateQueries({ queryKey: ['projects'] })
    },
  })

  const declineInvitation = useMutation({
    mutationFn: ({ notifId, token }: { notifId: string; token: string }) =>
      invitationsApi.decline(token).then(() => notificationsApi.markRead(notifId)),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['notifications'] }),
  })

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [])

  const count = countData?.data?.count ?? 0
  const notifications = listData?.data ?? []

  return (
    <div ref={ref} className="relative">
      <button onClick={() => setOpen(!open)}
        className={clsx(
          'relative w-8 h-8 flex items-center justify-center rounded text-text-2 hover:text-text-0 hover:bg-bg-3 transition-colors',
          open && 'bg-bg-3 text-text-0',
          pulse && 'text-accent'
        )}>
        <Bell size={16} className={pulse ? 'animate-bounce' : ''} />
        {count > 0 && (
          <span className="absolute -top-0.5 -right-0.5 w-4 h-4 bg-accent text-bg-0 text-[10px] font-bold rounded-full flex items-center justify-center">
            {count > 9 ? '9+' : count}
          </span>
        )}
        {pulse && (
          <span className="absolute -top-0.5 -right-0.5 w-4 h-4 rounded-full bg-accent/60 animate-ping" />
        )}
      </button>

      {open && (
        <div className="absolute right-0 top-10 w-80 bg-bg-2 border border-border-2 rounded shadow-xl z-50 animate-scale-in">
          <div className="flex items-center justify-between px-4 py-3 border-b border-border-1">
            <span className="text-sm font-medium text-text-0">Notifications</span>
            {count > 0 && (
              <button onClick={() => markAll.mutate()}
                className="text-xs text-accent hover:text-accent-dim transition-colors flex items-center gap-1">
                <Check size={12} /> Mark all read
              </button>
            )}
          </div>

          <div className="max-h-80 overflow-y-auto divide-y divide-border-1">
            {notifications.length === 0 && (
              <p className="text-center text-sm text-text-3 py-8">No notifications</p>
            )}
            {notifications.map((n) => {
              const isInvitation = n.type === 'project_invitation_sent'
              const token = isInvitation ? String(n.payload?.token ?? '') : ''
              const projectName = String(n.payload?.project_name ?? n.payload?.project_id ?? '')
              const invitedByEmail = String(n.payload?.invited_by_email ?? '')

              return (
                <div key={n.id}
                  className={clsx('px-4 py-3 flex gap-3 hover:bg-bg-3 transition-colors', !n.read_at && 'bg-accent/5')}>

                  <div className="mt-0.5 flex-shrink-0">
                    {isInvitation
                      ? <UserPlus size={14} className="text-accent" />
                      : <div className={clsx('w-1.5 h-1.5 rounded-full mt-1', !n.read_at ? 'bg-accent' : 'bg-border-3')} />
                    }
                  </div>

                  <div className="flex-1 min-w-0">
                    {isInvitation ? (
                      <>
                        <p className="text-sm text-text-0 leading-snug">
                          <span className="font-medium">{invitedByEmail}</span> hat dich zu{' '}
                          <span className="font-medium text-accent">{projectName}</span> eingeladen
                        </p>
                        <p className="mono mt-0.5 mb-2">{formatDistanceToNow(new Date(n.created_at), { addSuffix: true })}</p>
                        {!n.read_at && token && (
                          <div className="flex gap-2">
                            <button
                              onClick={() => acceptInvitation.mutate({ notifId: n.id, token })}
                              disabled={acceptInvitation.isPending || declineInvitation.isPending}
                              className="flex items-center gap-1 px-2.5 py-1 bg-success/10 text-success border border-success/30 rounded text-xs font-medium hover:bg-success/20 transition-colors">
                              <CheckCircle size={11} /> Annehmen
                            </button>
                            <button
                              onClick={() => declineInvitation.mutate({ notifId: n.id, token })}
                              disabled={acceptInvitation.isPending || declineInvitation.isPending}
                              className="flex items-center gap-1 px-2.5 py-1 bg-danger/10 text-danger border border-danger/30 rounded text-xs font-medium hover:bg-danger/20 transition-colors">
                              <XCircle size={11} /> Ablehnen
                            </button>
                          </div>
                        )}
                      </>
                    ) : (
                      <>
                        <p className="text-sm text-text-1 leading-snug">
                          {String(n.payload?.message ?? n.type)}
                        </p>
                        <p className="mono mt-1">{formatDistanceToNow(new Date(n.created_at), { addSuffix: true })}</p>
                      </>
                    )}
                  </div>

                  {!n.read_at && !isInvitation && (
                    <button onClick={() => markRead.mutate(n.id)}
                      className="text-text-3 hover:text-accent transition-colors mt-0.5 flex-shrink-0">
                      <X size={13} />
                    </button>
                  )}
                </div>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}
